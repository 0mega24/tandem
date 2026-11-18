// Package saga runs a list of steps and undoes the finished ones when a later step fails, so a
// failed command leaves the disk as it was.
package saga

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Step is one change with its inverse.
//
// Do must clean up its own partial work when it fails: Undo runs only for steps that finished.
type Step struct {
	Name string
	Do   func(ctx context.Context) error
	Undo func(ctx context.Context) error // nil when there is nothing to undo
}

// Error is a failed step and the errors of the rollback that followed it.
type Error struct {
	Step     string
	Err      error
	UndoErrs []error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s: %v", e.Step, e.Err)
	if len(e.UndoErrs) == 0 {
		return msg + " (rolled back)"
	}

	parts := make([]string, len(e.UndoErrs))
	for i, err := range e.UndoErrs {
		parts[i] = err.Error()
	}

	return msg + "; rollback incomplete: " + strings.Join(parts, "; ")
}

func (e *Error) Unwrap() error { return e.Err }

// Run runs the steps in order. When one fails, it undoes the finished steps in reverse order and
// returns an *Error. An undo error does not stop the other undos.
func Run(ctx context.Context, steps []Step) error {
	for i, s := range steps {
		if err := s.Do(ctx); err != nil {
			return &Error{Step: s.Name, Err: err, UndoErrs: undo(ctx, steps[:i])}
		}
	}

	return nil
}

func undo(ctx context.Context, done []Step) []error {
	// Ctrl-C cancels ctx, and that is often the reason for the rollback, so the undos must
	// still be able to run git.
	ctx = context.WithoutCancel(ctx)

	var errs []error
	for _, s := range slices.Backward(done) {
		if s.Undo == nil {
			continue
		}

		if err := s.Undo(ctx); err != nil {
			errs = append(errs, fmt.Errorf("undo %s: %w", s.Name, err))
		}
	}

	return errs
}

// IsRolledBack reports whether err is a step failure whose rollback finished without errors.
func IsRolledBack(err error) bool {
	e, ok := errors.AsType[*Error](err)
	return ok && len(e.UndoErrs) == 0
}
