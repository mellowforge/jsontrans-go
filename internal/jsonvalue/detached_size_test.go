package jsonvalue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

func TestDetachedSizeMatchesEncodingAndLimits(t *testing.T) {
	for _, v := range []any{nil, true, false, 1.25, float64(1e-8), float64(1e21), json.Number("1.000e4"), "<é>\n\u2028", UTF16String("\xed\xa0\x80"), UTF16Object{"\xed\xa0\x80": []any{false, "&"}}, map[string]any{"z": []any{1, nil, "x"}, "a": true}, []any{}, map[string]any{}, strings.Repeat("x", 2000)} {
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		b, err := budget.New(context.Background(), budget.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		out, size, err := CloneDetachedSize(v, b, len(encoded))
		b.Close()
		if err != nil || size != len(encoded) {
			t.Fatalf("%#v: size=%d want=%d: %v", v, size, len(encoded), err)
		}
		result, err := json.Marshal(out)
		if err != nil || string(result) != string(encoded) {
			t.Fatalf("%q %q %v", result, encoded, err)
		}
		b, _ = budget.New(context.Background(), budget.DefaultLimits())
		_, _, err = CloneDetachedSize(v, b, len(encoded)-1)
		b.Close()
		if !errors.Is(err, budget.ErrLimit) {
			t.Fatalf("%#v: limit %v", v, err)
		}
	}
}
