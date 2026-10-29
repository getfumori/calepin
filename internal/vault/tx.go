package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/getfumori/fumori/internal/format"
)

// txOp moves one file from its base content at From to its target content
// at To. From and To are equal unless the file is renamed.
type txOp struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Base   string `json:"base"`
	Target string `json:"target"`
	Staged string `json:"staged"`
}

type journal struct {
	Label string `json:"label"`
	Ops   []txOp `json:"ops"`
}

func (v *Vault) txRoot() string { return filepath.Join(v.StateDir, "tx") }

// transactLocked stages every new file, commits a journal, then applies it.
// A crash after the journal commit rolls forward on the next start.
func (v *Vault) transactLocked(ctx context.Context, label string, changes []txChange) error {
	dir, err := os.MkdirTemp(ensureDir(v.txRoot()), "tx-")
	if err != nil {
		return err
	}
	j := journal{Label: label}
	for i, c := range changes {
		staged := strconv.Itoa(i)
		if err := writeDurable(filepath.Join(dir, staged), c.source); err != nil {
			return err
		}
		j.Ops = append(j.Ops, txOp{From: c.from, To: c.to, Base: c.base, Target: revision(c.source), Staged: staged})
	}
	raw, _ := json.Marshal(j)
	if err := writeDurable(filepath.Join(dir, "journal.json"), string(raw)); err != nil {
		return err
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	if err := crashAt("after-journal"); err != nil {
		return err
	}
	if err := v.applyJournal(dir, j); err != nil {
		return err
	}
	_, err = v.checkpoint(ctx, "transaction: "+label)
	return err
}

type txChange struct {
	from, to, base, source string
}

func ensureDir(dir string) string {
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

func writeDurable(path, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (v *Vault) applyJournal(dir string, j journal) error {
	for _, op := range j.Ops {
		if v.currentRevision(op.To) != op.Target {
			if op.From == op.To || v.currentRevision(op.From) == op.Base {
				staged, err := os.ReadFile(filepath.Join(dir, op.Staged))
				if err != nil {
					return err
				}
				if err := v.publish(op.To, string(staged)); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("transaction %s cannot finish: %s changed outside Calepin", j.Label, op.From)
			}
		}
		if op.From != op.To && v.currentRevision(op.From) == op.Base {
			if err := os.Remove(filepath.Join(v.Root, filepath.FromSlash(op.From))); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return os.RemoveAll(dir)
}

// recoverTransactions rolls committed journals forward and drops staging
// directories that never reached their commit point.
func (v *Vault) recoverTransactions() error {
	entries, err := os.ReadDir(v.txRoot())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		dir := filepath.Join(v.txRoot(), entry.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "journal.json"))
		if errors.Is(err, os.ErrNotExist) {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		var j journal
		if err := json.Unmarshal(raw, &j); err != nil {
			return os.RemoveAll(dir)
		}
		if err := v.applyJournal(dir, j); err != nil {
			return err
		}
	}
	return nil
}

// RenameToTitle renames a note's file to its title slug and rewrites every
// wikilink that resolved to it, as one transaction.
func (v *Vault) RenameToTitle(ctx context.Context, id, base string) (*Object, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	cur, ok := v.byID[id]
	if !ok || cur.Kind != KindNote {
		return nil, ErrNotFound
	}
	if cur.Revision != base {
		return nil, &StaleError{Current: cur.Revision}
	}
	if cur.Title == "Untitled note" {
		return nil, invalidf("rename to title requires a meaningful H1")
	}
	for _, r := range "[]|" {
		if containsRune(cur.Title, r) {
			return nil, invalidf("the title cannot be used as a wikilink target")
		}
	}
	dest := v.availableNotePathLocked(readableSlug(cur.Title), cur.Path)
	var changes []txChange
	for _, o := range v.sortedObjects() {
		source := format.RewriteWikilinks(o.Source, func(target string) bool {
			return v.resolvesToLocked(format.Wikilink{Target: target}, id)
		}, cur.Title)
		to := o.Path
		if o.ID == id {
			to = dest
		}
		if source == o.Source && to == o.Path {
			continue
		}
		if _, err := v.validateLocked(o.Path, to, source); err != nil {
			return nil, err
		}
		changes = append(changes, txChange{from: o.Path, to: to, base: o.Revision, source: source})
	}
	if err := v.transactLocked(ctx, "rename note", changes); err != nil {
		return nil, err
	}
	for _, c := range changes {
		o, _ := v.validateLocked(c.from, c.to, c.source)
		v.installLocked(c.from, o)
	}
	return v.byID[id], nil
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
