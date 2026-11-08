package main

import (
	"reflect"
	"testing"
	"time"

	n "github.com/mellowforge/jsontrans-go/experiments/messages/nats"
)

func TestFocusedObjectConfiguration(t *testing.T) {
	got, err := loadConfigurations("object-one-worker", 10*time.Second, false)
	want := []n.Config{{Engine: "jsonata", Kind: "object", Fanout: 1, Workers: 1, Rate: 5000, Count: 50000, Queue: 64, Size: 1024}, {Engine: "jsonata", Kind: "object", Fanout: 4, Workers: 1, Rate: 5000, Count: 50000, Queue: 64, Size: 1024}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v %v, want %#v", got, err, want)
	}
	for _, tc := range []struct {
		profile  string
		duration time.Duration
		smoke    bool
	}{{"unknown", time.Second, false}, {"full", 6 * time.Second, false}, {"object-one-worker", 11 * time.Second, false}, {"object-one-worker", 10 * time.Second, true}, {"object-one-worker", 0, false}, {"object-one-worker", time.Nanosecond, false}} {
		if _, err := loadConfigurations(tc.profile, tc.duration, tc.smoke); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	full, err := loadConfigurations("full", time.Second, false)
	if err != nil || len(full) != 54 || full[0].Count != 1000 || full[1].Count != 5000 {
		t.Fatalf("default matrix changed: %d %v", len(full), err)
	}
	smoke, err := loadConfigurations("full", time.Second, true)
	if err != nil || len(smoke) != 2 || smoke[0].Count != 128 {
		t.Fatalf("smoke changed: %#v %v", smoke, err)
	}
}
