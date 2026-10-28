package vault

import (
	"reflect"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/getfumori/fumori/internal/format"
	"github.com/getfumori/fumori/internal/model"
)

func parseDate(s string) (time.Time, error) { return time.Parse(time.DateOnly, s) }

// URL is the Web route of an object.
func URL(o *Object) string {
	if o.Kind == KindDaily {
		return "/daily/" + o.Date
	}
	return "/notes/" + o.ID
}

func displayTitle(o *Object) string {
	if o.Kind == KindDaily {
		return o.Date
	}
	return o.Title
}

// Notes returns standalone notes whose state passes keep, sorted by title.
func (v *Vault) Notes(keep func(state string) bool) []*Object {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var out []*Object
	for _, o := range v.sortedObjects() {
		if o.Kind == KindNote && keep(o.State) {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// Fields is the queryable projection of one object.
func Fields(o *Object) map[string]any {
	fields := map[string]any{
		"kind": string(o.Kind), "title": displayTitle(o), "canonical_path": o.Path, "created": o.Created,
		"state": o.State, "tags": o.Tags, "aliases": o.Aliases, "type": nil,
	}
	if o.Type != nil {
		fields["type"] = *o.Type
	}
	for k, value := range o.Properties {
		fields[k] = value
	}
	return fields
}

// Query evaluates a QuerySpec over every valid Human Note and Daily Note.
func (v *Vault) Query(q model.QuerySpec) []*Object {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var out []*Object
	for _, o := range v.sortedObjects() {
		if q.Filter == nil || matches(Fields(o), *q.Filter) {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		for _, order := range q.Order {
			c := compareValues(Fields(out[i])[order.Field], Fields(out[j])[order.Field])
			if c != 0 {
				return (c < 0) == (order.Direction != "descending")
			}
		}
		return false
	})
	return out
}

func matches(fields map[string]any, f model.Filter) bool {
	switch {
	case f.All != nil:
		for _, child := range f.All {
			if !matches(fields, child) {
				return false
			}
		}
		return true
	case f.Any != nil:
		for _, child := range f.Any {
			if matches(fields, child) {
				return true
			}
		}
		return false
	case f.Not != nil:
		return !matches(fields, *f.Not)
	}
	candidate, present := fields[f.Field]
	expected := normalizeValue(f.Value)
	switch f.Operator {
	case "equals":
		return equalValues(candidate, expected)
	case "not_equals":
		return !equalValues(candidate, expected)
	case "in", "not_in":
		list, _ := expected.([]string)
		found := false
		for _, entry := range list {
			found = found || equalValues(candidate, entry)
		}
		return found == (f.Operator == "in")
	case "contains":
		if list, ok := candidate.([]string); ok {
			for _, entry := range list {
				if equalValues(entry, expected) {
					return true
				}
			}
			return false
		}
		s, ok1 := candidate.(string)
		e, ok2 := expected.(string)
		return ok1 && ok2 && strings.Contains(s, e)
	case "exists":
		exists := present && candidate != nil
		if want, ok := expected.(bool); ok {
			return exists == want
		}
		return exists
	}
	if !present || candidate == nil || expected == nil {
		return false
	}
	c := compareValues(candidate, expected)
	switch f.Operator {
	case "greater_than":
		return c > 0
	case "greater_than_or_equal":
		return c >= 0
	case "less_than":
		return c < 0
	case "less_than_or_equal":
		return c <= 0
	}
	return false
}

func equalValues(a, b any) bool {
	if fa, ok := toFloat(a); ok {
		fb, ok := toFloat(b)
		return ok && fa == fb
	}
	return reflect.DeepEqual(a, b)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func compareValues(a, b any) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	if fa, ok := toFloat(a); ok {
		if fb, ok := toFloat(b); ok {
			switch {
			case fa < fb:
				return -1
			case fa > fb:
				return 1
			}
			return 0
		}
	}
	return strings.Compare(toString(a), toString(b))
}

func toString(v any) string {
	if list, ok := v.([]string); ok {
		return strings.Join(list, ",")
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Resolved is one wikilink resolved against the Vault.
type Resolved struct {
	format.Wikilink
	Status  string
	Matches []*Object
}

func linkKey(s string) string {
	decomposed := norm.NFKD.String(s)
	var b strings.Builder
	for _, r := range decomposed {
		if !(r >= 0x300 && r <= 0x36f) {
			b.WriteRune(r)
		}
	}
	return strings.Trim(slugSeparators.ReplaceAllString(strings.ToLower(b.String()), "-"), "-")
}

func (v *Vault) rebuildTargetsLocked() {
	targets := map[string][]*Object{}
	for _, o := range v.sortedObjects() {
		filename := strings.TrimSuffix(o.Path[strings.LastIndex(o.Path, "/")+1:], ".md")
		seen := map[string]bool{}
		for _, candidate := range append([]string{displayTitle(o), filename}, o.Aliases...) {
			for _, key := range []string{strings.ToLower(candidate), "\x00" + linkKey(candidate)} {
				if !seen[key] {
					seen[key] = true
					targets[key] = append(targets[key], o)
				}
			}
		}
	}
	v.targets = targets
}

func (v *Vault) resolveLocked(link format.Wikilink) Resolved {
	var found []*Object
	seen := map[*Object]bool{}
	for _, key := range []string{strings.ToLower(link.Target), "\x00" + linkKey(link.Target)} {
		for _, o := range v.targets[key] {
			if !seen[o] {
				seen[o] = true
				found = append(found, o)
			}
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Path < found[j].Path })
	status := "unresolved"
	switch {
	case len(found) == 1:
		status = "resolved"
	case len(found) > 1:
		status = "ambiguous"
	}
	return Resolved{Wikilink: link, Status: status, Matches: found}
}

// Resolve resolves a wikilink target.
func (v *Vault) Resolve(target string) Resolved {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.resolveLocked(format.Wikilink{Target: target, Label: target, SourceMarkdown: "[[" + target + "]]"})
}

func (v *Vault) resolvesToLocked(link format.Wikilink, id string) bool {
	r := v.resolveLocked(link)
	return r.Status == "resolved" && r.Matches[0].ID == id
}

func relationshipLinks(value any) []format.Wikilink {
	var out []format.Wikilink
	switch val := value.(type) {
	case string:
		out = append(out, format.Wikilinks(val)...)
	case []string:
		for _, entry := range val {
			out = append(out, format.Wikilinks(entry)...)
		}
	}
	return out
}

func (v *Vault) incomingLinkCountLocked(id string) int {
	count := 0
	for _, source := range v.sortedObjects() {
		if source.ID == id {
			continue
		}
		links := format.Wikilinks(source.Doc.Body())
		for _, value := range source.Relationships {
			links = append(links, relationshipLinks(value)...)
		}
		for _, link := range links {
			if v.resolvesToLocked(link, id) {
				count++
			}
		}
	}
	return count
}

// DeletionImpact reports how many links point at a note.
func (v *Vault) DeletionImpact(id string) (*Object, int, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	o, ok := v.byID[id]
	if !ok || o.Kind != KindNote {
		return nil, 0, ErrNotFound
	}
	return o, v.incomingLinkCountLocked(id), nil
}

// Connections is the outgoing, incoming, and relationship graph of a note.
type Connections struct {
	Outgoing      []Resolved
	Backlinks     []*Object
	Relationships []RelationshipLinks
	Inverse       []InverseLink
}

type RelationshipLinks struct {
	Definition model.Relationship
	Targets    []Resolved
}

type InverseLink struct {
	Key    string
	Source *Object
}

func (v *Vault) Connections(id string) (Connections, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	o, ok := v.byID[id]
	if !ok {
		return Connections{}, ErrNotFound
	}
	c := Connections{Outgoing: []Resolved{}, Backlinks: []*Object{}, Relationships: []RelationshipLinks{}, Inverse: []InverseLink{}}
	for _, link := range format.Wikilinks(o.Doc.Body()) {
		c.Outgoing = append(c.Outgoing, v.resolveLocked(link))
	}
	for _, source := range v.sortedObjects() {
		for _, link := range format.Wikilinks(source.Doc.Body()) {
			if v.resolvesToLocked(link, id) {
				c.Backlinks = append(c.Backlinks, source)
				break
			}
		}
		for _, def := range v.Model.Relationships {
			for _, link := range relationshipLinks(source.Relationships[def.Key]) {
				if v.resolvesToLocked(link, id) {
					c.Inverse = append(c.Inverse, InverseLink{Key: def.Inverse, Source: source})
					break
				}
			}
		}
	}
	for _, def := range v.Model.Relationships {
		value, ok := o.Relationships[def.Key]
		if !ok {
			continue
		}
		rl := RelationshipLinks{Definition: def, Targets: []Resolved{}}
		for _, link := range relationshipLinks(value) {
			rl.Targets = append(rl.Targets, v.resolveLocked(link))
		}
		c.Relationships = append(c.Relationships, rl)
	}
	return c, nil
}

// Suggestions returns link targets whose title or alias contains query.
func (v *Vault) Suggestions(query string) []*Object {
	v.mu.RLock()
	defer v.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	var out []*Object
	for _, o := range v.sortedObjects() {
		for _, candidate := range append([]string{displayTitle(o)}, o.Aliases...) {
			if query == "" || strings.Contains(strings.ToLower(candidate), query) {
				out = append(out, o)
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return displayTitle(out[i]) < displayTitle(out[j]) })
	return out
}

// SearchResult is one search hit with its object.
type SearchResult struct {
	Object  *Object
	Snippet string
}

// Search runs a lexical query through the Search index.
func (v *Vault) Search(query string) ([]SearchResult, error) {
	if v.index == nil {
		return nil, nil
	}
	hits, err := v.index.Search(query, 50)
	if err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	var out []SearchResult
	for _, hit := range hits {
		if o, ok := v.byPath[hit.Path]; ok {
			out = append(out, SearchResult{Object: o, Snippet: hit.Snippet})
		}
	}
	return out, nil
}
