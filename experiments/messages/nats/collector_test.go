package natsexperiment

import (
	jt "github.com/mellowforge/jsontrans-go"
	"testing"
)

func TestCollectorChecksEveryOutput(t *testing.T) {
	want := [][]jt.JSONResult{{{Value: []byte(`1`), Defined: true}, {Value: []byte(`null`), Defined: true}}}
	c := newCollector(want)
	if err := c.add(0, 1, []byte(`null`), 10); err != nil {
		t.Fatal(err)
	}
	if c.completed() != 0 {
		t.Fatal("completed partial fanout")
	}
	if err := c.add(0, 0, []byte(`1`), 20); err != nil {
		t.Fatal(err)
	}
	if c.completed() != 1 {
		t.Fatal("did not complete")
	}
	for _, v := range []struct {
		id, slot int
		raw      string
	}{{0, 0, `1`}, {1, 0, `1`}, {0, 2, `1`}} {
		if err := c.add(v.id, v.slot, []byte(v.raw), 30); err == nil {
			t.Fatal("accepted duplicate or invalid correlation")
		}
	}
	bad := newCollector(want)
	if err := bad.add(0, 0, []byte(`2`), 10); err == nil {
		t.Fatal("accepted wrong transformed value")
	}
}
func TestQuantiles(t *testing.T) {
	q := quantiles([]int64{100, 1, 50})
	if q.P50 != 50 || q.P95 != 100 || q.P99 != 100 {
		t.Fatal(q)
	}
}
