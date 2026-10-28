// Package api is the HTTP interface of the Vault Service. Response shapes
// match the Web client's contracts in src/web/contracts/.
package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/getfumori/fumori/internal/format"
	"github.com/getfumori/fumori/internal/model"
	"github.com/getfumori/fumori/internal/vault"
)

type Config struct {
	AutosaveDebounce   time.Duration
	AutosaveMaxDirty   time.Duration
	CheckpointInterval time.Duration
	// Web holds the built Web client; nil serves the API only.
	Web fs.FS
}

type server struct {
	v   *vault.Vault
	cfg Config
}

// Handler serves the API and, when configured, the Web client.
func Handler(v *vault.Vault, cfg Config) http.Handler {
	s := &server{v: v, cfg: cfg}
	mux := http.NewServeMux()
	routes := map[string]func(http.ResponseWriter, *http.Request) error{
		"GET /api/v1/config":                      s.config,
		"POST /api/v1/checkpoint":                 s.checkpoint,
		"GET /api/v1/diagnostics":                 s.diagnostics,
		"GET /api/v1/today":                       s.today,
		"GET /api/v1/daily/{date}":                s.getDaily,
		"PUT /api/v1/daily/{date}":                s.putDaily,
		"POST /api/v1/daily/{date}":               s.postDaily,
		"GET /api/v1/notes":                       s.listNotes,
		"POST /api/v1/notes":                      s.createNote,
		"GET /api/v1/notes/{id}":                  s.getNote,
		"PUT /api/v1/notes/{id}":                  s.putNote,
		"DELETE /api/v1/notes/{id}":               s.deleteNote,
		"GET /api/v1/notes/{id}/deletion-impact":  s.deletionImpact,
		"POST /api/v1/notes/{id}/rename-to-title": s.renameToTitle,
		"GET /api/v1/notes/{id}/connections":      s.connections,
		"GET /api/v1/connections/{id}":            s.connections,
		"GET /api/v1/wikilinks/suggestions":       s.suggestions,
		"GET /api/v1/inbox":                       s.inbox,
		"GET /api/v1/archive":                     s.archive,
		"GET /api/v1/model":                       s.model,
		"GET /api/v1/types":                       s.types,
		"GET /api/v1/types/{key}":                 s.typeResult,
		"GET /api/v1/views":                       s.views,
		"GET /api/v1/views/{key}":                 s.viewResult,
		"GET /api/v1/search":                      s.search,
		"POST /api/v1/capture":                    s.capture,
		"POST /api/v1/memos/{id}/comments":        s.comment,
		"GET /api/v1/tasks":                       s.tasks,
		"POST /api/v1/tasks/{id}/{action}":        s.taskAction,
	}
	for pattern, h := range routes {
		mux.HandleFunc(pattern, wrap(h))
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	})
	if cfg.Web != nil {
		mux.Handle("/", webHandler(cfg.Web))
	}
	return mux
}

func wrap(h func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if err := h(w, r); err != nil {
			writeError(w, err)
		}
	}
}

type badRequest struct{ message string }

func (e badRequest) Error() string { return e.message }

func writeError(w http.ResponseWriter, err error) {
	var stale *vault.StaleError
	var invalid *vault.InvalidError
	var impact *vault.DeletionImpactError
	var conflict *vault.ConflictError
	var bad badRequest
	switch {
	case errors.Is(err, vault.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	case errors.As(err, &stale):
		var current any
		if stale.Current != "" {
			current = stale.Current
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error": "stale_revision", "currentRevision": current})
	case errors.Is(err, vault.ErrExplicitCreation):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "explicit_creation_required"})
	case errors.As(err, &impact):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "deletion_impact_changed", "currentIncomingLinkCount": impact.Count})
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "conflict", "message": conflict.Message})
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid_canonical_markdown", "message": invalid.Message})
	case errors.As(err, &bad):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": bad.message})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal", "message": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) error {
	writeJSON(w, http.StatusOK, v)
	return nil
}

func decodeBody(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest{"request body is not valid JSON: " + err.Error()}
	}
	return nil
}

func (s *server) vaultRef() map[string]string {
	return map[string]string{"id": s.v.Manifest.ID, "name": s.v.Manifest.Name}
}

func (s *server) config(w http.ResponseWriter, r *http.Request) error {
	return ok(w, map[string]any{
		"autosave":   map[string]int64{"debounceMs": s.cfg.AutosaveDebounce.Milliseconds(), "maxDirtyMs": s.cfg.AutosaveMaxDirty.Milliseconds()},
		"checkpoint": map[string]int64{"intervalMs": s.cfg.CheckpointInterval.Milliseconds()},
	})
}

func (s *server) checkpoint(w http.ResponseWriter, r *http.Request) error {
	result, err := s.v.Checkpoint(r.Context())
	if err != nil {
		return err
	}
	return ok(w, result)
}

func (s *server) diagnostics(w http.ResponseWriter, r *http.Request) error {
	out := []map[string]any{}
	for _, q := range s.v.Diagnostics() {
		out = append(out, map[string]any{"path": q.Path, "revision": q.Revision, "problems": q.Problems})
	}
	return ok(w, out)
}

func strs(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

func typeOf(o *vault.Object) any {
	if o.Type == nil {
		return nil
	}
	return *o.Type
}

func (s *server) noteJSON(o *vault.Object) map[string]any {
	return map[string]any{
		"id": o.ID, "title": o.Title, "canonicalPath": o.Path, "revision": o.Revision,
		"bodyMarkdown":   strings.TrimSuffix(strings.TrimPrefix(o.Doc.Body(), "\n"), "\n"),
		"sourceMarkdown": o.Source, "type": typeOf(o), "state": o.State,
		"tags": strs(o.Tags), "aliases": strs(o.Aliases), "properties": o.Properties,
		"relationships": o.Relationships, "vault": s.vaultRef(),
	}
}

func listItem(o *vault.Object) map[string]any {
	return map[string]any{"id": o.ID, "title": o.Title, "canonicalPath": o.Path, "revision": o.Revision, "state": o.State, "url": vault.URL(o)}
}

func (s *server) dailyJSON(view vault.DailyView) map[string]any {
	if o := view.Object; o != nil {
		free, _ := vault.DailyParts(o)
		return map[string]any{
			"id": o.ID, "date": o.Date, "exists": true, "revision": o.Revision,
			"bodyMarkdown": free, "sourceMarkdown": o.Source, "type": "daily-note", "state": o.State,
			"tags": strs(o.Tags), "aliases": strs(o.Aliases), "properties": o.Properties,
			"vault": s.vaultRef(), "memos": memosJSON(o),
		}
	}
	var source any
	if view.Date == s.v.Today() {
		source = s.v.NewDailySource(view.Date)
	}
	return map[string]any{
		"id": view.VirtualID, "date": view.Date, "exists": false, "revision": nil,
		"bodyMarkdown": "", "sourceMarkdown": source, "type": "daily-note",
		"state": s.v.Model.Type("daily-note").DefaultState, "tags": []string{}, "aliases": []string{},
		"properties": map[string]any{}, "vault": s.vaultRef(), "memos": []any{},
	}
}

func memosJSON(o *vault.Object) []map[string]any {
	body := o.Doc.Body()
	memos := []map[string]any{}
	index := map[string]int{}
	for _, b := range o.Blocks {
		entry := map[string]any{"id": b.ID, "created": b.MetaString("created"), "markdown": b.Content(body)}
		switch b.Kind {
		case format.KindMemo:
			entry["comments"] = []map[string]any{}
			index[b.ID] = len(memos)
			memos = append(memos, entry)
		case format.KindComment:
			if i, ok := index[b.MetaString("parent")]; ok {
				memos[i]["comments"] = append(memos[i]["comments"].([]map[string]any), entry)
			}
		}
	}
	return memos
}

func (s *server) today(w http.ResponseWriter, r *http.Request) error {
	view, err := s.v.Daily(s.v.Today())
	if err != nil {
		return err
	}
	return ok(w, s.dailyJSON(view))
}

func (s *server) getDaily(w http.ResponseWriter, r *http.Request) error {
	view, err := s.v.Daily(r.PathValue("date"))
	if err != nil {
		return err
	}
	return ok(w, s.dailyJSON(view))
}

type saveRequest struct {
	Format        string         `json:"format"`
	BaseRevision  *string        `json:"baseRevision"`
	BodyMarkdown  string         `json:"bodyMarkdown"`
	Source        string         `json:"sourceMarkdown"`
	Type          *string        `json:"type"`
	State         string         `json:"state"`
	Tags          []string       `json:"tags"`
	Aliases       []string       `json:"aliases"`
	Properties    map[string]any `json:"properties"`
	Relationships map[string]any `json:"relationships"`
}

func (req saveRequest) toSave() vault.NoteSave {
	base := ""
	if req.BaseRevision != nil {
		base = *req.BaseRevision
	}
	return vault.NoteSave{Format: req.Format, BaseRevision: base, BodyMarkdown: req.BodyMarkdown, Source: req.Source,
		Type: req.Type, State: req.State, Tags: req.Tags, Aliases: req.Aliases, Properties: req.Properties, Relationships: req.Relationships}
}

func (s *server) putDaily(w http.ResponseWriter, r *http.Request) error {
	var req saveRequest
	if err := decodeBody(r, &req); err != nil {
		return err
	}
	date := r.PathValue("date")
	if _, err := s.v.SaveDaily(date, req.toSave()); err != nil {
		return err
	}
	view, err := s.v.Daily(date)
	if err != nil {
		return err
	}
	return ok(w, s.dailyJSON(view))
}

func (s *server) postDaily(w http.ResponseWriter, r *http.Request) error {
	date := r.PathValue("date")
	_, created, err := s.v.CreateDaily(date)
	if err != nil {
		return err
	}
	view, err := s.v.Daily(date)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, s.dailyJSON(view))
	return nil
}

func (s *server) notesWhere(keep func(string) bool) []map[string]any {
	out := []map[string]any{}
	for _, o := range s.v.Notes(keep) {
		out = append(out, listItem(o))
	}
	return out
}

func (s *server) listNotes(w http.ResponseWriter, r *http.Request) error {
	return ok(w, s.notesWhere(func(state string) bool { return state != s.v.Model.ArchivedState }))
}

func (s *server) inbox(w http.ResponseWriter, r *http.Request) error {
	return ok(w, s.notesWhere(func(state string) bool { return state == s.v.Model.CreationState }))
}

func (s *server) archive(w http.ResponseWriter, r *http.Request) error {
	return ok(w, s.notesWhere(func(state string) bool { return state == s.v.Model.ArchivedState }))
}

func (s *server) createNote(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Context string `json:"context"`
		Type    string `json:"type"`
		Target  string `json:"target"`
	}
	if err := decodeBody(r, &req); err != nil {
		return err
	}
	typeKey, title := "note", ""
	switch req.Context {
	case "global", "inbox":
	case "type":
		typeKey = req.Type
	case "wikilink":
		title = strings.TrimSpace(req.Target)
		if title == "" || strings.ContainsAny(title, "[]|\n") {
			return badRequest{"invalid wikilink target"}
		}
	default:
		return badRequest{"unknown creation context"}
	}
	o, err := s.v.CreateNote(typeKey, title)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, s.noteJSON(o))
	return nil
}

func (s *server) getNote(w http.ResponseWriter, r *http.Request) error {
	o, err := s.v.Note(r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, s.noteJSON(o))
}

func (s *server) putNote(w http.ResponseWriter, r *http.Request) error {
	var req saveRequest
	if err := decodeBody(r, &req); err != nil {
		return err
	}
	o, err := s.v.SaveNote(r.PathValue("id"), req.toSave())
	if err != nil {
		return err
	}
	return ok(w, s.noteJSON(o))
}

func (s *server) deleteNote(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		BaseRevision string `json:"baseRevision"`
		Confirmed    int    `json:"confirmedIncomingLinkCount"`
	}
	if err := decodeBody(r, &req); err != nil {
		return err
	}
	if err := s.v.DeleteNote(r.PathValue("id"), req.BaseRevision, req.Confirmed); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) deletionImpact(w http.ResponseWriter, r *http.Request) error {
	o, count, err := s.v.DeletionImpact(r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"id": o.ID, "revision": o.Revision, "incomingLinkCount": count})
}

func (s *server) renameToTitle(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		BaseRevision string `json:"baseRevision"`
	}
	if err := decodeBody(r, &req); err != nil {
		return err
	}
	o, err := s.v.RenameToTitle(r.Context(), r.PathValue("id"), req.BaseRevision)
	if err != nil {
		return err
	}
	return ok(w, s.noteJSON(o))
}

func linked(o *vault.Object) map[string]any {
	title := o.Title
	if o.Kind == vault.KindDaily {
		title = o.Date
	}
	return map[string]any{"id": o.ID, "title": title, "url": vault.URL(o)}
}

func resolvedJSON(r vault.Resolved) map[string]any {
	matches := []map[string]any{}
	for _, m := range r.Matches {
		matches = append(matches, linked(m))
	}
	var url any
	if r.Status == "resolved" {
		url = vault.URL(r.Matches[0])
	}
	return map[string]any{"target": r.Target, "label": r.Label, "sourceMarkdown": r.SourceMarkdown, "status": r.Status, "url": url, "matches": matches}
}

func (s *server) connections(w http.ResponseWriter, r *http.Request) error {
	c, err := s.v.Connections(r.PathValue("id"))
	if err != nil {
		return err
	}
	outgoing, backlinks, rels, inverse := []any{}, []any{}, []any{}, []any{}
	for _, o := range c.Outgoing {
		outgoing = append(outgoing, resolvedJSON(o))
	}
	for _, o := range c.Backlinks {
		backlinks = append(backlinks, linked(o))
	}
	for _, rl := range c.Relationships {
		targets := []any{}
		for _, t := range rl.Targets {
			targets = append(targets, resolvedJSON(t))
		}
		rels = append(rels, map[string]any{"key": rl.Definition.Key, "name": rl.Definition.Name, "cardinality": rl.Definition.Cardinality, "targets": targets})
	}
	for _, inv := range c.Inverse {
		inverse = append(inverse, map[string]any{"key": inv.Key, "source": linked(inv.Source)})
	}
	return ok(w, map[string]any{"outgoing": outgoing, "backlinks": backlinks, "relationships": rels, "inverseRelationships": inverse})
}

func (s *server) suggestions(w http.ResponseWriter, r *http.Request) error {
	out := []map[string]any{}
	for _, o := range s.v.Suggestions(r.URL.Query().Get("q")) {
		l := linked(o)
		out = append(out, map[string]any{"target": l["title"], "title": l["title"], "url": l["url"]})
	}
	return ok(w, out)
}

func (s *server) model(w http.ResponseWriter, r *http.Request) error { return ok(w, s.v.Model) }

func (s *server) types(w http.ResponseWriter, r *http.Request) error { return ok(w, s.v.Model.Types) }

func (s *server) views(w http.ResponseWriter, r *http.Request) error { return ok(w, s.v.Model.Views) }

func structured(o *vault.Object) map[string]any {
	title := o.Title
	if o.Kind == vault.KindDaily {
		title = o.Date
	}
	return map[string]any{
		"kind": string(o.Kind), "id": o.ID, "title": title, "canonicalPath": o.Path, "revision": o.Revision,
		"type": typeOf(o), "state": o.State, "tags": strs(o.Tags), "aliases": strs(o.Aliases),
		"properties": o.Properties, "url": vault.URL(o), "fields": vault.Fields(o),
	}
}

func (s *server) typeResult(w http.ResponseWriter, r *http.Request) error {
	t := s.v.Model.Type(r.PathValue("key"))
	if t == nil {
		return vault.ErrNotFound
	}
	items := []any{}
	for _, o := range s.v.Query(model.QuerySpec{Filter: &model.Filter{Field: "type", Operator: "equals", Value: t.Key}}) {
		items = append(items, structured(o))
	}
	result := map[string]any{"key": t.Key, "name": t.Name, "properties": t.Properties, "items": items}
	if t.DefaultState != "" {
		result["defaultState"] = t.DefaultState
	}
	return ok(w, result)
}

func (s *server) viewResult(w http.ResponseWriter, r *http.Request) error {
	view := s.v.Model.View(r.PathValue("key"))
	if view == nil {
		return vault.ErrNotFound
	}
	items := []any{}
	groups := []map[string]any{}
	groupIndex := map[string]int{}
	for _, o := range s.v.Query(view.Query) {
		item := structured(o)
		items = append(items, item)
		if view.Query.GroupBy == "" {
			continue
		}
		key := ""
		switch value := vault.Fields(o)[view.Query.GroupBy].(type) {
		case string:
			key = value
		case []string:
			key = strings.Join(value, ", ")
		}
		i, seen := groupIndex[key]
		if !seen {
			i = len(groups)
			groupIndex[key] = i
			groups = append(groups, map[string]any{"key": key, "items": []any{}})
		}
		groups[i]["items"] = append(groups[i]["items"].([]any), item)
	}
	return ok(w, map[string]any{"key": view.Key, "name": view.Name, "query": view.Query, "groups": groups, "items": items})
}

func (s *server) search(w http.ResponseWriter, r *http.Request) error {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" || len(query) > 500 {
		return badRequest{"query must be 1 to 500 characters"}
	}
	results, err := s.v.Search(query)
	if err != nil {
		return badRequest{err.Error()}
	}
	out := []map[string]any{}
	for _, hit := range results {
		o := hit.Object
		kind, title := "note", o.Title
		if o.Kind == vault.KindDaily {
			kind, title = "daily-note", o.Date
		}
		out = append(out, map[string]any{"kind": kind, "id": o.ID, "title": title, "canonicalPath": o.Path, "url": vault.URL(o), "revision": o.Revision, "snippet": hit.Snippet})
	}
	return ok(w, out)
}
