//go:build prepared

package comparison

import (
	"fmt"
	"runtime"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
)

var preparedResultSink jt.Result

func prepareChecked(t testing.TB, raw []byte, rs []rule, c compiled) *jt.Document {
	t.Helper()
	d, err := jt.PrepareJSON(ctx, raw, jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rs {
		v, err := jt.EvalPrepared(ctx, c.ours[i], d, jt.EvalOptions{})
		if err != nil {
			t.Fatalf("prepared %q: %v", r.expr, err)
		}
		check(t, "prepared", v.Value, v.Defined, r)
	}
	return d
}
func TestPreparedParity(t *testing.T) {
	for _, size := range []int{1024, 8192, 65536} {
		for _, kind := range []string{"simple", "mixed", "unique"} {
			for _, n := range []int{4, 16, 64} {
				t.Run(fmt.Sprintf("%d/%s/N%d", size, kind, n), func(t *testing.T) {
					raw := fixture(size)
					rs := rules(kind, n)
					c := setup(t, raw, rs)
					prepareChecked(t, raw, rs, c)
				})
			}
		}
	}
}
func BenchmarkPrepared(b *testing.B) {
	for _, size := range []int{1024, 8192, 65536} {
		for _, kind := range []string{"simple", "mixed", "unique"} {
			for _, n := range []int{4, 16, 64} {
				raw := fixture(size)
				rs := rules(kind, n)
				c := setup(b, raw, rs)
				d := prepareChecked(b, raw, rs, c)
				name := fmt.Sprintf("%d/%s/N%d", size, kind, n)
				b.Run(name+"/prepare_only", func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						v, err := jt.PrepareJSON(ctx, raw, jt.PrepareOptions{})
						if err != nil {
							b.Fatal(err)
						}
						sink = v
					}
				})
				b.Run(name+"/prepared_reuse", func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						for _, p := range c.ours {
							v, err := jt.EvalPrepared(ctx, p, d, jt.EvalOptions{})
							if err != nil {
								b.Fatal(err)
							}
							preparedResultSink = v
						}
					}
				})
				b.Run(name+"/prepare_and_eval", func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						doc, err := jt.PrepareJSON(ctx, raw, jt.PrepareOptions{})
						if err != nil {
							b.Fatal(err)
						}
						for _, p := range c.ours {
							v, err := jt.EvalPrepared(ctx, p, doc, jt.EvalOptions{})
							if err != nil {
								b.Fatal(err)
							}
							preparedResultSink = v
						}
					}
				})
				b.Run(name+"/prepared_parallel", func(b *testing.B) {
					b.ReportAllocs()
					b.RunParallel(func(pb *testing.PB) {
						var last jt.Result
						for pb.Next() {
							for _, p := range c.ours {
								var err error
								last, err = jt.EvalPrepared(ctx, p, d, jt.EvalOptions{})
								if err != nil {
									b.Error(err)
									return
								}
							}
						}
						runtime.KeepAlive(last)
					})
				})
				b.Run(name+"/gnata_parallel", func(b *testing.B) {
					b.ReportAllocs()
					b.RunParallel(func(pb *testing.PB) {
						var last []any
						for pb.Next() {
							var err error
							last, err = c.stream.EvalMany(ctx, raw, "fixture", c.indices)
							if err != nil {
								b.Error(err)
								return
							}
						}
						runtime.KeepAlive(last)
					})
				})
			}
		}
	}
}

// BenchmarkRetained reports an approximate post-GC live-heap cost per document.
// A single measurement holds 128 documents; allocator and GC noise still apply.
func BenchmarkRetained(b *testing.B) {
	for _, size := range []int{1024, 8192, 65536} {
		b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
			raw := fixture(size)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			docs := make([]*jt.Document, 128)
			for i := range docs {
				var err error
				docs[i], err = jt.PrepareJSON(ctx, raw, jt.PrepareOptions{})
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.GC()
			runtime.ReadMemStats(&after)
			b.ResetTimer()
			if after.HeapAlloc >= before.HeapAlloc {
				b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/float64(len(docs)), "retained-B/doc")
			}
			for range b.N {
				runtime.KeepAlive(docs)
			}
			runtime.KeepAlive(docs)
		})
	}
}
