//go:build integration

package natsexperiment

import (
	"context"
	"testing"
	"time"

	m "github.com/mellowforge/jsontrans-go/experiments/messages"
)

func TestOneWorkerWithIndependentRuntimeProcs(t *testing.T) {
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
	r, err := Run(ctx, broker.URL, Config{Engine: "jsonata", Kind: "object", Strategy: m.Prepared, Fanout: 4, Workers: 1, ClientProcs: 2, Rate: 500, Count: 128, Queue: 64, Size: 1024})
	if err != nil || !r.Valid || !r.Lossless || r.GOMAXPROCS != 2 || r.Config.Workers != 1 {
		t.Fatalf("runtime/worker control: %#v %v", r, err)
	}
}
