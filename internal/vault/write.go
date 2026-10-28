package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/getfumori/fumori/internal/format"
)

// commitLocked validates and publishes one object, replacing whatever was at
// oldPath. The caller holds v.mu for writing.
func (v *Vault) commitLocked(oldPath, rel, source string) (*Object, error) {
	o, err := v.validateLocked(oldPath, rel, source)
	if err != nil {
		return nil, err
	}
	if err := v.publish(rel, source); err != nil {
		return nil, err
	}
	if oldPath != "" && oldPath != rel {
		if err := os.Remove(filepath.Join(v.Root, filepath.FromSlash(oldPath))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	v.installLocked(oldPath, o)
	return o, nil
}

func (v *Vault) validateLocked(oldPath, rel, source string) (*Object, error) {
	o, problems := decodeObject(rel, source, v.Model)
	if problems != nil {
		return nil, &InvalidError{Message: strings.Join(problems, "; ")}
	}
	if other, ok := v.byID[o.ID]; ok && other.Path != oldPath {
		return nil, invalidf("id %s already belongs to %s", o.ID, other.Path)
	}
	for _, id := range blockIDs(o) {
		if owner, ok := v.blockOwner[id]; ok && owner != oldPath {
			return nil, invalidf("block id %s already belongs to %s", id, owner)
		}
	}
	return o, nil
}

func (v *Vault) installLocked(oldPath string, o *Object) {
	v.forgetLocked(oldPath)
	v.forgetLocked(o.Path)
	v.byPath[o.Path] = o
	v.byID[o.ID] = o
	if o.Kind == KindDaily {
		v.daily[o.Date] = o
	}
	for _, id := range blockIDs(o) {
		v.blockOwner[id] = o.Path
	}
	v.rebuildTargetsLocked()
	if v.index != nil {
		v.indexError(v.index.Put(o))
	}
}

func (v *Vault) forgetLocked(rel string) {
	delete(v.quarantine, rel)
	o, ok := v.byPath[rel]
	if !ok {
		return
	}
	delete(v.byPath, rel)
	delete(v.byID, o.ID)
	if o.Kind == KindDaily {
		delete(v.daily, o.Date)
	}
	for _, id := range blockIDs(o) {
		delete(v.blockOwner, id)
	}
	if v.index != nil {
		v.indexError(v.index.Remove(rel))
	}
}

// indexError records an index failure. The file is already durable and the
// next startup re-indexes it by content hash, so the write still succeeds.
func (v *Vault) indexError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fumori: search index update failed: %v\n", err)
	}
}

// crashAt lets tests stop a write at a named crash window. It is nil in
// production.
var crashHook func(point string) error

func crashAt(point string) error {
	if crashHook == nil {
		return nil
	}
	return crashHook(point)
}

// Command runs one idempotent write. respond builds the response from the
// written object; with a key, the same request returns the recorded response
// and a reused key with a different request is a conflict.
func (v *Vault) command(key, fingerprint, rel string, apply func(cur *Object) (string, error), respond func(o *Object) any) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if key != "" {
		in, err := v.store.lookup(key)
		switch {
		case err == nil && in.fingerprint != fingerprint:
			return nil, &ConflictError{Message: "idempotency key was used for a different request"}
		case err == nil && in.status == "completed":
			return in.response, nil
		case err != nil && !errors.Is(err, errNoIntent):
			return nil, err
		}
	}
	cur := v.byPath[rel]
	if cur == nil {
		return nil, ErrNotFound
	}
	source, err := apply(cur)
	if err != nil {
		return nil, err
	}
	o, err := v.validateLocked(rel, rel, source)
	if err != nil {
		return nil, err
	}
	response, err := json.Marshal(respond(o))
	if err != nil {
		return nil, err
	}
	if key != "" {
		if err := v.store.begin(key, fingerprint, rel, cur.Revision, o.Revision, response); err != nil {
			return nil, err
		}
	}
	if err := crashAt("after-intent"); err != nil {
		return nil, err
	}
	if err := v.publish(rel, source); err != nil {
		return nil, err
	}
	if err := crashAt("after-publish"); err != nil {
		return nil, err
	}
	v.installLocked(rel, o)
	if key != "" {
		if err := v.store.complete(key); err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (v *Vault) newUUID() string {
	b := make([]byte, 16)
	if _, err := v.random.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (v *Vault) newBlockID(prefix string) string { return format.NewBlockID(v.random, prefix) }

func frontmatterText(lines ...string) string {
	return "---\n" + strings.Join(lines, "\n") + "\n---\n"
}

func defaultProperties(v *Vault, typeKey string) []string {
	var out []string
	for _, p := range v.Model.Type(typeKey).Properties {
		if p.Default == nil {
			continue
		}
		encoded, _ := json.Marshal(p.Default)
		out = append(out, fmt.Sprintf("%s: %s", p.Key, encoded))
	}
	return out
}

var slugSeparators = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func readableSlug(title string) string {
	decomposed := norm.NFKD.String(title)
	var b strings.Builder
	for _, r := range decomposed {
		if !(r >= 0x300 && r <= 0x36f) {
			b.WriteRune(r)
		}
	}
	slug := strings.Trim(slugSeparators.ReplaceAllString(strings.ToLower(b.String()), "-"), "-")
	if runes := []rune(slug); len(runes) > 80 {
		slug = strings.TrimRight(string(runes[:80]), "-")
	}
	if slug == "" {
		return "note"
	}
	return slug
}

func (v *Vault) availableNotePathLocked(slug, own string) string {
	for n := 1; ; n++ {
		name := slug + ".md"
		if n > 1 {
			name = fmt.Sprintf("%s-%d.md", slug, n)
		}
		rel := path.Join(notesDir, name)
		if rel == own {
			return rel
		}
		if _, taken := v.byPath[rel]; taken {
			continue
		}
		if _, err := os.Stat(filepath.Join(v.Root, filepath.FromSlash(rel))); errors.Is(err, os.ErrNotExist) {
			return rel
		}
	}
}

// CreateNote creates a standalone Human Note of the given Type.
func (v *Vault) CreateNote(typeKey, title string) (*Object, error) {
	if typeKey == "" {
		typeKey = "note"
	}
	t := v.Model.Type(typeKey)
	if t == nil || typeKey == "daily-note" {
		return nil, invalidf("unknown type %q", typeKey)
	}
	state := t.DefaultState
	if state == "" {
		state = v.Model.CreationState
	}
	id := v.newUUID()
	lines := append([]string{
		"_id: " + id, "_schema: fumori.note", "_version: 1", fmt.Sprintf("_created: %q", v.now()),
		"type: " + typeKey, "state: " + state, "tags: []", "aliases: []",
	}, defaultProperties(v, typeKey)...)
	body := ""
	if title != "" {
		body = "# " + title
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	rel := path.Join(notesDir, "note-"+id+".md")
	if title != "" {
		rel = v.availableNotePathLocked(readableSlug(title), "")
	}
	return v.commitLocked("", rel, composeNote(frontmatterText(lines...), body))
}

// NoteSave is one save request from an editor.
type NoteSave struct {
	Format        string
	BaseRevision  string
	BodyMarkdown  string
	Source        string
	Type          *string
	State         string
	Tags          []string
	Aliases       []string
	Properties    map[string]any
	Relationships map[string]any
}

// Note returns a valid note or a quarantined one by ID.
func (v *Vault) Note(id string) (*Object, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	o, ok := v.byID[id]
	if !ok || o.Kind != KindNote {
		return nil, ErrNotFound
	}
	return o, nil
}

// SaveNote applies an editor save with optimistic concurrency.
func (v *Vault) SaveNote(id string, req NoteSave) (*Object, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	cur, ok := v.byID[id]
	if !ok || cur.Kind != KindNote {
		return nil, ErrNotFound
	}
	if cur.Revision != req.BaseRevision {
		return nil, &StaleError{Current: cur.Revision}
	}
	source, err := v.applyNoteSave(cur, req)
	if err != nil {
		return nil, err
	}
	if req.Format == "raw" {
		submitted, problems := decodeObject(cur.Path, source, v.Model)
		if problems == nil && (submitted.ID != cur.ID || submitted.Created != cur.Created) {
			return nil, invalidf("reserved fields _id and _created cannot be changed")
		}
	}
	rel := cur.Path
	if path.Base(cur.Path) == "note-"+cur.ID+".md" {
		if doc, err := format.Parse(source); err == nil {
			if title := format.Title(doc.Body()); title != "" {
				rel = v.availableNotePathLocked(readableSlug(title), cur.Path)
			}
		}
	}
	return v.commitLocked(cur.Path, rel, source)
}

func (v *Vault) applyNoteSave(cur *Object, req NoteSave) (string, error) {
	switch req.Format {
	case "raw":
		return req.Source, nil
	case "rich":
		return composeNote(cur.Doc.Frontmatter(), req.BodyMarkdown), nil
	case "metadata", "document":
		source, err := v.editMetadata(cur, req)
		if err != nil {
			return "", err
		}
		if req.Format == "document" {
			doc, err := format.Parse(source)
			if err != nil {
				return "", err
			}
			source = composeNote(doc.Frontmatter(), req.BodyMarkdown)
		}
		return source, nil
	}
	return "", invalidf("unknown save format %q", req.Format)
}

func (v *Vault) editMetadata(cur *Object, req NoteSave) (string, error) {
	if !v.Model.HasState(req.State) {
		return "", invalidf("unknown state %q", req.State)
	}
	var changes []format.Change
	if cur.Kind == KindNote {
		if req.Type == nil {
			changes = append(changes, format.Change{Key: "type", Value: format.Null{}})
		} else {
			if v.Model.Type(*req.Type) == nil {
				return "", invalidf("unknown type %q", *req.Type)
			}
			changes = append(changes, format.Change{Key: "type", Value: *req.Type})
		}
	}
	changes = append(changes,
		format.Change{Key: "state", Value: req.State},
		format.Change{Key: "tags", Value: nonNil(req.Tags)},
		format.Change{Key: "aliases", Value: nonNil(req.Aliases)},
	)
	known := map[string]bool{}
	typeKey := ""
	if cur.Kind == KindDaily {
		typeKey = "daily-note"
	} else if req.Type != nil {
		typeKey = *req.Type
	}
	if typeKey != "" {
		for _, p := range v.Model.Type(typeKey).Properties {
			known[p.Key] = true
		}
	}
	for key, value := range req.Properties {
		if !known[key] {
			return "", invalidf("type does not define property %q", key)
		}
		changes = append(changes, format.Change{Key: key, Value: value})
	}
	if req.Relationships != nil {
		for _, rel := range v.Model.Relationships {
			value, ok := req.Relationships[rel.Key]
			if !ok {
				changes = append(changes, format.Change{Key: rel.Key})
				continue
			}
			changes = append(changes, format.Change{Key: rel.Key, Value: value})
		}
	}
	return format.Edit(cur.Source, changes)
}

func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// DeleteNote removes a note after the owner confirmed its incoming links.
func (v *Vault) DeleteNote(id, base string, confirmedLinks int) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	cur, ok := v.byID[id]
	if !ok || cur.Kind != KindNote {
		return ErrNotFound
	}
	if cur.Revision != base {
		return &StaleError{Current: cur.Revision}
	}
	if count := v.incomingLinkCountLocked(id); count != confirmedLinks {
		return &DeletionImpactError{Count: count}
	}
	if err := os.Remove(filepath.Join(v.Root, filepath.FromSlash(cur.Path))); err != nil {
		return err
	}
	if err := syncDir(filepath.Join(v.Root, notesDir)); err != nil {
		return err
	}
	v.forgetLocked(cur.Path)
	v.rebuildTargetsLocked()
	return nil
}

// DailyView is a Daily Note, real or not yet created.
type DailyView struct {
	Date   string
	Object *Object
	// VirtualID identifies a Daily Note that does not exist yet, so the
	// editor has a stable ID before the first write.
	VirtualID string
}

// Daily returns the Daily Note for a date without creating it.
func (v *Vault) Daily(date string) (DailyView, error) {
	if !validDate(date) {
		return DailyView{}, invalidf("invalid date %q", date)
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if o, ok := v.daily[date]; ok {
		return DailyView{Date: date, Object: o}, nil
	}
	return DailyView{Date: date, VirtualID: v.virtualDailyID(date)}, nil
}

func (v *Vault) virtualDailyID(date string) string {
	sum := revision(v.Manifest.ID + "/daily/" + date)
	return fmt.Sprintf("%s-%s-4%s-8%s-%s", sum[0:8], sum[8:12], sum[13:16], sum[17:20], sum[20:32])
}

func (v *Vault) newDailySource(date, free string) string {
	lines := append([]string{
		"_id: " + v.virtualDailyID(date), "_schema: fumori.daily-note", "_version: 1", fmt.Sprintf("_created: %q", v.now()),
		"type: daily-note", "state: " + orDefault(v.Model.Type("daily-note").DefaultState, v.Model.CreationState),
		"tags: []", "aliases: []",
	}, defaultProperties(v, "daily-note")...)
	lines = append(lines, fmt.Sprintf("date: %q", date))
	return frontmatterText(lines...) + composeDaily(date, free, "")
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// CreateDaily creates a Daily Note for any date. It reports whether it
// created one.
func (v *Vault) CreateDaily(date string) (*Object, bool, error) {
	if !validDate(date) {
		return nil, false, invalidf("invalid date %q", date)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if o, ok := v.daily[date]; ok {
		return o, false, nil
	}
	o, err := v.commitLocked("", dailyRelPath(date), v.newDailySource(date, ""))
	return o, err == nil, err
}

// SaveDaily applies an editor save. A missing Daily Note is created by the
// first save only for today; other dates need CreateDaily.
func (v *Vault) SaveDaily(date string, req NoteSave) (*Object, error) {
	if !validDate(date) {
		return nil, invalidf("invalid date %q", date)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	cur, exists := v.daily[date]
	current := ""
	if exists {
		current = cur.Revision
	}
	if current != req.BaseRevision {
		return nil, &StaleError{Current: current}
	}
	rel := dailyRelPath(date)
	if !exists {
		if date != v.Today() {
			return nil, ErrExplicitCreation
		}
		if req.Format == "raw" {
			return v.commitLocked("", rel, req.Source)
		}
		created, err := v.validateLocked("", rel, v.newDailySource(date, ""))
		if err != nil {
			return nil, err
		}
		cur = created
	}
	var source string
	switch req.Format {
	case "raw":
		submitted, problems := decodeObject(rel, req.Source, v.Model)
		if problems == nil && (submitted.ID != cur.ID || submitted.Created != cur.Created) {
			return nil, invalidf("reserved fields _id and _created cannot be changed")
		}
		source = req.Source
	case "rich", "document":
		source = cur.Source
		if req.Format == "document" {
			edited, err := v.editMetadata(cur, req)
			if err != nil {
				return nil, err
			}
			source = edited
		}
		doc, err := format.Parse(source)
		if err != nil {
			return nil, err
		}
		_, stream := dailyParts(cur)
		source = doc.WithBody(composeDaily(date, req.BodyMarkdown, stream))
	default:
		return nil, invalidf("unknown save format %q", req.Format)
	}
	return v.commitLocked(rel, rel, source)
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validDate(s string) bool {
	if !datePattern.MatchString(s) {
		return false
	}
	_, err := parseDate(s)
	return err == nil
}
