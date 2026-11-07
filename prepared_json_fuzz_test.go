package jsontrans_test

import (
	"context"
	"reflect"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

// Compare byte encoding over the indexed and ordinary paths, with bounded input
// and no fuzz-supplied expression compilation.
func FuzzPreparedJSON(f *testing.F) {
	for _, seed := range []string{`null`, `[]`, `{"a":[1,null,"<>"]}`, `9007199254740993`, `{"\ud800":"\udfff"}`} {
		f.Add(seed)
	}
	ctx := context.Background()
	e, err := jsonpath.New()
	if err != nil {
		f.Fatal(err)
	}
	p, err := e.Compile(ctx, `$`)
	if err != nil {
		f.Fatal(err)
	}
	b, err := jt.NewBatch([]jt.Program{p, p})
	if err != nil {
		f.Fatal(err)
	}
	l := jt.Limits{MaxInputBytes: 4096, MaxDepth: 32, MaxCollectionItems: 128}
	o := jt.EvalOptions{Limits: &l}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}
		d, err := jt.PrepareJSON(ctx, []byte(input), jt.PrepareOptions{Limits: &l})
		if err != nil {
			return
		}
		want, err := jt.EvalJSON(ctx, p, []byte(input), o)
		if err != nil {
			return
		} // Decoder validation may reject duplicates/numbers.
		got, err := jt.EvalPreparedJSON(ctx, p, d, o)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("single: %+v want %+v error=%v", got, want, err)
		}
		all, err := b.EvalPreparedJSON(ctx, d, nil, o)
		if err != nil || len(all) != 2 || !reflect.DeepEqual(all[0], want) || !reflect.DeepEqual(all[1], want) {
			t.Fatalf("batch: %+v want %+v error=%v", all, want, err)
		}
	})
}
