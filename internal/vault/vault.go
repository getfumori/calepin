// Package vault owns every read and write of a running Vault: canonical
// files, Git checkpoints, crash recovery, and the in-memory projection.
package vault

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getfumori/fumori/internal/gitrepo"
	"github.com/getfumori/fumori/internal/model"
)

// Index is the Search index as the vault sees it.
type Index interface {
	Sync(objects []*Object) error
	Put(o *Object) error
	Remove(path string) error
	Search(query string, limit int) ([]Hit, error)
	Close() error
}

// Hit is one search result before projection to the API.
type Hit struct {
	Path    string
	Snippet string
}

type Options struct {
	Root     string
	StateDir string
	Clock    func() time.Time
	Random   io.Reader
	// OpenIndex opens the Search index inside the state directory.
	OpenIndex func(stateDir string) (Index, error)
}

type Vault struct {
	Root     string
	StateDir string
	Manifest *model.Manifest
	Model    *model.Model

	clock  func() time.Time
	random io.Reader
	git    gitrepo.Repo
	index  Index
	store  *store
	lock   *os.File

	mu         sync.RWMutex
	byID       map[string]*Object
	byPath     map[string]*Object
	daily      map[string]*Object
	quarantine map[string]*Quarantined
	blockOwner map[string]string
	targets    map[string][]*Object
}

// Open locks the Vault, recovers interrupted work, validates every file, and
// brings the Search index up to date.
func Open(ctx context.Context, opts Options) (*Vault, error) {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.Random == nil {
		opts.Random = rand.Reader
	}
	manifest, err := model.LoadManifest(opts.Root)
	if err != nil {
		return nil, err
	}
	m, err := model.Load(opts.Root)
	if err != nil {
		return nil, err
	}
	if opts.StateDir == "" {
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			base = filepath.Join(home, ".local", "state")
		}
		opts.StateDir = filepath.Join(base, "fumori", manifest.ID)
	}
	if err := os.MkdirAll(opts.StateDir, 0o700); err != nil {
		return nil, err
	}
	v := &Vault{
		Root: opts.Root, StateDir: opts.StateDir, Manifest: manifest, Model: m,
		clock: opts.Clock, random: opts.Random, git: gitrepo.Repo{Root: opts.Root},
	}
	if err := v.acquireLock(); err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			v.Close()
		}
	}()
	if v.store, err = openStore(filepath.Join(opts.StateDir, "state.db")); err != nil {
		return nil, err
	}
	if err := v.git.ClearStaleIndexLock(); err != nil {
		return nil, err
	}
	if err := v.removeTempFiles(); err != nil {
		return nil, err
	}
	if err := v.recoverTransactions(); err != nil {
		return nil, err
	}
	if err := v.scan(); err != nil {
		return nil, err
	}
	if err := v.store.settleIntents(v.currentRevision); err != nil {
		return nil, err
	}
	if opts.OpenIndex != nil {
		if v.index, err = opts.OpenIndex(opts.StateDir); err != nil {
			return nil, err
		}
		if err := v.index.Sync(v.objects()); err != nil {
			return nil, err
		}
	}
	if _, err := v.checkpoint(ctx, "recovery checkpoint"); err != nil {
		return nil, err
	}
	ok = true
	return v, nil
}

func (v *Vault) acquireLock() error {
	f, err := os.OpenFile(filepath.Join(v.StateDir, "vault.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return errors.New("another Fumori process is serving this Vault")
	}
	v.lock = f
	return nil
}

func (v *Vault) Close() error {
	var errs []error
	if v.index != nil {
		errs = append(errs, v.index.Close())
	}
	if v.store != nil {
		errs = append(errs, v.store.close())
	}
	if v.lock != nil {
		errs = append(errs, v.lock.Close())
	}
	return errors.Join(errs...)
}

// removeTempFiles deletes half-written temporary files from a crash. They
// were never renamed into place, so they hold no saved content.
func (v *Vault) removeTempFiles() error {
	return filepath.WalkDir(filepath.Join(v.Root, "human"), func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), ".fumori-tmp-") {
			return os.Remove(p)
		}
		return nil
	})
}

func (v *Vault) scan() error {
	byID := map[string]*Object{}
	byPath := map[string]*Object{}
	daily := map[string]*Object{}
	quarantine := map[string]*Quarantined{}
	blockOwner := map[string]string{}
	claims := map[string][]string{}

	err := filepath.WalkDir(filepath.Join(v.Root, "human"), func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return err
		}
		rel, _ := filepath.Rel(v.Root, p)
		rel = filepath.ToSlash(rel)
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		o, problems := decodeObject(rel, string(raw), v.Model)
		if problems != nil {
			quarantine[rel] = &Quarantined{Path: rel, Revision: revision(string(raw)), Source: string(raw), Problems: problems}
			return nil
		}
		byPath[rel] = o
		claims[o.ID] = append(claims[o.ID], rel)
		for _, id := range blockIDs(o) {
			claims[id] = append(claims[id], rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for id, paths := range claims {
		if len(paths) < 2 {
			continue
		}
		for _, rel := range paths {
			if o := byPath[rel]; o != nil {
				quarantine[rel] = &Quarantined{Path: rel, Revision: o.Revision, Source: o.Source,
					Problems: []string{fmt.Sprintf("id %s is also claimed by %s", id, strings.Join(paths, ", "))}}
				delete(byPath, rel)
			}
		}
	}
	for rel, o := range byPath {
		byID[o.ID] = o
		if o.Kind == KindDaily {
			daily[o.Date] = o
		}
		for _, id := range blockIDs(o) {
			blockOwner[id] = rel
		}
	}
	v.mu.Lock()
	v.byID, v.byPath, v.daily, v.quarantine, v.blockOwner = byID, byPath, daily, quarantine, blockOwner
	v.rebuildTargetsLocked()
	v.mu.Unlock()
	return nil
}

func blockIDs(o *Object) []string {
	ids := make([]string, 0, len(o.Blocks)+len(o.InlineTasks))
	for _, b := range o.Blocks {
		ids = append(ids, b.ID)
	}
	for _, t := range o.InlineTasks {
		ids = append(ids, t.ID)
	}
	return ids
}

func (v *Vault) currentRevision(rel string) string {
	raw, err := os.ReadFile(filepath.Join(v.Root, rel))
	if err != nil {
		return ""
	}
	return revision(string(raw))
}

// objects returns valid objects sorted by path. Callers hold no lock.
func (v *Vault) objects() []*Object {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.sortedObjects()
}

func (v *Vault) sortedObjects() []*Object {
	out := make([]*Object, 0, len(v.byPath))
	for _, o := range v.byPath {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Diagnostics lists quarantined files.
func (v *Vault) Diagnostics() []Quarantined {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]Quarantined, 0, len(v.quarantine))
	for _, q := range v.quarantine {
		out = append(out, *q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Today is the owner's current local date.
func (v *Vault) Today() string { return v.clock().In(v.Manifest.Timezone).Format(time.DateOnly) }

func (v *Vault) now() string { return v.clock().In(v.Manifest.Timezone).Format(time.RFC3339) }

// CheckpointResult reports one checkpoint.
type CheckpointResult struct {
	Created      bool   `json:"created"`
	Commit       string `json:"commit,omitempty"`
	ChangedFiles int    `json:"changedFiles"`
}

// Checkpoint commits the work tree when it changed.
func (v *Vault) Checkpoint(ctx context.Context) (CheckpointResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.checkpoint(ctx, "checkpoint")
}

func (v *Vault) checkpoint(ctx context.Context, label string) (CheckpointResult, error) {
	changed, err := v.git.Dirty(ctx)
	if err != nil || changed == 0 {
		return CheckpointResult{}, err
	}
	sha, n, err := v.git.Commit(ctx, fmt.Sprintf("%s: %d files", label, changed))
	if err != nil {
		return CheckpointResult{}, err
	}
	return CheckpointResult{Created: sha != "", Commit: sha, ChangedFiles: n}, nil
}

// RunCheckpoints checkpoints on an interval until ctx ends.
func (v *Vault) RunCheckpoints(ctx context.Context, interval time.Duration, onError func(error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := v.Checkpoint(ctx); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}

// publish atomically replaces one file and returns once it is durable.
func (v *Vault) publish(rel, source string) error {
	target := filepath.Join(v.Root, filepath.FromSlash(rel))
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".fumori-tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(source); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
