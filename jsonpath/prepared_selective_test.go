package jsonpath

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

func TestPreparedContainerSkipsUnselectedMaterialization(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	raw := `{"selected":{"n":123456789012345678901234567890,"s":"\ud800"},"unused":[` + strings.Repeat(`null,`, 2000) + `null]}`
	d, err := jsontrans.PrepareJSON(ctx, []byte(raw), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Compile(ctx, `$.selected`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jsontrans.EvalPrepared(ctx, p, d, jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxSteps: 100}})
	if err != nil {
		t.Fatalf("selected subtree should fit work budget: %v", err)
	}
	want := []any{map[string]any{"n": json.Number("123456789012345678901234567890"), "s": jsonvalue.UTF16String("\xed\xa0\x80")}}
	if !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v want %#v", got.Value, want)
	}
}
