package javascript

import (
	"context"
	"errors"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
)

func TestResourceFailureOracleRejectsUnrelatedCauses(t *testing.T) {
	limit := &jt.Error{Phase: jt.PhaseOutput, Cause: errors.Join(jt.ErrLimit, jt.ErrConversion)}
	oom := &jt.Error{Phase: jt.PhaseEvaluate, Cause: &Exception{Name: "InternalError", Message: "out of memory"}}
	for _, tc := range []struct {
		err    error
		native bool
	}{{limit, false}, {oom, true}} {
		if !resourceFailureMatches(tc.err, tc.native) {
			t.Fatalf("valid fixture failure rejected: %v", tc.err)
		}
		for _, cause := range []error{context.Canceled, context.DeadlineExceeded, ErrClosed} {
			if resourceFailureMatches(errors.Join(tc.err, cause), tc.native) {
				t.Errorf("unexpected joined cause accepted: %v", cause)
			}
		}
	}
	for _, err := range []error{
		nil, errors.New("unrelated"),
		&jt.Error{Phase: jt.PhaseEvaluate, Cause: &Exception{Name: "InternalError", Message: "unrelated internal failure"}},
		&jt.Error{Phase: jt.PhaseOutput, Cause: &Exception{Name: "InternalError", Message: "out of memory"}},
		&jt.Error{Phase: jt.PhaseEvaluate, Cause: errors.Join(&Exception{Name: "InternalError", Message: "out of memory"}, jt.ErrLimit)},
	} {
		if resourceFailureMatches(err, true) {
			t.Errorf("unrelated OOM failure accepted: %v", err)
		}
	}
}
