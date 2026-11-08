// Command diagnose captures a bounded, separately instrumented NATS case.
// Its timing numbers must not be mixed with uninstrumented load comparisons.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/trace"
	"syscall"
	"time"

	m "github.com/mellowforge/jsontrans-go/experiments/messages"
	n "github.com/mellowforge/jsontrans-go/experiments/messages/nats"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() (returned error) {
	output := flag.String("output", "", "new diagnostic directory")
	duration := flag.Duration("duration", 5*time.Second, "diagnostic publication window (1..10s)")
	fanout := flag.Int("fanout", 1, "object rules: 1 or 4")
	strategy := flag.String("strategy", "prepared", "direct, prepared or batch")
	procs := flag.Int("procs", 1, "Go runtime execution slots (1..8); transform workers remain one")
	tracing := flag.Bool("trace", true, "capture a separate execution trace")
	flag.Parse()
	if *procs < 1 || *procs > 8 || *output == "" || *duration < time.Second || *duration > 10*time.Second || (*fanout != 1 && *fanout != 4) {
		return fmt.Errorf("invalid diagnostic configuration")
	}
	mode := m.Strategy(*strategy)
	if mode != m.Direct && mode != m.Prepared && mode != m.Batched {
		return fmt.Errorf("invalid diagnostic strategy")
	}
	if err := os.Mkdir(*output, 0755); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	startup, stop := context.WithTimeout(ctx, 2*time.Minute)
	broker, err := n.StartBroker(startup, n.DefaultImage, "16,17")
	stop()
	if err != nil {
		return err
	}
	metadata := map[string]any{"diagnostic_trace": *tracing, "completed": false, "go": runtime.Version(), "image": n.DefaultImage, "container_id": broker.Container.GetContainerID(), "duration": duration.String(), "trace": "trace.out"}
	if info, ok := debug.ReadBuildInfo(); ok {
		metadata["build_info"] = info
	}
	defer func() {
		cleanup := broker.Close()
		metadata["container_removed"] = cleanup == nil
		returned = errors.Join(returned, cleanup)
		if returned != nil {
			metadata["error"] = returned.Error()
			metadata["completed"] = false
		}
		raw, err := json.MarshalIndent(metadata, "", "  ")
		returned = errors.Join(returned, err)
		if err == nil {
			returned = errors.Join(returned, os.WriteFile(filepath.Join(*output, "metadata.json"), append(raw, '\n'), 0644))
		}
	}()
	c := n.Config{Engine: "jsonata", Kind: "object", Strategy: mode, Fanout: *fanout, Workers: 1, ClientProcs: *procs, Rate: 5000, Count: int(duration.Seconds() * 5000), Queue: 64, Size: 1024}
	warm := c
	warm.Count = 32
	warm.Rate = 500
	pre, err := n.Run(ctx, broker.URL, warm)
	if err != nil {
		return err
	}
	if !pre.Lossless {
		return fmt.Errorf("diagnostic warmup failed")
	}
	var f *os.File
	if *tracing {
		f, err = os.Create(filepath.Join(*output, "trace.out"))
		if err != nil {
			return err
		}
		if err = trace.Start(f); err != nil {
			f.Close()
			return err
		}
	}
	report, runErr := n.Run(ctx, broker.URL, c)
	var closeErr error
	if *tracing {
		trace.Stop()
		closeErr = f.Close()
	}
	metadata["report"] = report
	if err = errors.Join(runErr, closeErr); err != nil {
		return err
	}
	if !report.Valid || report.TransformErrors != 0 {
		return fmt.Errorf("invalid diagnostic result")
	}
	metadata["completed"] = true
	return nil
}
