package javascript

import (
	"context"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"testing"
	"time"
)

func TestMemoryProbeRequiresExpectedOOM(t *testing.T) {
	oom := &Exception{Name: "InternalError", Message: "out of memory"}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false}, {"actual fixture OOM", oom, true},
		{"deadline", context.DeadlineExceeded, false}, {"cancellation", context.Canceled, false},
		{"unsupported", jt.ErrUnsupported, false}, {"closed", ErrClosed, false},
		{"unrelated", errors.New("out of memory"), false},
		{"other exception", &Exception{Name: "TypeError", Message: "out of memory"}, false},
		{"deadline joined to OOM", errors.Join(context.DeadlineExceeded, oom), false},
		{"closed joined to OOM", errors.Join(ErrClosed, oom), false},
		{"unsupported joined to OOM", errors.Join(jt.ErrUnsupported, oom), false},
	} {
		if got := memoryProbeRejection(tc.err); got != tc.want {
			t.Errorf("%s: accepted=%v want=%v", tc.name, got, tc.want)
		}
	}
	e, err := NewOwned(context.Background(), Config{GuestMemoryBytes: 8 << 20, Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	p, err := e.Compile(context.Background(), `()=>new ArrayBuffer(40*1024*1024).byteLength`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jt.EvalOptions{Limits: &jt.Limits{Timeout: time.Nanosecond}})
	if !errors.Is(err, context.DeadlineExceeded) || memoryProbeRejection(err) {
		t.Fatalf("real deadline accepted as cap evidence: %v", err)
	}
}
