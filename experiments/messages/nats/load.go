package natsexperiment

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	m "github.com/mellowforge/jsontrans-go/experiments/messages"
	"github.com/nats-io/nats.go"
)

type Config struct {
	Engine   string     `json:"engine"`
	Kind     string     `json:"kind"`
	Strategy m.Strategy `json:"strategy"`
	Fanout   int        `json:"fanout"`
	Workers  int        `json:"workers"`
	Rate     int        `json:"target_rate"`
	Count    int        `json:"count"`
	Queue    int        `json:"queue_capacity"`
	Size     int        `json:"target_bytes"`
}
type Report struct {
	Config               Config      `json:"config"`
	ServerVersion        string      `json:"server_version"`
	GOMAXPROCS           int         `json:"gomaxprocs"`
	Sent                 int         `json:"sent"`
	Received             int64       `json:"received"`
	Processed            int64       `json:"processed"`
	Completed            int         `json:"completed"`
	Outputs              int         `json:"outputs"`
	QueueRejected        int64       `json:"queue_rejected"`
	TransformErrors      int64       `json:"transform_errors"`
	PublishErrors        int64       `json:"publish_errors"`
	InvalidOutputs       int64       `json:"invalid_outputs"`
	AsyncErrors          int64       `json:"async_errors"`
	InputPendingDropped  int         `json:"input_pending_dropped"`
	OutputPendingDropped int         `json:"output_pending_dropped"`
	Unaccounted          int         `json:"unaccounted_inputs"`
	IncompleteFanout     int         `json:"incomplete_fanout"`
	PublishSeconds       float64     `json:"publish_seconds"`
	TotalSeconds         float64     `json:"total_seconds"`
	ActualOffered        float64     `json:"actual_offered_per_second"`
	CompletedPerSecond   float64     `json:"completed_per_second"`
	MaxPacingLatenessNS  int64       `json:"max_pacing_lateness_ns"`
	EndToEnd             Percentiles `json:"end_to_end"`
	QueueWait            Percentiles `json:"queue_wait"`
	Processing           Percentiles `json:"processing"`
	AllocatedBytes       uint64      `json:"allocated_bytes_during_run"`
	Mallocs              uint64      `json:"mallocs_during_run"`
	GCs                  uint32      `json:"gcs_during_run"`
	Lossless             bool        `json:"lossless"`
	Valid                bool        `json:"valid"`
}
type record struct {
	sent, queue, process atomic.Int64
	failed, rejected     atomic.Bool
}
type route struct {
	plan    *m.Plan
	fixture m.Fixture
}
type job struct {
	id, route int
	raw       []byte
	received  time.Time
}

var sequence atomic.Uint64

func Run(ctx context.Context, url string, c Config) (report Report, err error) {
	if c.Count < 1 || c.Count > 50000 || c.Workers < 1 || c.Workers > 8 || c.Fanout < 1 || c.Fanout > 16 || c.Queue < 1 || c.Queue > 8192 || c.Rate < 0 || c.Rate > 100000 {
		return report, fmt.Errorf("load config bounds")
	}
	if c.Strategy != m.Direct && c.Strategy != m.Prepared && c.Strategy != m.Batched {
		return report, fmt.Errorf("strategy")
	}
	runtime.GOMAXPROCS(c.Workers)
	routes := make([]route, 8)
	for i := range routes {
		engine, kind := c.Engine, c.Kind
		if engine == "mixed" {
			engines := []string{"jsonata", "jsonata", "jsonpath", "jsonpath", "jsonata", "jsonpath", "jsonata", "jsonata"}
			kinds := []string{"scalar", "object", "scalar", "object", "array", "array", "bindings", "host"}
			engine, kind = engines[i], kinds[i]
		}
		f, e := m.NewFixture(engine, kind, c.Size, c.Fanout)
		if e != nil {
			return report, e
		}
		p, e := m.NewPlan(ctx, f.Config)
		if e != nil {
			return report, e
		}
		defer p.Close()
		routes[i] = route{p, f}
	}
	expected := make([][]jt.JSONResult, c.Count)
	records := make([]record, c.Count)
	for i := range expected {
		expected[i] = routes[i%8].fixture.Samples[(i/8)%8].Want
	}
	collect := newCollector(expected)
	var received, rejected, processed, transformErr, publishErr, invalid, asyncErr atomic.Int64
	onError := func(_ *nats.Conn, _ *nats.Subscription, _ error) { asyncErr.Add(1) }
	publisher, e := nats.Connect(url, nats.NoReconnect(), nats.Timeout(3*time.Second), nats.ErrorHandler(onError))
	if e != nil {
		return report, e
	}
	defer publisher.Close()
	workerConn, e := nats.Connect(url, nats.NoReconnect(), nats.Timeout(3*time.Second), nats.ErrorHandler(onError))
	if e != nil {
		return report, e
	}
	defer workerConn.Close()
	prefix := fmt.Sprintf("bench.%d.%d", time.Now().UnixNano(), sequence.Add(1))
	inputSubject := prefix + ".in."
	outputSubject := prefix + ".out.result"
	bySubject := map[string]int{}
	for i := range routes {
		bySubject[inputSubject+strconv.Itoa(i)] = i
	}
	inputBarrier, outputBarrier := make(chan struct{}), make(chan struct{})
	var inputOnce, outputOnce sync.Once
	jobs := make(chan job, c.Queue)
	var producerMu sync.Mutex
	var stop atomic.Bool
	var clock atomic.Pointer[time.Time]
	outSub, e := publisher.Subscribe(prefix+".out.*", func(msg *nats.Msg) {
		if msg.Header.Get("X-Barrier") == "1" {
			outputOnce.Do(func() { close(outputBarrier) })
			return
		}
		id, e1 := strconv.Atoi(msg.Header.Get("X-Id"))
		slot, e2 := strconv.Atoi(msg.Header.Get("X-Slot"))
		if e1 != nil || e2 != nil || id < 0 || id >= len(records) {
			invalid.Add(1)
			return
		}
		o := clock.Load()
		if o == nil {
			invalid.Add(1)
			return
		}
		ns := time.Since(*o).Nanoseconds() - records[id].sent.Load()
		if e := collect.add(id, slot, msg.Data, ns); e != nil {
			invalid.Add(1)
		}
	})
	if e != nil {
		return report, e
	}
	defer outSub.Unsubscribe()
	if e = outSub.SetPendingLimits(100000, 64<<20); e != nil {
		return report, e
	}
	inSub, e := workerConn.Subscribe(prefix+".in.*", func(msg *nats.Msg) {
		producerMu.Lock()
		defer producerMu.Unlock()
		if stop.Load() {
			return
		}
		if msg.Header.Get("X-Barrier") == "1" {
			inputOnce.Do(func() { close(inputBarrier) })
			return
		}
		id, e := strconv.Atoi(msg.Header.Get("X-Id"))
		routeIndex, known := bySubject[msg.Subject]
		if e != nil || id < 0 || id >= len(records) || !known || routeIndex != id%8 {
			asyncErr.Add(1)
			return
		}
		at := time.Now()
		received.Add(1)
		if len(jobs) == cap(jobs) {
			records[id].rejected.Store(true)
			rejected.Add(1)
			return
		}
		// One serialized callback produces jobs; workers only remove them, so a
		// non-full queue cannot fill between this check and the send.
		jobs <- job{id: id, route: routeIndex, raw: append([]byte(nil), msg.Data...), received: at}
	})
	if e != nil {
		return report, e
	}
	if e = inSub.SetPendingLimits(100000, 64<<20); e != nil {
		return report, e
	}
	if e = publisher.FlushTimeout(3 * time.Second); e != nil {
		return report, e
	}
	if e = workerConn.FlushTimeout(3 * time.Second); e != nil {
		return report, e
	}
	duration := time.Duration(0)
	if c.Rate > 0 {
		duration = time.Duration(c.Count) * time.Second / time.Duration(c.Rate)
	}
	if duration > 30*time.Second {
		return report, fmt.Errorf("publish window capped at30seconds")
	}
	runCtx, cancel := context.WithTimeout(ctx, duration+15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for range c.Workers {
		wg.Go(func() {
			workers := make([]*m.Worker, 8)
			for i, r := range routes {
				workers[i] = r.plan.NewWorker()
			}
			for j := range jobs {
				start := time.Now()
				records[j.id].queue.Store(start.Sub(j.received).Nanoseconds())
				out, e := workers[j.route].Eval(runCtx, j.raw, c.Strategy)
				records[j.id].process.Store(time.Since(start).Nanoseconds())
				processed.Add(1)
				if e != nil {
					records[j.id].failed.Store(true)
					transformErr.Add(1)
					continue
				}
				bad := len(out) != c.Fanout
				for _, result := range out {
					if !result.Defined {
						bad = true
					}
				}
				if bad {
					records[j.id].failed.Store(true)
					transformErr.Add(1)
					invalid.Add(1)
					continue
				}
				for slot, result := range out {
					h := nats.Header{}
					h.Set("X-Id", strconv.Itoa(j.id))
					h.Set("X-Slot", strconv.Itoa(slot))
					if e = workerConn.PublishMsg(&nats.Msg{Subject: outputSubject, Header: h, Data: result.Value}); e != nil {
						publishErr.Add(1)
					}
				}
			}
		})
	}
	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			stop.Store(true)
			inSub.Unsubscribe()
			producerMu.Lock()
			close(jobs)
			producerMu.Unlock()
			wg.Wait()
		})
	}
	defer shutdown()
	var memoryBefore, memoryAfter runtime.MemStats
	runtime.ReadMemStats(&memoryBefore)
	origin := time.Now()
	clock.Store(&origin)
	for i := 0; i < c.Count; i++ {
		if c.Rate > 0 {
			due := origin.Add(time.Duration(i) * time.Second / time.Duration(c.Rate))
			if delay := time.Until(due); delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-runCtx.Done():
					timer.Stop()
					return report, runCtx.Err()
				}
			}
			late := time.Since(due).Nanoseconds()
			if late > report.MaxPacingLatenessNS {
				report.MaxPacingLatenessNS = late
			}
		}
		if e = runCtx.Err(); e != nil {
			return report, e
		}
		records[i].sent.Store(time.Since(origin).Nanoseconds())
		sample := routes[i%8].fixture.Samples[(i/8)%8]
		h := nats.Header{}
		h.Set("X-Id", strconv.Itoa(i))
		if e = publisher.PublishMsg(&nats.Msg{Subject: inputSubject + strconv.Itoa(i%8), Header: h, Data: sample.Input}); e != nil {
			return report, e
		}
		report.Sent++
	}
	report.PublishSeconds = time.Since(origin).Seconds()
	sendBarrier := func(nc *nats.Conn, subject string, done <-chan struct{}) error {
		h := nats.Header{}
		h.Set("X-Barrier", "1")
		if e := nc.PublishMsg(&nats.Msg{Subject: subject, Header: h}); e != nil {
			return e
		}
		if e := nc.FlushTimeout(3 * time.Second); e != nil {
			return e
		}
		select {
		case <-done:
			return nil
		case <-runCtx.Done():
			return runCtx.Err()
		}
	}
	if e = sendBarrier(publisher, inputSubject+"barrier", inputBarrier); e != nil {
		return report, fmt.Errorf("input barrier: %w", e)
	}
	report.InputPendingDropped, _ = inSub.Dropped()
	shutdown()
	if e = sendBarrier(workerConn, prefix+".out.barrier", outputBarrier); e != nil {
		return report, fmt.Errorf("output barrier: %w", e)
	}
	report.OutputPendingDropped, _ = outSub.Dropped()
	runtime.ReadMemStats(&memoryAfter)
	report.AllocatedBytes = memoryAfter.TotalAlloc - memoryBefore.TotalAlloc
	report.Mallocs = memoryAfter.Mallocs - memoryBefore.Mallocs
	report.GCs = memoryAfter.NumGC - memoryBefore.NumGC
	report.TotalSeconds = time.Since(origin).Seconds()
	report.Config = c
	report.ServerVersion = publisher.ConnectedServerVersion()
	report.GOMAXPROCS = runtime.GOMAXPROCS(0)
	report.Received = received.Load()
	report.QueueRejected = rejected.Load()
	report.Processed = processed.Load()
	report.TransformErrors = transformErr.Load()
	report.PublishErrors = publishErr.Load()
	report.InvalidOutputs = invalid.Load()
	report.AsyncErrors = asyncErr.Load()
	collect.mu.Lock()
	queueNS, processNS, e2e := []int64{}, []int64{}, []int64{}
	for i := range records {
		if records[i].process.Load() > 0 {
			queueNS = append(queueNS, records[i].queue.Load())
			processNS = append(processNS, records[i].process.Load())
		}
		if collect.arrival[i] > 0 {
			e2e = append(e2e, collect.arrival[i])
		} else if !records[i].rejected.Load() && !records[i].failed.Load() && records[i].process.Load() > 0 {
			report.IncompleteFanout++
		}
	}
	report.Completed = collect.done
	report.Outputs = collect.outputs
	collect.mu.Unlock()
	report.Unaccounted = c.Count - int(report.QueueRejected+report.TransformErrors) - report.Completed
	report.QueueWait = quantiles(queueNS)
	report.Processing = quantiles(processNS)
	report.EndToEnd = quantiles(e2e)
	report.ActualOffered = float64(report.Sent) / report.PublishSeconds
	report.CompletedPerSecond = float64(report.Completed) / report.TotalSeconds
	report.Valid = report.InvalidOutputs == 0 && report.AsyncErrors == 0 && report.PublishErrors == 0 && report.InputPendingDropped == 0 && report.OutputPendingDropped == 0 && report.Unaccounted == 0 && report.IncompleteFanout == 0
	report.Lossless = report.Valid && report.QueueRejected == 0 && report.TransformErrors == 0 && report.Completed == report.Sent
	return report, nil
}
