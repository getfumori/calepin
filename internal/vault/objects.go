package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/getfumori/fumori/internal/format"
	"github.com/getfumori/fumori/internal/model"
)

type Kind string

const (
	KindNote  Kind = "standalone"
	KindDaily Kind = "daily"
)

const (
	notesDir = "human/notes"
	dailyDir = "human/daily"
)

// Object is one valid canonical Markdown file.
type Object struct {
	ID            string
	Kind          Kind
	Path          string
	Revision      string
	Source        string
	Doc           *format.Document
	Title         string
	Date          string
	Type          *string
	State         string
	Tags          []string
	Aliases       []string
	Created       string
	Properties    map[string]any
	Relationships map[string]any
	Blocks        []format.Block
	InlineTasks   []format.InlineTask
	Task          *format.PageTask
}

// Quarantined is a file that failed validation. It is readable and editable
// as raw Markdown but contributes nothing else.
type Quarantined struct {
	Path     string
	Revision string
	Source   string
	Problems []string
}

func revision(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	dailyPath   = regexp.MustCompile(`^human/daily/(\d{4})/(\d{4}-\d{2}-\d{2})\.md$`)
	notePath    = regexp.MustCompile(`^human/notes/[^/]+\.md$`)
	wikilinkVal = regexp.MustCompile(`^\[\[[^\]\n]+\]\]$`)
)

func dailyRelPath(date string) string { return path.Join(dailyDir, date[:4], date+".md") }

// decodeObject validates one file. It returns the object or the problems
// that quarantine it.
func decodeObject(rel, source string, m *model.Model) (*Object, []string) {
	var problems []string
	fail := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	doc, err := format.Parse(source)
	if err != nil {
		return nil, []string{err.Error()}
	}
	o := &Object{Path: rel, Source: source, Revision: revision(source), Doc: doc,
		Properties: map[string]any{}, Relationships: map[string]any{}, Tags: []string{}, Aliases: []string{}}

	wantSchema := "fumori.note"
	o.Kind = KindNote
	if match := dailyPath.FindStringSubmatch(rel); match != nil {
		o.Kind, o.Date, wantSchema = KindDaily, match[2], "fumori.daily-note"
		if !strings.HasPrefix(match[2], match[1]) {
			fail("daily note is in the wrong year folder")
		}
	} else if !notePath.MatchString(rel) {
		return nil, []string{"file is outside a known Human Notes folder"}
	}

	o.ID, _ = doc.String("_id")
	if !uuidPattern.MatchString(o.ID) {
		fail("_id must be a lowercase UUID")
	}
	if got, _ := doc.String("_schema"); got != wantSchema {
		fail("_schema must be %s", wantSchema)
	}
	if got, _ := doc.String("_version"); got != "1" {
		fail("_version must be 1")
	}
	o.Created, _ = doc.String("_created")
	if _, err := time.Parse(time.RFC3339, o.Created); err != nil {
		fail("_created must be an RFC 3339 time")
	}

	if typ, ok := doc.String("type"); ok {
		if m.Type(typ) == nil {
			fail("unknown type %q", typ)
		}
		o.Type = &typ
	}
	o.State, _ = doc.String("state")
	if !m.HasState(o.State) {
		fail("unknown state %q", o.State)
	}
	for key, target := range map[string]*[]string{"tags": &o.Tags, "aliases": &o.Aliases} {
		if _, err := doc.Decode(key, target); err != nil {
			fail("%s must be a list of strings", key)
		}
	}

	if o.Type != nil {
		for _, property := range m.Type(*o.Type).Properties {
			var value any
			if found, err := doc.Decode(property.Key, &value); err != nil || !found {
				if property.Required {
					fail("type property %q is required", property.Key)
				}
				continue
			}
			if !propertyMatches(property, value) {
				fail("type property %q does not match kind %s", property.Key, property.Kind)
			}
			o.Properties[property.Key] = normalizeValue(value)
		}
	}
	for _, rel := range m.Relationships {
		var value any
		if found, _ := doc.Decode(rel.Key, &value); !found {
			continue
		}
		if !relationshipMatches(rel, value) {
			fail("relationship %q must hold %s", rel.Key, map[string]string{"one": "one wikilink", "many": "a list of wikilinks"}[rel.Cardinality])
		}
		o.Relationships[rel.Key] = normalizeValue(value)
	}

	body := doc.Body()
	o.Title = format.Title(body)
	if o.Kind == KindDaily {
		if o.Type == nil || *o.Type != "daily-note" {
			fail("type must be daily-note")
		}
		if got, _ := doc.String("date"); got != o.Date {
			fail("date must be %s", o.Date)
		}
		if o.Title != o.Date {
			fail("the first heading must be '# %s'", o.Date)
		}
	} else if o.Title == "" {
		o.Title = "Untitled note"
	}

	var diags []format.Diagnostic
	o.Blocks, diags = format.Blocks(body)
	for _, d := range diags {
		fail("%s", d)
	}
	o.InlineTasks, diags = format.InlineTasks(body)
	for _, d := range diags {
		fail("%s", d)
	}
	if o.Task, err = doc.PageTask(); err != nil {
		fail("%v", err)
	}
	if o.Task != nil && o.Kind == KindDaily {
		fail("a Daily Note cannot be a Page Task")
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return o, nil
}

func normalizeValue(value any) any {
	if list, ok := value.([]any); ok {
		out := make([]string, 0, len(list))
		for _, entry := range list {
			s, _ := entry.(string)
			out = append(out, s)
		}
		return out
	}
	return value
}

func propertyMatches(p model.Property, value any) bool {
	switch p.Kind {
	case "text":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case int, float64:
			return true
		}
		return false
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "date":
		s, ok := value.(string)
		_, err := time.Parse(time.DateOnly, s)
		return ok && err == nil
	case "select":
		s, ok := value.(string)
		return ok && contains(p.Options, s)
	case "multi_select":
		list, ok := value.([]any)
		if !ok {
			return false
		}
		for _, entry := range list {
			s, ok := entry.(string)
			if !ok || !contains(p.Options, s) {
				return false
			}
		}
		return true
	}
	return false
}

func relationshipMatches(r model.Relationship, value any) bool {
	if r.Cardinality == "one" {
		s, ok := value.(string)
		return ok && wikilinkVal.MatchString(s)
	}
	list, ok := value.([]any)
	if !ok {
		return false
	}
	for _, entry := range list {
		s, ok := entry.(string)
		if !ok || !wikilinkVal.MatchString(s) {
			return false
		}
	}
	return true
}

func contains(list []string, s string) bool {
	for _, entry := range list {
		if entry == s {
			return true
		}
	}
	return false
}

func composeNote(frontmatter, body string) string {
	if body == "" {
		return frontmatter
	}
	return frontmatter + "\n" + strings.TrimRight(body, "\n") + "\n"
}

const historyHeadingLine = "## History"

// NoteParts splits a Human Note body into the free text the editor owns and
// the Occurrence records. The History heading that introduces the records is
// layout, so it belongs to neither part.
func NoteParts(o *Object) (free, records string) {
	free, records = format.SplitFreeform(o.Doc.Body(), o.Blocks)
	if records != "" {
		free = strings.TrimSuffix(strings.TrimRight(free, "\n"), historyHeadingLine)
	}
	return strings.Trim(free, "\n"), records
}

// composeNoteBody keeps Occurrence records at the end of the body under one
// History heading.
func composeNoteBody(free, records string) string {
	free = strings.Trim(free, "\n")
	if records == "" {
		return free
	}
	if free == "" {
		return historyHeadingLine + "\n\n" + records
	}
	return free + "\n\n" + historyHeadingLine + "\n\n" + records
}

// dailyParts splits a Daily Note body into the free-form text under the date
// heading and the Memo stream.
func dailyParts(o *Object) (free, stream string) {
	free, stream = format.SplitFreeform(o.Doc.Body(), o.Blocks)
	free = strings.TrimPrefix(free, "# "+o.Date)
	return strings.Trim(free, "\n"), stream
}

func composeDaily(date, free, stream string) string {
	head := "# " + date
	if free = strings.Trim(free, "\n"); free != "" {
		head += "\n\n" + free
	}
	return "\n" + format.JoinFreeform(head, stream)
}
