//go:build integration

package natsexperiment

import (
	"context"
	m "github.com/mellowforge/jsontrans-go/experiments/messages"
	"testing"
	"time"
)

func TestContainerRoutes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	broker, err := StartBroker(ctx, DefaultImage, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, fanout := range []int{1, 4, 16} {
		for _, s := range []m.Strategy{m.Direct, m.Prepared, m.Batched} {
			report, err := Run(ctx, broker.URL, Config{Engine: "mixed", Kind: "mixed", Strategy: s, Fanout: fanout, Workers: 4, Rate: 1000, Count: 256, Queue: 256, Size: 1024})
			if err != nil {
				t.Fatal(err)
			}
			if !report.Valid || !report.Lossless || report.Completed != 256 || report.Outputs != 256*fanout {
				t.Fatalf("%s: %+v", s, report)
			}
		}
	}
}
