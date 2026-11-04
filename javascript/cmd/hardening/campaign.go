package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	js "github.com/mellowforge/jsontrans-go/javascript"
)

type campaignSettings struct {
	Duration       time.Duration `json:"duration_ns"`
	Recycle        time.Duration `json:"recycle_ns"`
	SampleInterval time.Duration `json:"sample_interval_ns"`
	Workers        int           `json:"workers"`
	GuestMiB       int           `json:"guest_mib"`
}

type campaignProgress struct {
	Status     string        `json:"status"`
	ElapsedNS  int64         `json:"elapsed_ns"`
	Recoveries int           `json:"verified_recoveries"`
	Lifecycles int           `json:"closed_lifecycles"`
	Sample     *memorySample `json:"sample,omitempty"`
	Failure    string        `json:"failure,omitempty"`
}

func (s campaignSettings) validate() error {
	if s.Duration < time.Second || s.Duration > 2*time.Hour || s.SampleInterval < time.Second || s.SampleInterval > s.Duration || s.Recycle < 0 || (s.Recycle > 0 && (s.Recycle < time.Second || s.Recycle > s.Duration)) || s.Workers < 1 || s.Workers > 8 || (s.GuestMiB != 2 && s.GuestMiB != 8 && s.GuestMiB != 64) {
		return errors.New("invalid campaign settings: duration 1s..2h; sample/recycle interval 1s..duration (recycle 0 means persistent); workers 1..8; guest MiB 2, 8 or 64")
	}
	return nil
}

// Every batch is fully checked before normal duration expiry is considered.
// Cancellation, watchdog expiry, IO errors and cleanup errors are failures.
func runCampaign(ctx context.Context, s campaignSettings, observer func(campaignProgress) error) (r report, err error) {
	complete := false
	r = report{GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, Settings: settings{Iterations: 25, Workers: s.Workers}, GuestCapBytes: s.GuestMiB << 20, Outcomes: make(map[string]outcome), Campaign: &s, Status: "running", Complete: &complete}
	started := time.Now()
	var engine *js.Engine
	emit := func(m *memorySample) error {
		if observer == nil {
			return nil
		}
		return observer(campaignProgress{Status: r.Status, ElapsedNS: time.Since(started).Nanoseconds(), Recoveries: r.Recoveries, Lifecycles: len(r.CloseNS), Sample: m, Failure: r.Failure})
	}
	snapshot := func(stage string) error {
		m := sample(len(r.CloseNS))
		m.Stage = stage
		m.ElapsedNS = time.Since(started).Nanoseconds()
		r.Memory = append(r.Memory, m)
		return emit(&m)
	}
	closeEngine := func() error {
		if engine == nil {
			return nil
		}
		closing := engine
		engine = nil
		before := time.Now()
		closeErr := closing.Close()
		r.CloseNS = append(r.CloseNS, time.Since(before).Nanoseconds())
		r.Settings.Cycles = len(r.CloseNS)
		if _, admissionErr := closing.Compile(context.Background(), `()=>1`); !errors.Is(admissionErr, js.ErrClosed) {
			closeErr = errors.Join(closeErr, fmt.Errorf("closed engine admitted compilation: %v", admissionErr))
		}
		return errors.Join(closeErr, snapshot("closed"))
	}
	defer func() {
		cleanupErr := closeEngine()
		err = errors.Join(err, cleanupErr, context.Cause(ctx))
		r.ElapsedNS = time.Since(started).Nanoseconds()
		if err == nil {
			complete = true
			r.Status = "passed"
		} else {
			r.Status = "failed"
			r.Failure = err.Error()
		}
		if finalErr := emit(nil); finalErr != nil {
			err = errors.Join(err, finalErr)
			complete = false
			r.Status = "failed"
			r.Failure = err.Error()
		}
	}()
	if err = s.validate(); err != nil {
		return r, err
	}
	if err = context.Cause(ctx); err != nil {
		return r, err
	}
	if err = snapshot("baseline"); err != nil {
		return r, err
	}
	var created, lastSample time.Time
	for {
		if err = context.Cause(ctx); err != nil {
			return r, err
		}
		if engine == nil {
			before := time.Now()
			engine, err = js.New(ctx, js.Config{GuestMemoryBytes: r.GuestCapBytes, Limits: jt.Limits{Timeout: 10 * time.Second}})
			if err != nil {
				return r, err
			}
			r.ConstructionNS = append(r.ConstructionNS, time.Since(before).Nanoseconds())
			created = time.Now()
			lastSample = created
			if err = snapshot("live"); err != nil {
				return r, err
			}
		}
		if err = exercise(ctx, engine, settings{Iterations: 25, Workers: s.Workers, Cycles: 1}, &r); err != nil {
			return r, err
		}
		r.Batches++
		now := time.Now()
		if now.Sub(lastSample) >= s.SampleInterval {
			if err = snapshot("live"); err != nil {
				return r, err
			}
			lastSample = time.Now()
		}
		if time.Since(started) >= s.Duration {
			return r, nil
		}
		if s.Recycle > 0 && now.Sub(created) >= s.Recycle {
			if err = closeEngine(); err != nil {
				return r, err
			}
		}
	}
}

func campaignCLI(s campaignSettings, path string) (r report, err error) {
	if err = s.validate(); err != nil {
		return runCampaign(context.Background(), s, nil)
	}
	var progress *os.File
	var observer func(campaignProgress) error
	if path != "" {
		progress, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			r, _ = runCampaign(context.Background(), s, func(campaignProgress) error { return err })
			return r, err
		}
		observer = func(p campaignProgress) error { return json.NewEncoder(progress).Encode(p) }
	}
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signals, s.Duration+30*time.Second)
	defer cancel()
	r, err = runCampaign(ctx, s, observer)
	if progress != nil {
		if closeErr := progress.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
			*r.Complete = false
			r.Status = "failed"
			r.Failure = err.Error()
		}
	}
	return r, err
}
