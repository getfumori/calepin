package format

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type TaskStatus string

const (
	StatusOpen     TaskStatus = "open"
	StatusPaused   TaskStatus = "paused"
	StatusDone     TaskStatus = "done"
	StatusCanceled TaskStatus = "canceled"
)

type Basis string

const (
	BasisSchedule   Basis = "schedule"
	BasisCompletion Basis = "completion"
)

type Frequency string

const (
	Daily   Frequency = "daily"
	Weekly  Frequency = "weekly"
	Monthly Frequency = "monthly"
)

// Recurrence is the rule that schedules the next Occurrence.
type Recurrence struct {
	Basis     Basis     `yaml:"basis" json:"basis"`
	Frequency Frequency `yaml:"frequency" json:"frequency"`
	Interval  int       `yaml:"interval" json:"interval"`
	Weekdays  []string  `yaml:"weekdays,omitempty" json:"weekdays,omitempty"`
	// Day is the monthly day of month; a short month clamps to its last day.
	Day int `yaml:"day,omitempty" json:"day,omitempty"`
}

// Occurrence is the current round of a recurring Task.
type Occurrence struct {
	ID        string `yaml:"id" json:"id"`
	Original  string `yaml:"original" json:"original"`
	Scheduled string `yaml:"scheduled" json:"scheduled"`
}

// PageTask is the `task` property of a Human Note.
type PageTask struct {
	Status     TaskStatus  `yaml:"status" json:"status"`
	List       string      `yaml:"list,omitempty" json:"list,omitempty"`
	Scheduled  string      `yaml:"scheduled,omitempty" json:"scheduled,omitempty"`
	Due        string      `yaml:"due,omitempty" json:"due,omitempty"`
	Completed  string      `yaml:"completed,omitempty" json:"completed,omitempty"`
	Timezone   string      `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	Occurrence *Occurrence `yaml:"occurrence,omitempty" json:"occurrence,omitempty"`
	Recurrence *Recurrence `yaml:"recurrence,omitempty" json:"recurrence,omitempty"`
}

func (t PageTask) Recurring() bool { return t.Recurrence != nil }

var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// Weekday parses a weekday name used in recurrence rules.
func Weekday(name string) (time.Weekday, bool) {
	day, ok := weekdayNames[name]
	return day, ok
}

func validDate(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

// PageTask decodes and validates the `task` property, or returns nil when the
// note is not a Page Task.
func (d *Document) PageTask() (*PageTask, error) {
	var task PageTask
	found, err := d.Decode("task", &task)
	if err != nil || !found {
		return nil, err
	}
	if err := task.Validate(); err != nil {
		return nil, fmt.Errorf("task: %w", err)
	}
	return &task, nil
}

// Validate checks the Task state machine invariants.
func (t PageTask) Validate() error {
	switch t.Status {
	case StatusOpen, StatusDone, StatusCanceled:
	case StatusPaused:
		if !t.Recurring() {
			return fmt.Errorf("only recurring tasks can be paused")
		}
	default:
		return fmt.Errorf("unknown status %q", t.Status)
	}
	for name, value := range map[string]string{"scheduled": t.Scheduled, "due": t.Due} {
		if value != "" && !validDate(value) {
			return fmt.Errorf("%s must be a YYYY-MM-DD date", name)
		}
	}
	if t.Completed != "" {
		if _, err := time.Parse(time.RFC3339, t.Completed); err != nil {
			return fmt.Errorf("completed must be an RFC 3339 time")
		}
	}
	if t.Timezone != "" {
		if _, err := time.LoadLocation(t.Timezone); err != nil {
			return fmt.Errorf("unknown timezone %q", t.Timezone)
		}
	}
	if t.List != "" && !strings.HasPrefix(t.List, "[[") {
		return fmt.Errorf("list must be a wikilink")
	}
	if !t.Recurring() {
		if t.Occurrence != nil {
			return fmt.Errorf("occurrence requires recurrence")
		}
		return nil
	}
	r := t.Recurrence
	if r.Basis != BasisSchedule && r.Basis != BasisCompletion {
		return fmt.Errorf("recurrence basis must be schedule or completion")
	}
	if r.Frequency != Daily && r.Frequency != Weekly && r.Frequency != Monthly {
		return fmt.Errorf("recurrence frequency must be daily, weekly, or monthly")
	}
	if r.Interval < 1 {
		return fmt.Errorf("recurrence interval must be at least 1")
	}
	if r.Day < 0 || r.Day > 31 || (r.Day > 0 && r.Frequency != Monthly) {
		return fmt.Errorf("recurrence day must be 1-31 and only for monthly rules")
	}
	for _, day := range r.Weekdays {
		if _, ok := Weekday(day); !ok {
			return fmt.Errorf("unknown weekday %q", day)
		}
	}
	if t.Due != "" {
		return fmt.Errorf("recurring tasks have no due date")
	}
	if t.Status == StatusOpen || t.Status == StatusPaused {
		o := t.Occurrence
		if o == nil || o.ID == "" || !validDate(o.Original) || !validDate(o.Scheduled) {
			return fmt.Errorf("an active recurring task needs occurrence id, original, and scheduled dates")
		}
	}
	return nil
}

// yamlTask mirrors PageTask with quoted date and time values for writing.
type yamlTask struct {
	Status     TaskStatus      `yaml:"status"`
	List       Quoted          `yaml:"list,omitempty"`
	Scheduled  Quoted          `yaml:"scheduled,omitempty"`
	Due        Quoted          `yaml:"due,omitempty"`
	Completed  Quoted          `yaml:"completed,omitempty"`
	Timezone   string          `yaml:"timezone,omitempty"`
	Occurrence *yamlOccurrence `yaml:"occurrence,omitempty"`
	Recurrence *Recurrence     `yaml:"recurrence,omitempty"`
}

type yamlOccurrence struct {
	ID        string `yaml:"id"`
	Original  Quoted `yaml:"original"`
	Scheduled Quoted `yaml:"scheduled"`
}

// SetPageTask returns the source with the `task` property replaced.
func (d *Document) SetPageTask(t PageTask) (string, error) {
	out := yamlTask{
		Status: t.Status, List: Quoted(t.List), Scheduled: Quoted(t.Scheduled),
		Due: Quoted(t.Due), Completed: Quoted(t.Completed), Timezone: t.Timezone,
		Recurrence: t.Recurrence,
	}
	if t.Occurrence != nil {
		out.Occurrence = &yamlOccurrence{t.Occurrence.ID, Quoted(t.Occurrence.Original), Quoted(t.Occurrence.Scheduled)}
	}
	return d.Set("task", out)
}

// InlineTask is an explicitly marked checkbox.
type InlineTask struct {
	ID         string
	Checked    bool
	Text       string
	Meta       map[string]any
	Line       int
	Start, End int
}

var inlineTaskLine = regexp.MustCompile(`^(\s*[-*+] \[)([ xX])(\] )(.*?)\s*<!-- calepin:task (\{.*\}) -->\s*$`)

// InlineTasks returns marked checkboxes outside code.
func InlineTasks(body string) ([]InlineTask, []Diagnostic) {
	var tasks []InlineTask
	var diags []Diagnostic
	for i, l := range scanLines(body) {
		if l.inFence || l.isFence || !strings.Contains(l.text, "<!-- calepin:task ") {
			continue
		}
		m := inlineTaskLine.FindStringSubmatch(l.text)
		if m == nil {
			diags = append(diags, Diagnostic{i + 1, "task marker must end a checkbox list item"})
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal([]byte(m[5]), &meta); err != nil {
			diags = append(diags, Diagnostic{i + 1, "task metadata is not valid JSON"})
			continue
		}
		id, _ := meta["id"].(string)
		if id == "" {
			diags = append(diags, Diagnostic{i + 1, "task metadata has no id"})
			continue
		}
		tasks = append(tasks, InlineTask{
			ID: id, Checked: m[2] != " ", Text: m[4], Meta: meta,
			Line: i + 1, Start: l.start, End: l.end,
		})
	}
	return tasks, diags
}

// RenderInlineTask writes one Inline Task list item line.
func RenderInlineTask(indent string, checked bool, text string, meta []KV) string {
	box := " "
	if checked {
		box = "x"
	}
	return fmt.Sprintf("%s- [%s] %s <!-- calepin:task %s -->", indent, box, text, encodeMeta(meta))
}

// ReplaceRange replaces body[start:end].
func ReplaceRange(body string, start, end int, text string) string {
	return body[:start] + text + body[end:]
}
