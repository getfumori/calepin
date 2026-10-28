package domain

import (
	"testing"

	"github.com/getfumori/fumori/internal/format"
)

func TestNextOccurrence(t *testing.T) {
	weeklySunday := format.Recurrence{Basis: format.BasisSchedule, Frequency: format.Weekly, Interval: 1, Weekdays: []string{"sun"}}
	cases := []struct {
		name             string
		rule             format.Recurrence
		original, action string
		want             string
	}{
		{"weekly done on time", weeklySunday, "2026-11-01", "2026-11-01", "2026-11-08"},
		{"weekly done a day late keeps Sunday", weeklySunday, "2026-11-01", "2026-11-02", "2026-11-08"},
		{"missed rounds collapse", weeklySunday, "2026-11-01", "2026-11-20", "2026-11-22"},
		{"done early", weeklySunday, "2026-11-08", "2026-11-05", "2026-11-15"},
		{"every two weeks", format.Recurrence{Basis: format.BasisSchedule, Frequency: format.Weekly, Interval: 2, Weekdays: []string{"sun"}}, "2026-11-01", "2026-11-01", "2026-11-15"},
		{"two weekdays", format.Recurrence{Basis: format.BasisSchedule, Frequency: format.Weekly, Interval: 1, Weekdays: []string{"mon", "thu"}}, "2026-11-02", "2026-11-02", "2026-11-05"},
		{"daily", format.Recurrence{Basis: format.BasisSchedule, Frequency: format.Daily, Interval: 3}, "2026-11-01", "2026-11-02", "2026-11-04"},
		{"monthly clamps to month end", format.Recurrence{Basis: format.BasisSchedule, Frequency: format.Monthly, Interval: 1}, "2026-01-31", "2026-01-31", "2026-02-28"},
		{"monthly returns to day 31 after a short month", format.Recurrence{Basis: format.BasisSchedule, Frequency: format.Monthly, Interval: 1, Day: 31}, "2026-02-28", "2026-02-28", "2026-03-31"},
		{"seven days after completion", format.Recurrence{Basis: format.BasisCompletion, Frequency: format.Daily, Interval: 7}, "2026-11-01", "2026-11-02", "2026-11-09"},
	}
	for _, c := range cases {
		if got := NextOccurrence(c.rule, c.original, c.action); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	if got := ResumeDate(weeklySunday, "2026-11-01", "2026-11-18"); got != "2026-11-22" {
		t.Errorf("resume: got %s", got)
	}
}
