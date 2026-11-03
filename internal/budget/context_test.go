package budget

import (
	"context"
	"errors"
	"testing"
)

func TestSetContextPreservesCountersDeadlineAndOwnership(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxSteps = 2
	limits.MaxDepth = 1
	var b Budget
	if err := InitFastResolved(&b, context.Background(), limits); err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.Enter(); err != nil {
		t.Fatal(err)
	}
	deadline, _ := b.Context().Deadline()
	type key struct{}
	b.SetContext(context.WithValue(b.Context(), key{}, true))
	if got, _ := b.Context().Deadline(); got != deadline || b.Context().Value(key{}) != true {
		t.Fatal("lost context metadata or deadline")
	}
	// Enter's depth and work remain charged across the metadata replacement.
	if err := b.Enter(); !errors.Is(err, ErrLimit) {
		t.Fatalf("depth: %v", err)
	}
	b.Leave()
	if err := b.Step(); !errors.Is(err, ErrLimit) {
		t.Fatalf("steps: %v", err)
	}
	b.Close()
	if !errors.Is(b.Context().Err(), context.Canceled) {
		t.Fatal("lost Close ownership")
	}
}
