package main

import (
	"context"
	"errors"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	js "github.com/mellowforge/jsontrans-go/javascript"
)

func TestSoakChecksEveryOutcome(t *testing.T) {
	report, err := run(context.Background(), settings{Iterations: 5, Workers: 1, Cycles: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"success", "exception", "conversion", "cancellation", "allocation"} {
		if got := report.Outcomes[kind].Calls; got != 1 {
			t.Errorf("%s: %d calls, want 1", kind, got)
		}
	}
	if report.Recoveries != 5 || len(report.Memory) != 2 || len(report.ConstructionNS) != 1 || len(report.CompilationNS) != 1 || len(report.FirstEvalNS) != 1 || len(report.CloseNS) != 1 {
		t.Fatalf("incomplete run: %+v", report)
	}
}

func TestSoakRejectsWrongFailureCauses(t *testing.T) {
	for kind := range 5 {
		if check(kind, jt.Result{}, nil) == nil || check(kind, jt.Result{}, errors.New("unrelated failure")) == nil {
			t.Fatalf("kind %d accepted missing/wrong outcome", kind)
		}
	}
	oom := &js.Exception{Name: "InternalError", Message: "out of memory"}
	for _, err := range []error{context.DeadlineExceeded, context.Canceled, errors.Join(oom, context.DeadlineExceeded), errors.Join(oom, context.Canceled), errors.Join(oom, js.ErrClosed)} {
		if check(4, jt.Result{}, err) == nil {
			t.Fatalf("timeout/cancellation passed allocation gate: %v", err)
		}
	}
	if check(2, jt.Result{}, &jt.Error{Phase: jt.PhaseOutput, Cause: errors.Join(jt.ErrConversion, jt.ErrLimit)}) == nil {
		t.Fatal("resource exhaustion passed cycle-conversion gate")
	}
	if check(3, jt.Result{}, errors.Join(context.DeadlineExceeded, js.ErrClosed)) == nil {
		t.Fatal("closure passed caller cancellation gate")
	}
}

func TestSoakRejectsUnboundedSettings(t *testing.T) {
	for _, s := range []settings{
		{0, 1, 1}, {1, 0, 1}, {1, 1, 0}, {100001, 1, 1},
		{1, 33, 1}, {1, 1, 101}, {100000, 32, 100},
	} {
		if _, err := run(context.Background(), s); err == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := run(ctx, settings{5, 1, 1}); err == nil {
		t.Fatal("ignored canceled run")
	}
}
