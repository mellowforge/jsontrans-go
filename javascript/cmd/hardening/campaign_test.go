package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCampaignAccountsForCallsAndCleanup(t *testing.T) {
	var events []campaignProgress
	r, err := runCampaign(context.Background(), campaignSettings{Duration: time.Second, SampleInterval: time.Second, Workers: 1, GuestMiB: 8}, func(p campaignProgress) error { events = append(events, p); return nil })
	if err != nil || r.Status != "passed" || r.Complete == nil || !*r.Complete {
		t.Fatalf("campaign: %+v, %v", r, err)
	}
	calls := 0
	for _, kind := range []string{"success", "exception", "conversion", "cancellation", "allocation"} {
		if r.Outcomes[kind].Calls == 0 {
			t.Fatalf("missing %s", kind)
		}
		calls += r.Outcomes[kind].Calls
	}
	if calls != r.Recoveries || calls < 25 || len(r.ConstructionNS) != len(r.CloseNS) || len(r.CloseNS) == 0 {
		t.Fatalf("incomplete accounting: %+v", r)
	}
	stages := map[string]bool{}
	for _, m := range r.Memory {
		stages[m.Stage] = true
	}
	if !stages["baseline"] || !stages["live"] || !stages["closed"] {
		t.Fatalf("missing memory stages: %v", stages)
	}
	if len(events) < 3 || events[len(events)-1].Status != "passed" || events[len(events)-1].Recoveries != r.Recoveries {
		t.Fatalf("bad progress: %+v", events)
	}
	if time.Duration(r.ElapsedNS) < time.Second {
		t.Fatalf("campaign ended early: %d", r.ElapsedNS)
	}
}

func TestCampaignCancellationDuringCleanupIsIncomplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := runCampaign(ctx, campaignSettings{Duration: time.Second, SampleInterval: time.Second, Workers: 1, GuestMiB: 8}, func(p campaignProgress) error {
		if p.Sample != nil && p.Sample.Stage == "closed" {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || r.Status != "failed" || r.Complete == nil || *r.Complete || r.Recoveries == 0 {
		t.Fatalf("cleanup cancellation passed: %+v, %v", r, err)
	}
}

func TestCampaignProgressNeverOverwritesEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.jsonl")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := campaignCLI(campaignSettings{Duration: time.Second, SampleInterval: time.Second, Workers: 1, GuestMiB: 8}, path)
	if err == nil || r.Status != "failed" || r.Complete == nil || *r.Complete {
		t.Fatalf("existing progress file accepted: %+v, %v", r, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep" {
		t.Fatalf("previous evidence changed: %q %v", data, err)
	}
}

func TestCampaignRejectsInvalidSettings(t *testing.T) {
	good := campaignSettings{Duration: time.Second, SampleInterval: time.Second, Workers: 1, GuestMiB: 8}
	for _, change := range []func(*campaignSettings){
		func(s *campaignSettings) { s.Duration = 0 }, func(s *campaignSettings) { s.Duration = 3 * time.Hour },
		func(s *campaignSettings) { s.SampleInterval = 0 }, func(s *campaignSettings) { s.Recycle = -1 },
		func(s *campaignSettings) { s.Recycle = 2 * time.Second }, func(s *campaignSettings) { s.Workers = 0 },
		func(s *campaignSettings) { s.Workers = 9 }, func(s *campaignSettings) { s.GuestMiB = 3 },
	} {
		s := good
		change(&s)
		if _, err := runCampaign(context.Background(), s, nil); err == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
}

func TestCampaignPreservesCancellationAndObserverFailure(t *testing.T) {
	cfg := campaignSettings{Duration: time.Second, SampleInterval: time.Second, Workers: 1, GuestMiB: 8}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := runCampaign(ctx, cfg, nil)
	if !errors.Is(err, context.Canceled) || r.Status != "failed" || r.Complete == nil || *r.Complete {
		t.Fatalf("canceled campaign: %+v, %v", r, err)
	}
	sentinel := errors.New("progress write failed")
	r, err = runCampaign(context.Background(), cfg, func(campaignProgress) error { return sentinel })
	if !errors.Is(err, sentinel) || r.Status != "failed" || r.Complete == nil || *r.Complete || r.Failure == "" || r.Recoveries != 0 {
		t.Fatalf("observer failure: %+v, %v", r, err)
	}
}
