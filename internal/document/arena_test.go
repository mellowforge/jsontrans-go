package document

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

// Per-container slices make this workload allocate once or more for every
// small object and array. Preparation should instead grow shared arenas.
func TestParseSmallContainersAllocation(t *testing.T) {
	raw := []byte(`[` + strings.Repeat(`{"a":[1,2],"b":{"x":3}},`, 511) + `{"a":[1,2],"b":{"x":3}}]`)
	var parseErr error
	allocs := testing.AllocsPerRun(10, func() {
		_, parseErr = Parse(context.Background(), raw, Options{})
	})
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if allocs > 100 {
		t.Fatalf("small-container preparation allocated %.0f objects; want <= 100", allocs)
	}
}

func TestArenaNestedDuplicateMetadata(t *testing.T) {
	raw := []byte(`{"first":{"a":[1,{"nested":2}]},"\u0066irst":{"last":[3,4]},"other":[],"\ud800":"\udfff","big":9007199254740993,"gone":1e9999,"gone":5}`)
	d, err := Parse(context.Background(), raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		raw[i] = ' '
	}
	root := d.Root()
	if got := root.Keys(); !reflect.DeepEqual(got, []string{"first", "other", "\xed\xa0\x80", "big", "gone"}) {
		t.Fatalf("source keys: %q", got)
	}
	first, _ := root.Member("first")
	last, _ := first.Member("last")
	n, ok := last.Index(1)
	if !ok || n.JSON() != "4" || last.Len() != 2 {
		t.Fatalf("duplicate nested value: %s", first.JSON())
	}
	big, _ := root.Member("big")
	if v, ok := big.Number(); !ok || v.String() != "9007199254740993" {
		t.Fatalf("exact number: %q", v)
	}
	surrogate, _ := root.Member("\xed\xa0\x80")
	if v, ok := surrogate.Text(); !ok || v != "\xed\xbf\xbf" {
		t.Fatalf("surrogate: %q", v)
	}
	if d.Stats().Binary64Overflow || d.Stats().MaxCollectionItems != 5 {
		t.Fatalf("duplicate summary: %+v", d.Stats())
	}
}

func TestArenaWideObjectDuplicates(t *testing.T) {
	var raw strings.Builder
	raw.WriteByte('{')
	for i := 0; i < 2048; i++ {
		fmt.Fprintf(&raw, `"k%d":[%d],`, i, i)
	}
	raw.WriteString(`"k0":[9999]}`)
	d, err := Parse(context.Background(), []byte(raw.String()), Options{})
	if err != nil {
		t.Fatal(err)
	}
	root := d.Root()
	if root.Len() != 2048 || root.Keys()[0] != "k0" {
		t.Fatal("wide duplicate changed member count or first key position")
	}
	for _, i := range []int{0, 1, 1024, 2047} {
		v, ok := root.Member(fmt.Sprintf("k%d", i))
		if !ok {
			t.Fatalf("missing member k%d", i)
		}
		n, _ := v.Index(0)
		want := fmt.Sprint(i)
		if i == 0 {
			want = "9999"
		}
		if n.JSON() != want {
			t.Fatalf("k%d=%s, want %s", i, n.JSON(), want)
		}
	}
}

func TestArenaMaterializeSourceOrder(t *testing.T) {
	ctx := jsonvalue.WithObjectOrder(context.Background())
	d, err := Parse(ctx, []byte(`{"books":{},"loans":{},"customers":{},"books":{"z":1,"a":2}}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	v, err := d.Materialize(ctx, Options{})
	if err != nil {
		t.Fatal(err)
	}
	root := v.(map[string]any)
	if got := jsonvalue.ObjectKeys(ctx, root); !reflect.DeepEqual(got, []string{"books", "loans", "customers"}) {
		t.Fatalf("materialized root order: %q", got)
	}
	if got := jsonvalue.ObjectKeys(ctx, root["books"].(map[string]any)); !reflect.DeepEqual(got, []string{"z", "a"}) {
		t.Fatalf("materialized nested order: %q", got)
	}
}

func TestMaterializeWithBudgetSharesCountersAndContext(t *testing.T) {
	d, err := Parse(context.Background(), []byte(`[1,2]`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	l := budget.DefaultLimits()
	l.MaxSteps = 3
	if _, err := d.Materialize(context.Background(), Options{Limits: &l}); err != nil {
		t.Fatalf("three-node materialization: %v", err)
	}
	b, err := budget.New(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.Step(); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializeWithBudget(d, b); !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("materialization reset caller counters: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	b, err = budget.New(ctx, budget.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	cancel()
	if _, err := MaterializeWithBudget(d, b); !errors.Is(err, context.Canceled) {
		t.Fatalf("materialization lost caller cancellation: %v", err)
	}
}

func TestArenaLimitsAndCancellation(t *testing.T) {
	for _, raw := range []string{`[0,1,2]`, `{"a":0,"b":1,"c":2}`} {
		if _, err := Parse(context.Background(), []byte(raw), Options{Limits: &budget.Limits{MaxCollectionItems: 2}}); !errors.Is(err, budget.ErrLimit) {
			t.Fatalf("collection limit: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Parse(ctx, []byte(`[{},[]]`), Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	for _, depth := range []int{1, 64} {
		raw := strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth)
		d, err := Parse(context.Background(), []byte(raw), Options{Limits: &budget.Limits{MaxDepth: depth + 1}})
		if err != nil || d.Stats().Depth != depth+1 {
			t.Fatalf("depth %d: %v", depth, err)
		}
	}
}

func TestUnpreparedDocumentRejected(t *testing.T) {
	for name, d := range map[string]*Document{"nil": nil, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			if err := d.CheckLimits(context.Background(), budget.DefaultLimits()); !errors.Is(err, jsonvalue.ErrInvalid) {
				t.Fatalf("unprepared document passed validation: %v", err)
			}
			if _, err := d.Materialize(context.Background(), Options{}); !errors.Is(err, jsonvalue.ErrInvalid) {
				t.Fatalf("unprepared document materialization: %v", err)
			}
			root := d.Root()
			if root.Kind() != "" || root.JSON() != "" || root.Len() != 0 {
				t.Fatal("unprepared document exposed a node")
			}
		})
	}
}

func arenaFixture(size int) []byte {
	const item = `{"id":123,"name":"sample","values":[1,2,3],"active":true}`
	n := max(1, (size-2)/(len(item)+1))
	return []byte(`[` + strings.Repeat(item+",", n-1) + item + `]`)
}

func BenchmarkPrepareArena(b *testing.B) {
	for _, size := range []int{1024, 8192, 65536} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			raw := arenaFixture(size)
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for b.Loop() {
				if _, err := Parse(context.Background(), raw, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
