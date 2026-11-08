package document

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

func TestMaterializeSelectedNodeBoundaries(t *testing.T) {
	ctx := context.Background()
	d, err := Parse(ctx, []byte(`{"take":{"\ud800":["\udfff",1e999]},"unused":[[null,null,null]]}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := d.Root().Member("take")
	b, _ := budget.New(ctx, budget.Limits{})
	v, err := MaterializeNodeWithBudget(n, b)
	b.Close()
	if err != nil {
		t.Fatal(err)
	}
	b, _ = budget.New(ctx, budget.Limits{})
	root, err := MaterializeWithBudget(d, b)
	b.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, root.(map[string]any)["take"]) {
		t.Fatalf("subtree %#v != %#v", v, root)
	}
	for _, l := range []budget.Limits{{MaxInputBytes: 4}, {MaxDepth: 3}, {MaxCollectionItems: 2}, {MaxSteps: 1}} {
		b, _ := budget.New(ctx, l)
		_, err := MaterializeNodeWithBudget(n, b)
		b.Close()
		if !errors.Is(err, budget.ErrLimit) {
			t.Fatalf("%+v: %v", l, err)
		}
	}
	cctx, cancel := context.WithCancel(ctx)
	b, _ = budget.New(cctx, budget.Limits{})
	cancel()
	_, err = MaterializeNodeWithBudget(n, b)
	b.Close()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	b, _ = budget.New(ctx, budget.Limits{})
	_, err = MaterializeNodeWithBudget(Node{}, b)
	b.Close()
	if !errors.Is(err, jsonvalue.ErrInvalid) {
		t.Fatalf("invalid node: %v", err)
	}
	if _, err := MaterializeNodeWithBudget(n, nil); !errors.Is(err, jsonvalue.ErrInvalid) {
		t.Fatalf("nil budget: %v", err)
	}
}

func TestMaterializeSelectedNodeOwnsStorage(t *testing.T) {
	ctx := context.Background()
	text := strings.Repeat("selected-", 512)
	d, err := Parse(ctx, []byte(`{"take":{"text":"`+text+`"},"unused":"padding"}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := d.Root().Member("take")
	b, _ := budget.New(ctx, budget.Limits{})
	v, err := MaterializeNodeWithBudget(n, b)
	b.Close()
	if err != nil {
		t.Fatal(err)
	}
	v.(map[string]any)["text"] = "modified"
	b, _ = budget.New(ctx, budget.Limits{})
	again, err := MaterializeNodeWithBudget(n, b)
	b.Close()
	if err != nil || again.(map[string]any)["text"] != text {
		t.Fatalf("aliased result: %#v %v", again, err)
	}
}
