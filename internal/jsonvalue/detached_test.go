package jsonvalue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unsafe"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

func TestCloneDetachedOwnsStrings(t *testing.T) {
	s := strings.Repeat("x", 1000)
	n := json.Number(strings.Repeat("1", 1000))
	b, _ := budget.New(context.Background(), budget.DefaultLimits())
	defer b.Close()
	v, err := CloneDetached(map[string]any{s: []any{s, n}}, b, b.Limits.MaxOutputBytes)
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	for k, a := range m {
		if unsafe.StringData(k) == unsafe.StringData(s) {
			t.Fatal("key retained source")
		}
		x := a.([]any)
		if unsafe.StringData(x[0].(string)) == unsafe.StringData(s) {
			t.Fatal("string retained source")
		}
		if unsafe.StringData(string(x[1].(json.Number))) == unsafe.StringData(string(n)) {
			t.Fatal("number retained source")
		}
	}
}

func TestDetachedLongUnicodeChargesBudget(t *testing.T) {
	b, _ := budget.New(context.Background(), budget.Limits{MaxSteps: 3})
	defer b.Close()
	_, err := CloneDetached("x"+strings.Repeat("é", 10000), b, b.Limits.MaxOutputBytes)
	if !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("Unicode scan bypassed budget: %v", err)
	}
}
