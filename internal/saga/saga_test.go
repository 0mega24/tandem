package saga

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// recorder builds steps that log what ran, and fail when told to.
type recorder struct {
	log []string
}

func (r *recorder) step(name string, doErr, undoErr error) Step {
	return Step{
		Name: name,
		Do: func(context.Context) error {
			r.log = append(r.log, "do "+name)
			return doErr
		},
		Undo: func(context.Context) error {
			r.log = append(r.log, "undo "+name)
			return undoErr
		},
	}
}

func TestRunAllSucceed(t *testing.T) {
	var r recorder
	err := Run(context.Background(), []Step{r.step("a", nil, nil), r.step("b", nil, nil)})
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"do a", "do b"}; !reflect.DeepEqual(r.log, want) {
		t.Errorf("log = %v, want %v", r.log, want)
	}
}

func TestRunRollsBackInReverse(t *testing.T) {
	var r recorder
	boom := errors.New("boom")
	steps := []Step{
		r.step("a", nil, nil),
		{Name: "no undo", Do: func(context.Context) error { r.log = append(r.log, "do no undo"); return nil }},
		r.step("b", nil, nil),
		r.step("c", boom, nil),
		r.step("d", nil, nil),
	}

	err := Run(context.Background(), steps)

	want := []string{"do a", "do no undo", "do b", "do c", "undo b", "undo a"}
	if !reflect.DeepEqual(r.log, want) {
		t.Errorf("log = %v, want %v (the failed step is not undone, d never runs)", r.log, want)
	}
	if !errors.Is(err, boom) || !IsRolledBack(err) {
		t.Errorf("Run() = %v, want the step error with a clean rollback", err)
	}
	if !strings.Contains(err.Error(), "c: boom (rolled back)") {
		t.Errorf("Run() message = %q, want the step name and \"rolled back\"", err)
	}
}

func TestUndoErrorsAreCollected(t *testing.T) {
	var r recorder
	steps := []Step{
		r.step("a", nil, errors.New("a stuck")),
		r.step("b", nil, errors.New("b stuck")),
		r.step("c", errors.New("boom"), nil),
	}

	err := Run(context.Background(), steps)

	if want := []string{"do a", "do b", "do c", "undo b", "undo a"}; !reflect.DeepEqual(r.log, want) {
		t.Errorf("log = %v, want %v (an undo error does not stop the others)", r.log, want)
	}

	e, ok := errors.AsType[*Error](err)
	if !ok || len(e.UndoErrs) != 2 || IsRolledBack(err) {
		t.Fatalf("Run() = %v, want an *Error with two undo errors", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "undo b: b stuck; undo a: a stuck") {
		t.Errorf("Run() message = %q, want both undo errors in reverse order", msg)
	}
}

func TestUndoRunsAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var undoCtxErr error
	steps := []Step{
		{
			Name: "a",
			Do:   func(context.Context) error { return nil },
			Undo: func(ctx context.Context) error { undoCtxErr = ctx.Err(); return nil },
		},
		{
			Name: "b",
			Do: func(ctx context.Context) error {
				cancel() // Ctrl-C during the step
				return ctx.Err()
			},
		},
	}

	if err := Run(ctx, steps); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want context.Canceled", err)
	}
	if undoCtxErr != nil {
		t.Errorf("undo context error = %v, want nil: the rollback must not be cancelled", undoCtxErr)
	}
}
