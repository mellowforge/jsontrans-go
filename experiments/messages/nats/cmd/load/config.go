package main

import (
	"fmt"
	"time"

	n "github.com/mellowforge/jsontrans-go/experiments/messages/nats"
)

// Profiles change only the development load matrix; evaluation limits and the
// library's production execution paths are unaffected.
func loadConfigurations(profile string, duration time.Duration, smoke bool) ([]n.Config, error) {
	maxDuration := 5 * time.Second
	switch profile {
	case "full":
	case "object-one-worker":
		maxDuration = 10 * time.Second
		if duration < time.Second {
			return nil, fmt.Errorf("focused duration must be at least one second")
		}
		if smoke {
			return nil, fmt.Errorf("focused profile cannot be combined with smoke")
		}
	default:
		return nil, fmt.Errorf("unknown workload profile %q", profile)
	}
	if duration <= 0 || duration > maxDuration {
		return nil, fmt.Errorf("invalid duration for %s profile", profile)
	}
	if smoke {
		return []n.Config{{Engine: "mixed", Kind: "mixed", Fanout: 1, Workers: 2, Rate: 500, Count: 128, Queue: 128, Size: 1024}, {Engine: "mixed", Kind: "mixed", Fanout: 4, Workers: 2, Rate: 500, Count: 128, Queue: 128, Size: 1024}}, nil
	}
	if profile == "object-one-worker" {
		out := []n.Config{}
		for _, count := range []int{1, 4} {
			out = append(out, n.Config{Engine: "jsonata", Kind: "object", Fanout: count, Workers: 1, Rate: 5000, Count: max(1, int(duration.Seconds()*5000)), Queue: 64, Size: 1024})
		}
		return out, nil
	}
	out := []n.Config{}
	for _, family := range []string{"scalar", "object", "mixed"} {
		for _, fanout := range []int{1, 4, 16} {
			for _, workers := range []int{1, 4, 8} {
				for _, rate := range []int{1000, 5000} {
					engine := "jsonata"
					if family == "mixed" {
						engine = "mixed"
					}
					out = append(out, n.Config{Engine: engine, Kind: family, Fanout: fanout, Workers: workers, Rate: rate, Count: max(1, int(duration.Seconds()*float64(rate))), Queue: workers * 64, Size: 1024})
				}
			}
		}
	}
	return out, nil
}
