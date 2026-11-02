package jsonata

import (
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"testing"
)

func TestExpressionDepthGuard(t *testing.T) {
	tracker := newDepthTracker(2)
	if err := tracker.enter(); err != nil {
		t.Fatal(err)
	}
	if err := tracker.enter(); err != nil {
		t.Fatal(err)
	}
	err := tracker.enter()
	var native *jt.Error
	if !errors.As(err, &native) || native.Code != "U1001" || !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("%v", err)
	}
	tracker.leave()
	if err := tracker.enter(); err != nil {
		t.Fatal(err)
	}
}
