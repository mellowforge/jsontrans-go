package jsontrans_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

func TestSelectivePreparedJSONBatchParityAndConcurrency(t *testing.T) {
	ctx := context.Background()
	a, _ := jsonata.New()
	p, _ := jsonpath.New()
	for _, processors := range [][]jt.Processor{{a, a}, {p, p}, {a, p}} {
		exprs := []string{`{"name":customer.name,"code":meta.id+2}`, `{"id":meta.id*3}`}
		if processors[0] == p {
			exprs[0] = `$.customer`
		}
		if processors[1] == p {
			exprs[1] = `$.meta`
		}
		programs := make([]jt.Program, 2)
		for i := range programs {
			var err error
			programs[i], err = processors[i].Compile(ctx, exprs[i])
			if err != nil {
				t.Fatal(err)
			}
		}
		batch, err := jt.NewBatch(programs)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{`{"customer":{"name":"Ada"},"meta":{"id":1}}`, `{"customer":{"name":"\ud800"},"meta":{"id":-0}}`, `{"customer":{"name":["A","B"]},"meta":{"id":2}}`} {
			d, err := jt.PrepareJSON(ctx, []byte(raw), jt.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want := make([]jt.JSONResult, 2)
			for i := range programs {
				want[i], err = jt.EvalJSON(ctx, programs[i], []byte(raw), jt.EvalOptions{})
				if err != nil {
					t.Fatal(err)
				}
			}
			var wg sync.WaitGroup
			for range 4 {
				wg.Go(func() {
					for range 8 {
						got, err := batch.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{})
						if err != nil || !reflect.DeepEqual(got, want) {
							t.Errorf("batch %#v/%v != %#v", got, err, want)
							return
						}
						saved := bytes.Clone(got[0].Value)
						prior := got[0]
						_, err = batch.EvalPreparedJSON(ctx, d, got, jt.EvalOptions{})
						if err != nil || !bytes.Equal(prior.Value, saved) {
							t.Errorf("reused bytes: %v", err)
							return
						}
					}
				})
			}
			wg.Wait()
			// Exact escaped-byte aggregate limit, including isolated-surrogate output.
			total := len(want[0].Value) + len(want[1].Value)
			if _, err = batch.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: total - 1}}); !errors.Is(err, jt.ErrLimit) {
				t.Fatalf("aggregate output cap: %v", err)
			}
		}
	}
}
