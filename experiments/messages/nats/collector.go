// Package natsexperiment is a development-only Core NATS load harness.
package natsexperiment

import (
	"fmt"
	jt "github.com/mellowforge/jsontrans-go"
	m "github.com/mellowforge/jsontrans-go/experiments/messages"
	"sort"
	"sync"
)

type Percentiles struct {
	P50 int64 `json:"p50_ns"`
	P95 int64 `json:"p95_ns"`
	P99 int64 `json:"p99_ns"`
}

func quantiles(v []int64) Percentiles {
	if len(v) == 0 {
		return Percentiles{}
	}
	values := append([]int64(nil), v...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	at := func(p int) int64 { return values[(p*len(values)+99)/100-1] }
	return Percentiles{at(50), at(95), at(99)}
}

type collector struct {
	mu      sync.Mutex
	want    [][]jt.JSONResult
	seen    []uint16
	arrival []int64
	done    int
	outputs int
}

func newCollector(want [][]jt.JSONResult) *collector {
	return &collector{want: want, seen: make([]uint16, len(want)), arrival: make([]int64, len(want))}
}
func (c *collector) add(id, slot int, raw []byte, ns int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id < 0 || id >= len(c.want) || slot < 0 || slot >= len(c.want[id]) || slot >= 16 {
		return fmt.Errorf("invalid output correlation %d/%d", id, slot)
	}
	bit := uint16(1) << uint(slot)
	if c.seen[id]&bit != 0 {
		return fmt.Errorf("duplicate output %d/%d", id, slot)
	}
	if !c.want[id][slot].Defined {
		return fmt.Errorf("published missing output")
	}
	if err := m.CheckOutputs([]jt.JSONResult{{Value: raw, Defined: true}}, c.want[id][slot:slot+1]); err != nil {
		return err
	}
	c.seen[id] |= bit
	c.outputs++
	if c.seen[id] == uint16((uint32(1)<<uint(len(c.want[id])))-1) {
		c.done++
		c.arrival[id] = ns
	}
	return nil
}
func (c *collector) completed() int { c.mu.Lock(); defer c.mu.Unlock(); return c.done }
