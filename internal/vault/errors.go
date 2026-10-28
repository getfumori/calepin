package vault

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrExplicitCreation = errors.New("explicit creation required")
)

// StaleError means the base revision is not current. Current is "" when the
// object does not exist.
type StaleError struct{ Current string }

func (e *StaleError) Error() string { return "stale revision" }

// InvalidError means the result would not be valid canonical Markdown.
type InvalidError struct{ Message string }

func (e *InvalidError) Error() string { return e.Message }

func invalidf(format string, args ...any) error {
	return &InvalidError{Message: fmt.Sprintf(format, args...)}
}

// DeletionImpactError means the incoming link count changed since the
// owner confirmed a delete.
type DeletionImpactError struct{ Count int }

func (e *DeletionImpactError) Error() string { return "deletion impact changed" }

// ConflictError means a command no longer applies, such as completing an
// Occurrence that is not current or reusing an idempotency key.
type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }
