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
	"strings"
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
	image := flag.String("image", n.DefaultImage, "immutable NATS image digest")
	output := flag.String("output", "", "new report directory")
	rounds := flag.Int("rounds", 3, "repeated process-local rounds")
	duration := flag.Duration("duration", time.Second, "offered publish window per case")
	smoke := flag.Bool("smoke", false, "small lossless transport validation")
	brokerCPUs := flag.String("broker-cpus", "16,17", "broker affinity on Bertram; empty disables")
	flag.Parse()
	if *output == "" || *rounds < 1 || *rounds > 6 || *duration <= 0 || *duration > 5*time.Second {
		return fmt.Errorf("invalid output/round/duration")
	}
	if err := os.Mkdir(*output, 0755); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	startup, stop := context.WithTimeout(ctx, 2*time.Minute)
	broker, err := n.StartBroker(startup, *image, *brokerCPUs)
	stop()
	if err != nil {
		return err
	}
	metadata := map[string]any{"image": *image, "container_id": broker.Container.GetContainerID(), "broker_cpus": *brokerCPUs, "broker_memory_bytes": 512 << 20, "client_affinity": "external driver pins performance cores", "go": runtime.Version(), "rounds": *rounds, "duration": duration.String(), "completed": false, "load": "paced deterministic arrivals; all completed-output percentiles exclude rejected/failed inputs; loss counters retained", "mode": "Core NATS, no persistence, TLS or acknowledgements; trace/debug disabled"}
	if info, ok := debug.ReadBuildInfo(); ok {
		metadata["build_info"] = info
	}
	defer func() {
		cleanupError := broker.Close()
		metadata["container_removed"] = cleanupError == nil
		if cleanupError != nil {
			metadata["cleanup_error"] = cleanupError.Error()
			returned = errors.Join(returned, cleanupError)
		}
		if returned != nil {
			metadata["completed"] = false
			metadata["error"] = returned.Error()
		}
		raw, marshalError := json.MarshalIndent(metadata, "", "  ")
		returned = errors.Join(returned, marshalError)
		if marshalError == nil {
			returned = errors.Join(returned, os.WriteFile(filepath.Join(*output, "metadata.json"), append(raw, '\n'), 0644))
		}
	}()
	inspect, err := broker.Container.Inspect(ctx)
	if err != nil {
		return err
	}
	metadata["container_inspect"] = inspect
	for _, bindings := range inspect.NetworkSettings.Ports {
		for _, binding := range bindings {
			if !binding.HostIP.IsLoopback() {
				return fmt.Errorf("broker port is not loopback-only: %s", binding.HostIP)
			}
		}
	}
	configs := []n.Config{}
	for _, family := range []string{"scalar", "object", "mixed"} {
		for _, fanout := range []int{1, 4, 16} {
			for _, workers := range []int{1, 4, 8} {
				for _, rate := range []int{1000, 5000} {
					kind, engine := family, "jsonata"
					if family == "mixed" {
						kind = "mixed"
						engine = "mixed"
					}
					count := int(duration.Seconds() * float64(rate))
					if count < 1 {
						count = 1
					}
					configs = append(configs, n.Config{Engine: engine, Kind: kind, Fanout: fanout, Workers: workers, Rate: rate, Count: count, Queue: workers * 64, Size: 1024})
				}
			}
		}
	}
	if *smoke {
		configs = []n.Config{{Engine: "mixed", Kind: "mixed", Fanout: 1, Workers: 2, Rate: 500, Count: 128, Queue: 128, Size: 1024}, {Engine: "mixed", Kind: "mixed", Fanout: 4, Workers: 2, Rate: 500, Count: 128, Queue: 128, Size: 1024}}
	}
	file, err := os.Create(filepath.Join(*output, "runs.jsonl"))
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, file.Close()) }()
	enc := json.NewEncoder(file)
	for round := 0; round < *rounds; round++ {
		// Rotate both configuration order and strategy order across repetitions.
		offset := round % len(configs)
		order := append(append([]n.Config(nil), configs[offset:]...), configs[:offset]...)
		for caseIndex, config := range order {
			for si := 0; si < 3; si++ {
				strategy := []m.Strategy{m.Direct, m.Prepared, m.Batched}[(si+round)%3]
				config.Strategy = strategy
				warm := config
				warm.Count = 32
				warm.Rate = 500
				report, err := n.Run(ctx, broker.URL, warm)
				if err != nil {
					return fmt.Errorf("warmup: %w", err)
				}
				if !report.Lossless {
					return fmt.Errorf("warmup failed: %+v", report)
				}
				report, err = n.Run(ctx, broker.URL, config)
				row := map[string]any{"round": round + 1, "case_index": caseIndex, "report": report}
				if err != nil {
					row["error"] = err.Error()
				}
				if e := enc.Encode(row); e != nil {
					return e
				}
				if e := file.Sync(); e != nil {
					return e
				}
				fmt.Fprintf(os.Stderr, "ROUND %d %s/%s N%d W%d R%d %s complete=%d/%d rejected=%d valid=%v\n", round+1, config.Engine, config.Kind, config.Fanout, config.Workers, config.Rate, strategy, report.Completed, report.Sent, report.QueueRejected, report.Valid)
				if err != nil {
					return err
				}
				if !report.Valid {
					return fmt.Errorf("invalid transport result: %+v", report)
				}
				if *smoke && !report.Lossless {
					return fmt.Errorf("smoke must be lossless")
				}
			}
		}
	}
	metadata["completed"] = true
	metadata["cases_per_round"] = len(configs) * 3
	if *image == n.DefaultImage {
		metadata["image_version_tag"] = "nats:2.15.2-alpine"
	}
	metadata["notes"] = strings.Join([]string{"One broker reused; broker setup/image pulls/compilation/warmup excluded", "Eight subject-specific plans per case; immutable shared state, worker-local scratch", "Partial transport delivery is not transactional; transform errors publish no outputs"}, ". ")
	return nil
}
