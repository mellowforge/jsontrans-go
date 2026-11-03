package budget

import (
	"context"
	"testing"
	"time"
)

func TestFastContextDeadlineClockModes(t *testing.T) {
	now := time.Now()
	for name, deadline := range map[string]time.Time{
		"monotonic_past":   now.Add(-time.Second),
		"wall_past":        now.Add(-time.Second).Round(0),
		"monotonic_future": now.Add(time.Hour),
		"wall_future":      now.Add(time.Hour).Round(0),
	} {
		t.Run(name, func(t *testing.T) {
			c := &deadlineContext{parent: context.Background(), deadline: deadline}
			expired := deadline.Before(now)
			got := c.Err()
			if expired && got != context.DeadlineExceeded {
				t.Fatal(got)
			}
			if !expired && got != nil {
				t.Fatal(got)
			}
			c.close()
			want := error(context.Canceled)
			if expired {
				want = context.DeadlineExceeded
			}
			if c.Err() != want {
				t.Fatal(c.Err())
			}
			select {
			case <-c.Done():
			default:
				t.Fatal("terminal context kept Done open")
			}
		})
	}
}
