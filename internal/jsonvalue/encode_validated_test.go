package jsonvalue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

// This path consumes the canonical, detached result and exact size produced by
// the prepared-evaluation boundary, rather than accepting arbitrary Go values.
func TestEncodeValidatedParity(t *testing.T) {
	values := []any{nil, false, true, int64(-9223372036854775808), uint64(18446744073709551615), float32(1e-7), math.Copysign(0, -1), 1e-7, 1e-6, 1e20, 1e21, json.Number("123456789012345678901234567890.001e+99"), "<>&\b\f\n\r\t\x00\"\\é\u2028\u2029", UTF16String("\xed\xa0\x80"), UTF16Object{"\xed\xa0\x80": []any{"\xed\xb0\x80", nil}}, map[string]any{"z": []any{false, 2.5}, "a": map[string]any{}}, []any{}, []any(nil), map[string]any(nil), strings.Repeat("é<", 4096)}
	for _, value := range values {
		b, _ := budget.New(context.Background(), budget.Limits{})
		owned, size, err := CloneDetachedSize(value, b, b.Limits.MaxOutputBytes)
		b.Close()
		if err != nil {
			t.Fatal(err)
		}
		b, _ = budget.New(context.Background(), budget.Limits{})
		want, err := Encode(owned, b)
		b.Close()
		if err != nil {
			t.Fatal(err)
		}
		b, _ = budget.New(context.Background(), budget.Limits{MaxOutputBytes: size})
		got, err := EncodeValidated(owned, size, b)
		b.Close()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%#v: got %q, want %q: %v", value, got, want, err)
		}
		// Each returned byte slice is independently owned.
		saved := bytes.Clone(got)
		b, _ = budget.New(context.Background(), budget.Limits{})
		again, err := EncodeValidated(owned, size, b)
		b.Close()
		if err != nil {
			t.Fatal(err)
		}
		again[0] ^= 1
		if !bytes.Equal(got, saved) {
			t.Fatal("encoding reused result storage")
		}
	}
}

func TestEncodeValidatedLimits(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  any
		size   int
		limits budget.Limits
	}{
		{"bytes", "<", 8, budget.Limits{MaxOutputBytes: 7}},
		{"depth", []any{[]any{nil}}, 8, budget.Limits{MaxDepth: 2}},
		{"collection", []any{nil, nil}, 11, budget.Limits{MaxCollectionItems: 1}},
		{"steps", []any{nil, nil}, 11, budget.Limits{MaxSteps: 2}},
		{"long-string-steps", strings.Repeat("x", 4096), 4098, budget.Limits{MaxSteps: 3}},
		{"long-key-steps", map[string]any{strings.Repeat("x", 4096): nil}, 4105, budget.Limits{MaxSteps: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := budget.New(context.Background(), tc.limits)
			defer b.Close()
			data, err := EncodeValidated(tc.value, tc.size, b)
			if !errors.Is(err, budget.ErrLimit) || data != nil {
				t.Fatalf("got %q, %v", data, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	b, _ := budget.New(ctx, budget.Limits{})
	defer b.Close()
	cancel()
	if data, err := EncodeValidated(nil, 4, b); data != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %q %v", data, err)
	}
}

func FuzzEncodeValidated(f *testing.F) {
	for _, s := range []string{`null`, `[1,-0,1e999,"\ud800"]`, `{"\ud800":["<>&",12345678901234567890]}`, `{"z":{},"a":[]}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 4096 {
			return
		}
		limits := budget.Limits{MaxDepth: 32, MaxSteps: 100000, MaxCollectionItems: 128}
		b, _ := budget.New(context.Background(), limits)
		v, err := Decode(raw, b)
		b.Close()
		if err != nil {
			return
		}
		b, _ = budget.New(context.Background(), limits)
		owned, size, err := CloneDetachedSize(v, b, b.Limits.MaxOutputBytes)
		b.Close()
		if err != nil {
			return
		}
		b, _ = budget.New(context.Background(), limits)
		want, err := Encode(owned, b)
		b.Close()
		if err != nil {
			return
		}
		b, _ = budget.New(context.Background(), limits)
		got, err := EncodeValidated(owned, size, b)
		b.Close()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("got %q, want %q: %v", got, want, err)
		}
	})
}

// Cancel through the context checkpoints, rather than relying on timer timing.
type encodingCancelContext struct {
	context.Context
	calls int
}

func (c *encodingCancelContext) Err() error {
	c.calls++
	if c.calls >= 4 {
		return context.Canceled
	}
	return nil
}
func TestEncodeValidatedCancellationDuringTraversal(t *testing.T) {
	for _, value := range []any{strings.Repeat("x", 1<<20), make([]any, 10000)} {
		ctx := &encodingCancelContext{Context: context.Background()}
		var b budget.Budget
		if err := budget.InitFastResolved(&b, ctx, budget.DefaultLimits()); err != nil {
			t.Fatal(err)
		}
		data, err := EncodeValidated(value, 2<<20, &b)
		b.Close()
		if data != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("got %d bytes: %v", len(data), err)
		}
	}
}
