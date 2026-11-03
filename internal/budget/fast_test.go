package budget

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFastBudgetDeadlineAndLazyDone(t *testing.T) {
	b, err := NewFast(context.Background(), Limits{Timeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, ok := b.Context().Deadline(); !ok {
		t.Fatal("deadline missing")
	}
	select {
	case <-b.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("deadline channel never closed")
	}
	if !errors.Is(b.Check(), context.DeadlineExceeded) {
		t.Fatal(b.Check())
	}
}
func TestFastBudgetCancellationAndClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b, err := NewFast(ctx, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if !errors.Is(b.Check(), context.Canceled) {
		t.Fatal(b.Check())
	}
	b.Close()
	b, err = NewFast(context.Background(), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
	select {
	case <-b.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("closed scope did not cancel late Done")
	}
	if !errors.Is(b.Check(), context.Canceled) {
		t.Fatal(b.Check())
	}
}
func TestNestedFastBudgetCannotCancelParent(t *testing.T) {
	parent, _ := NewFast(context.Background(), Limits{})
	defer parent.Close()
	child, err := NewFast(parent.Context(), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	child.Close()
	if err := parent.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestFastBudgetStepsRemainExact(t *testing.T) {
	b, err := NewFast(context.Background(), Limits{MaxSteps: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for i := 0; i < 3; i++ {
		if err := b.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(b.Step(), ErrLimit) {
		t.Fatal("lost step limit")
	}
}

func TestFastBudgetCancellationBetweenChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b, err := NewFast(ctx, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	cancel()
	for i := 0; i < 32; i++ {
		if errors.Is(b.Step(), context.Canceled) {
			return
		}
	}
	t.Fatal("cancellation not observed within bounded work")
}

func TestFastContextTerminalErrorDoesNotChange(t *testing.T) {
	b, _ := NewFast(context.Background(), Limits{Timeout: time.Millisecond})
	b.Close()
	first := b.Context().Err()
	time.Sleep(2 * time.Millisecond)
	if first != context.Canceled || b.Context().Err() != first {
		t.Fatalf("terminal error changed: %v -> %v", first, b.Context().Err())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b, _ = NewFast(ctx, Limits{Timeout: time.Millisecond})
	defer b.Close()
	time.Sleep(2 * time.Millisecond)
	first = b.Context().Err()
	cancel()
	if first != context.DeadlineExceeded || b.Context().Err() != first {
		t.Fatalf("terminal error changed: %v -> %v", first, b.Context().Err())
	}
}

func TestFastContextDoneCannotExposeNilError(t *testing.T) {
	notified, release := make(chan struct{}), make(chan struct{})
	c := &deadlineContext{parent: context.Background(), deadline: time.Now().Add(time.Hour)}
	c.cancel = func() { close(notified); <-release }
	finished := make(chan struct{})
	go func() { c.finish(1); close(finished) }()
	<-notified
	got := make(chan error, 1)
	go func() { got <- c.Err() }()
	select {
	case err := <-got:
		close(release)
		<-finished
		if err == nil {
			t.Fatal("closed Done exposed nil Err")
		}
	case <-time.After(time.Millisecond):
		close(release)
		<-finished
		if err := <-got; err != context.Canceled {
			t.Fatal(err)
		}
	}
}
