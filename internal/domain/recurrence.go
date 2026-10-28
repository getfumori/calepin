// Package domain holds pure business rules with no I/O.
package domain

import (
	"time"

	"github.com/getfumori/fumori/internal/format"
)

func date(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func addMonthsClamped(t time.Time, months, day int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(day, last), 0, 0, 0, 0, time.UTC)
}

func weekStart(t time.Time) time.Time { return t.AddDate(0, 0, -int(t.Weekday())) }

// onRule reports whether d is a date of the schedule anchored at original.
func onRule(r format.Recurrence, original, d time.Time) bool {
	switch r.Frequency {
	case format.Daily:
		return int(d.Sub(original).Hours()/24)%r.Interval == 0
	case format.Weekly:
		weeks := int(weekStart(d).Sub(weekStart(original)).Hours() / (24 * 7))
		if weeks%r.Interval != 0 {
			return false
		}
		days := r.Weekdays
		if len(days) == 0 {
			return d.Weekday() == original.Weekday()
		}
		for _, name := range days {
			if wd, _ := format.Weekday(name); wd == d.Weekday() {
				return true
			}
		}
		return false
	}
	return false
}

// firstOnOrAfter is the first schedule date at or after from.
func firstOnOrAfter(r format.Recurrence, original, from time.Time) time.Time {
	if r.Frequency == format.Monthly {
		day := r.Day
		if day == 0 {
			day = original.Day()
		}
		for k := 0; ; k += r.Interval {
			if d := addMonthsClamped(original, k, day); !d.Before(from) {
				return d
			}
		}
	}
	d := from
	if d.Before(original) {
		d = original
	}
	for !onRule(r, original, d) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// NextOccurrence returns the date of the Occurrence after one that was
// completed or skipped on actionDate. Missed rounds collapse: a schedule
// rule continues from the later of the original date and the action date.
func NextOccurrence(r format.Recurrence, original, actionDate string) string {
	o, a := date(original), date(actionDate)
	if r.Basis == format.BasisCompletion {
		switch r.Frequency {
		case format.Daily:
			return a.AddDate(0, 0, r.Interval).Format(time.DateOnly)
		case format.Weekly:
			return a.AddDate(0, 0, 7*r.Interval).Format(time.DateOnly)
		default:
			return addMonthsClamped(a, r.Interval, a.Day()).Format(time.DateOnly)
		}
	}
	after := o
	if a.After(after) {
		after = a
	}
	return firstOnOrAfter(r, o, after.AddDate(0, 0, 1)).Format(time.DateOnly)
}

// ResumeDate returns the scheduled date of the current Occurrence when a
// paused Task resumes on today.
func ResumeDate(r format.Recurrence, original, today string) string {
	if r.Basis == format.BasisCompletion {
		return today
	}
	return firstOnOrAfter(r, date(original), date(today)).Format(time.DateOnly)
}
