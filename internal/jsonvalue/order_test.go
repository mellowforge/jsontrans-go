package jsonvalue

import (
	"context"
	"errors"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

func TestOrderProvenanceVisitStopsBeforeCopying(t *testing.T) {
	ctx := WithObjectOrder(context.Background())
	for i := 0; i < 100; i++ {
		RememberObject(ctx, map[string]any{"x": i}, []string{"x"})
	}
	b, err := budget.New(ctx, budget.Limits{MaxSteps: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.Step(); err != nil {
		t.Fatal(err)
	}
	visited := 0
	err = VisitObjectOrders(ctx, b, func(ObjectOrderEntry) error { visited++; return nil })
	if !errors.Is(err, budget.ErrLimit) || visited != 0 {
		t.Fatalf("visited=%d, err=%v", visited, err)
	}
}
