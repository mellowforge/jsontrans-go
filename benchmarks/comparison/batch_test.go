//go:build batch && prepared

package comparison

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

var compiledBatchSink []jt.Result
var gnataBatchSink []any

func batchChecked(t testing.TB, rs []rule, c compiled, d *jt.Document) *jt.Batch {
	t.Helper()
	batch, err := jt.NewBatch(c.ours)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise both allocation modes, including stale populated destination slots.
	for _, dst := range [][]jt.Result{nil, make([]jt.Result, len(rs)+2)} {
		for i := range dst {
			dst[i] = jt.Result{Value: "stale", Defined: true}
		}
		results, err := batch.EvalPrepared(ctx, d, dst, jt.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != len(rs) {
			t.Fatalf("batch length %d, want %d", len(results), len(rs))
		}
		if len(dst) > 0 && len(results) > 0 && &results[0] != &dst[0] {
			t.Fatal("batch did not reuse destination capacity")
		}
		for i, r := range rs {
			check(t, "compiled batch", results[i].Value, results[i].Defined, r)
		}
	}
	return batch
}

func TestCompiledBatchParity(t *testing.T) {
	for _, size := range []int{1024, 8192, 65536} {
		for _, kind := range []string{"simple", "mixed", "unique"} {
			for _, n := range []int{4, 16, 64} {
				t.Run(fmt.Sprintf("%d/%s/N%d", size, kind, n), func(t *testing.T) {
					raw, rs := fixture(size), rules(kind, n)
					c := setup(t, raw, rs)
					d := prepareChecked(t, raw, rs, c)
					batchChecked(t, rs, c, d)
				})
			}
		}
	}
}

func TestCompiledBatchMixedErrors(t *testing.T) {
	a, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := jsonpath.New()
	if err != nil {
		t.Fatal(err)
	}
	compile := func(engine jt.Processor, expr string) jt.Program {
		t.Helper()
		program, err := engine.Compile(ctx, expr)
		if err != nil {
			t.Fatal(err)
		}
		return program
	}
	d, err := jt.PrepareJSON(ctx, fixture(1024), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	good := []jt.Program{compile(a, "Account.Missing"), compile(a, "Account.Nothing"), compile(p, "$.Account.Count")}
	batch, err := jt.NewBatch(good)
	if err != nil {
		t.Fatal(err)
	}
	got, err := batch.EvalPrepared(ctx, d, nil, jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range []rule{{"Account.Missing", nil, false}, {"Account.Nothing", nil, true}, {"$.Account.Count", []any{float64(4)}, true}} {
		check(t, "mixed batch", got[i].Value, got[i].Defined, r)
	}
	bad := compile(a, "$sum(Account.Name)")
	_, nativeErr := jt.EvalPrepared(ctx, bad, d, jt.EvalOptions{})
	var native *jt.Error
	if !errors.As(nativeErr, &native) || native.Code != "T0412" {
		t.Fatalf("independent error fixture: %v; want T0412", nativeErr)
	}
	programs := append(append([]jt.Program{}, good...), bad, compile(p, "$.Account.Name"))
	batch, err = jt.NewBatch(programs)
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]jt.Result, len(programs))
	for i := range dst {
		dst[i] = jt.Result{Value: map[string]any{"stale": true}, Defined: true}
	}
	got, err = batch.EvalPrepared(ctx, d, dst, jt.EvalOptions{})
	var batchErr *jt.BatchError
	var diagnostic *jt.Error
	if got != nil || !errors.As(err, &batchErr) || batchErr.Index != 3 || !errors.As(err, &diagnostic) || diagnostic.Code != "T0412" || diagnostic.Position != native.Position {
		t.Fatalf("mixed batch error/results: %#v, %v", got, err)
	}
	for i := 0; i < 3; i++ {
		if !reflect.DeepEqual(dst[i], jt.Result{}) {
			t.Fatalf("failed batch retained destination slot %d: %#v", i, dst[i])
		}
	}
}

// Every operation evaluates a whole document's rule set. Compilation, expected
// result checking, and warming gnata's schema plan are outside the timers.
func BenchmarkCompiledBatch(b *testing.B) {
	for _, size := range []int{1024, 8192, 65536} {
		for _, kind := range []string{"simple", "mixed", "unique"} {
			for _, n := range []int{4, 16, 64} {
				raw, rs := fixture(size), rules(kind, n)
				c := setup(b, raw, rs)
				d := prepareChecked(b, raw, rs, c)
				batch := batchChecked(b, rs, c, d)
				name := fmt.Sprintf("%d/%s/N%d", size, kind, n)
				for _, prepare := range []bool{false, true} {
					for _, reuse := range []bool{false, true} {
						operation := "reused_doc"
						if prepare {
							operation = "prepare_and_batch"
						}
						operation += "/new_dst"
						if reuse {
							operation = operation[:len(operation)-len("new_dst")] + "reused_dst"
						}
						b.Run(name+"/"+operation, func(b *testing.B) {
							var dst []jt.Result
							if reuse {
								dst = make([]jt.Result, n)
							}
							b.ReportAllocs()
							b.ResetTimer()
							for range b.N {
								doc := d
								var err error
								if prepare {
									doc, err = jt.PrepareJSON(ctx, raw, jt.PrepareOptions{})
									if err != nil {
										b.Fatal(err)
									}
								}
								compiledBatchSink, err = batch.EvalPrepared(ctx, doc, dst, jt.EvalOptions{})
								if err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				}
				for _, validate := range []bool{false, true} {
					operation := "gnata_warmed"
					if validate {
						operation = "gnata_validate_warmed"
					}
					b.Run(name+"/"+operation, func(b *testing.B) {
						b.ReportAllocs()
						for range b.N {
							if validate && !json.Valid(raw) {
								b.Fatal("invalid fixture")
							}
							var err error
							gnataBatchSink, err = c.stream.EvalMany(ctx, raw, "fixture", c.indices)
							if err != nil {
								b.Fatal(err)
							}
						}
					})
				}
			}
		}
	}
}
