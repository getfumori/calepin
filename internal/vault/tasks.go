package vault

import (
	"sort"
	"strings"

	"github.com/getfumori/fumori/internal/domain"
	"github.com/getfumori/fumori/internal/format"
)

// DailyParts returns a Daily Note's free-form text and its Memo stream.
func DailyParts(o *Object) (free, stream string) { return dailyParts(o) }

// NewDailySource previews the Daily Note that the first write would create.
func (v *Vault) NewDailySource(date string) string { return v.newDailySource(date, "") }

// Capture appends a Memo to today's Daily Note, creating the note on first
// use. With task set, the Memo is one Inline Task in list, or an Unlisted
// Task when list is empty.
func (v *Vault) Capture(key, text string, task bool, list string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, invalidf("capture text is empty")
	}
	date := v.Today()
	if _, _, err := v.CreateDaily(date); err != nil {
		return nil, err
	}
	created := v.now()
	if list != "" && !wikilinkVal.MatchString(list) {
		return nil, invalidf("list must be a wikilink")
	}
	return v.command(key, "capture\x00"+text+boolString(task)+"\x00"+list, dailyRelPath(date), func(cur *Object) (string, error) {
		content := text
		if task {
			meta := []format.KV{{Key: "id", Value: v.newBlockID("t")}}
			if list != "" {
				meta = append(meta, format.KV{Key: "list", Value: list})
			}
			content = format.RenderInlineTask("", false, strings.ReplaceAll(text, "\n", " "), meta)
		}
		block := format.RenderBlock(format.KindMemo, []format.KV{{Key: "id", Value: v.newBlockID("m")}, {Key: "created", Value: created}}, content)
		return cur.Doc.WithBody(format.AppendBlock(cur.Doc.Body(), block)), nil
	}, func(o *Object) any { return map[string]string{"date": o.Date, "revision": o.Revision} })
}

func boolString(b bool) string {
	if b {
		return "\x00task"
	}
	return ""
}

// Comment appends a Memo comment after the Memo's existing comments, in the
// Memo's own Daily Note.
func (v *Vault) Comment(key, memoID, text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, invalidf("comment text is empty")
	}
	v.mu.RLock()
	rel, ok := v.blockOwner[memoID]
	v.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	created := v.now()
	return v.command(key, "comment\x00"+memoID+"\x00"+text, rel, func(cur *Object) (string, error) {
		insertAt := -1
		for _, b := range cur.Blocks {
			if (b.Kind == format.KindMemo && b.ID == memoID) || (b.Kind == format.KindComment && b.MetaString("parent") == memoID) {
				insertAt = b.End
			}
		}
		if insertAt < 0 || cur.Kind != KindDaily {
			return "", ErrNotFound
		}
		block := format.RenderBlock(format.KindComment, []format.KV{{Key: "id", Value: v.newBlockID("c")}, {Key: "parent", Value: memoID}, {Key: "created", Value: created}}, text)
		return cur.Doc.WithBody(format.InsertAt(cur.Doc.Body(), insertAt, block)), nil
	}, func(o *Object) any { return map[string]string{"date": o.Date, "revision": o.Revision} })
}

// TaskView is one Task as listed by Task views.
type TaskView struct {
	ID         string             `json:"id"`
	Kind       string             `json:"kind"`
	Title      string             `json:"title"`
	Status     format.TaskStatus  `json:"status"`
	List       string             `json:"list,omitempty"`
	Scheduled  string             `json:"scheduled,omitempty"`
	Due        string             `json:"due,omitempty"`
	Recurring  bool               `json:"recurring"`
	Occurrence *format.Occurrence `json:"occurrence,omitempty"`
	Recurrence *format.Recurrence `json:"recurrence,omitempty"`
	Revision   string             `json:"revision"`
	Source     map[string]string  `json:"source"`
	Overdue    bool               `json:"overdue"`
}

// Tasks lists Tasks. list filters by Task List title; "unlisted" selects
// Unlisted Tasks; status "" means open and paused.
func (v *Vault) Tasks(list, status string) []TaskView {
	v.mu.RLock()
	defer v.mu.RUnlock()
	today := v.Today()
	var out []TaskView
	for _, o := range v.sortedObjects() {
		source := map[string]string{"id": o.ID, "title": displayTitle(o), "url": URL(o)}
		if t := o.Task; t != nil {
			view := TaskView{ID: o.ID, Kind: "page", Title: o.Title, Status: t.Status, List: t.List, Scheduled: t.Scheduled,
				Due: t.Due, Recurring: t.Recurring(), Occurrence: t.Occurrence, Recurrence: t.Recurrence, Revision: o.Revision, Source: source}
			if t.Occurrence != nil {
				view.Scheduled = t.Occurrence.Scheduled
			}
			out = append(out, view)
		}
		for _, it := range o.InlineTasks {
			st := format.StatusOpen
			if it.Checked {
				st = format.StatusDone
			}
			listRef, _ := it.Meta["list"].(string)
			scheduled, _ := it.Meta["scheduled"].(string)
			due, _ := it.Meta["due"].(string)
			out = append(out, TaskView{ID: it.ID, Kind: "inline", Title: it.Text, Status: st, List: listRef,
				Scheduled: scheduled, Due: due, Revision: o.Revision, Source: source})
		}
	}
	filtered := out[:0]
	for _, t := range out {
		t.Overdue = t.Status == format.StatusOpen && ((t.Scheduled != "" && t.Scheduled < today) || (t.Due != "" && t.Due < today))
		switch status {
		case "":
			if t.Status != format.StatusOpen && t.Status != format.StatusPaused {
				continue
			}
		case "all":
		default:
			if string(t.Status) != status {
				continue
			}
		}
		switch {
		case list == "unlisted" && t.List != "":
			continue
		case list != "" && list != "unlisted" && !strings.EqualFold(strings.Trim(t.List, "[]"), list):
			continue
		}
		filtered = append(filtered, t)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i].Scheduled, filtered[j].Scheduled
		if (a == "") != (b == "") {
			return a != ""
		}
		return a < b
	})
	return filtered
}

// TaskCommand is one Task action from a client.
type TaskCommand struct {
	TaskID             string
	Action             string
	ExpectedOccurrence string
	BaseRevision       string
	IdempotencyKey     string
	Fingerprint        string
	Notes              string
	Scheduled          *string
	Due                *string
	List               *string
	Recurrence         *format.Recurrence
}

// TaskAction applies a command to a Page Task (by note ID) or an Inline Task
// (by block ID).
func (v *Vault) TaskAction(cmd TaskCommand) ([]byte, error) {
	v.mu.RLock()
	rel := ""
	if o, ok := v.byID[cmd.TaskID]; ok {
		rel = o.Path
	} else if owner, ok := v.blockOwner[cmd.TaskID]; ok {
		rel = owner
	}
	v.mu.RUnlock()
	if rel == "" {
		return nil, ErrNotFound
	}
	return v.command(cmd.IdempotencyKey, cmd.Fingerprint, rel, func(cur *Object) (string, error) {
		if cmd.BaseRevision != "" && cmd.BaseRevision != cur.Revision {
			return "", &StaleError{Current: cur.Revision}
		}
		if cur.ID == cmd.TaskID {
			return v.applyPageTask(cur, cmd)
		}
		return v.applyInlineTask(cur, cmd)
	}, func(o *Object) any { return map[string]string{"id": o.ID, "revision": o.Revision} })
}

func (v *Vault) applyPageTask(cur *Object, cmd TaskCommand) (string, error) {
	t := cur.Task
	switch {
	case cmd.Action == "make":
		return v.makePageTask(cur, cmd)
	case t == nil:
		return "", invalidf("note is not a task")
	case cmd.Action == "remove":
		return cur.Doc.Delete("task"), nil
	}
	task := *t
	today := v.Today()
	switch cmd.Action {
	case "complete", "skip":
		if !task.Recurring() {
			if cmd.Action == "skip" {
				return "", invalidf("only recurring tasks can be skipped")
			}
			task.Status, task.Completed = format.StatusDone, v.now()
			return cur.Doc.SetPageTask(task)
		}
		if err := v.expectOccurrence(cur, task, cmd.ExpectedOccurrence); err != nil {
			if err == errAlreadyRecorded {
				return cur.Source, nil
			}
			return "", err
		}
		occ := *task.Occurrence
		outcome := map[string]string{"complete": "completed", "skip": "skipped"}[cmd.Action]
		content := "### " + today
		if notes := strings.TrimSpace(cmd.Notes); notes != "" {
			content += "\n\n" + notes
		}
		record := format.RenderBlock(format.KindOccurrence, []format.KV{
			{Key: "id", Value: occ.ID}, {Key: "original", Value: occ.Original}, {Key: "scheduled", Value: occ.Scheduled},
			{Key: "outcome", Value: outcome}, {Key: "at", Value: v.now()},
		}, content)
		next := domain.NextOccurrence(*task.Recurrence, occ.Original, today)
		task.Occurrence = &format.Occurrence{ID: v.newBlockID("o"), Original: next, Scheduled: next}
		free, records := NoteParts(cur)
		if records != "" {
			records = strings.TrimRight(records, "\n") + "\n\n"
		}
		doc, err := v.parseWithBody(cur, "\n"+composeNoteBody(free, records+record))
		if err != nil {
			return "", err
		}
		return doc.SetPageTask(task)
	case "undo":
		return v.undoOccurrence(cur, task, cmd)
	case "cancel":
		task.Status = format.StatusCanceled
	case "reopen":
		task.Status, task.Completed = format.StatusOpen, ""
	case "pause":
		if !task.Recurring() {
			return "", invalidf("only recurring tasks can be paused")
		}
		task.Status = format.StatusPaused
	case "resume":
		if task.Status != format.StatusPaused {
			return "", &ConflictError{Message: "task is not paused"}
		}
		task.Status = format.StatusOpen
		task.Occurrence.Scheduled = domain.ResumeDate(*task.Recurrence, task.Occurrence.Original, today)
	case "update":
		if cmd.Scheduled != nil {
			if task.Occurrence != nil {
				task.Occurrence.Scheduled = *cmd.Scheduled
			} else {
				task.Scheduled = *cmd.Scheduled
			}
		}
		if cmd.Due != nil {
			task.Due = *cmd.Due
		}
		if cmd.List != nil {
			task.List = *cmd.List
		}
	default:
		return "", invalidf("unknown task action %q", cmd.Action)
	}
	return cur.Doc.SetPageTask(task)
}

func (v *Vault) makePageTask(cur *Object, cmd TaskCommand) (string, error) {
	if cur.Task != nil {
		return "", &ConflictError{Message: "note is already a task"}
	}
	if cur.Kind != KindNote {
		return "", invalidf("only Human Notes can become Page Tasks")
	}
	task := format.PageTask{Status: format.StatusOpen}
	if cmd.Scheduled != nil {
		task.Scheduled = *cmd.Scheduled
	}
	if cmd.Due != nil {
		task.Due = *cmd.Due
	}
	if cmd.List != nil {
		task.List = *cmd.List
	}
	if r := cmd.Recurrence; r != nil {
		if task.Scheduled == "" {
			return "", invalidf("a recurring task needs a first scheduled date")
		}
		task.Recurrence = r
		task.Timezone = v.Manifest.Timezone.String()
		task.Occurrence = &format.Occurrence{ID: v.newBlockID("o"), Original: task.Scheduled, Scheduled: task.Scheduled}
		task.Scheduled = ""
	}
	if err := task.Validate(); err != nil {
		return "", invalidf("%v", err)
	}
	return cur.Doc.SetPageTask(task)
}

var errAlreadyRecorded = &ConflictError{Message: "occurrence already recorded"}

// expectOccurrence checks that the request targets the current Occurrence.
// A request for an Occurrence that already has a record is a retry and
// succeeds without writing again.
func (v *Vault) expectOccurrence(cur *Object, task format.PageTask, expected string) error {
	if task.Status != format.StatusOpen {
		return &ConflictError{Message: "task is not open"}
	}
	if expected == task.Occurrence.ID {
		return nil
	}
	for _, b := range cur.Blocks {
		if b.Kind == format.KindOccurrence && b.ID == expected {
			return errAlreadyRecorded
		}
	}
	return &ConflictError{Message: "expected occurrence is not current"}
}

func (v *Vault) parseWithBody(cur *Object, body string) (*format.Document, error) {
	return format.Parse(cur.Doc.WithBody(body))
}

func (v *Vault) undoOccurrence(cur *Object, task format.PageTask, cmd TaskCommand) (string, error) {
	if !task.Recurring() || task.Occurrence == nil {
		return "", invalidf("only recurring tasks can be undone")
	}
	if cmd.ExpectedOccurrence != task.Occurrence.ID {
		return "", &ConflictError{Message: "expected occurrence is not current"}
	}
	if task.Occurrence.Scheduled != task.Occurrence.Original {
		return "", &ConflictError{Message: "the next occurrence was already changed"}
	}
	var last *format.Block
	for i := range cur.Blocks {
		if cur.Blocks[i].Kind == format.KindOccurrence {
			last = &cur.Blocks[i]
		}
	}
	if last == nil {
		return "", &ConflictError{Message: "no occurrence record to undo"}
	}
	body := cur.Doc.Body()
	free, _ := NoteParts(cur)
	var kept []string
	for _, b := range cur.Blocks {
		if b.Start != last.Start {
			kept = append(kept, body[b.Start:b.End])
		}
	}
	trimmed := "\n" + composeNoteBody(free, strings.Join(kept, "\n"))
	scheduled := last.MetaString("scheduled")
	if scheduled == "" {
		scheduled = last.MetaString("original")
	}
	task.Occurrence = &format.Occurrence{ID: last.ID, Original: last.MetaString("original"), Scheduled: scheduled}
	doc, err := v.parseWithBody(cur, trimmed)
	if err != nil {
		return "", err
	}
	return doc.SetPageTask(task)
}

func (v *Vault) applyInlineTask(cur *Object, cmd TaskCommand) (string, error) {
	id := cmd.TaskID
	for _, it := range cur.InlineTasks {
		if it.ID != id {
			continue
		}
		body := cur.Doc.Body()
		line := body[it.Start:it.End]
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		meta := []format.KV{{Key: "id", Value: id}}
		checked := it.Checked
		for _, key := range []string{"list", "scheduled", "due"} {
			value, _ := it.Meta[key].(string)
			if key == "list" && cmd.List != nil {
				value = *cmd.List
			}
			if key == "scheduled" && cmd.Scheduled != nil {
				value = *cmd.Scheduled
			}
			if key == "due" && cmd.Due != nil {
				value = *cmd.Due
			}
			if value != "" {
				meta = append(meta, format.KV{Key: key, Value: value})
			}
		}
		switch cmd.Action {
		case "complete":
			checked = true
			meta = append(meta, format.KV{Key: "completed", Value: v.now()})
		case "reopen":
			checked = false
		case "update":
			if c, _ := it.Meta["completed"].(string); c != "" && checked {
				meta = append(meta, format.KV{Key: "completed", Value: c})
			}
		default:
			return "", invalidf("inline tasks support complete, reopen, and update")
		}
		return cur.Doc.WithBody(format.ReplaceRange(body, it.Start, it.End, format.RenderInlineTask(indent, checked, it.Text, meta))), nil
	}
	return "", ErrNotFound
}
