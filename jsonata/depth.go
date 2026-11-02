package jsonata

import (
	"fmt"
	jt "github.com/mellowforge/jsontrans-go"
)

// depthTracker accounts expression frames, independently of conversion and
// collection traversal. It is evaluation-local and shared by nested evaluation.
type depthTracker struct{ limit, current int }

func newDepthTracker(limit int) *depthTracker { return &depthTracker{limit: limit} }
func (d *depthTracker) enter() error {
	if d == nil || d.limit == 0 {
		return nil
	}
	if d.current >= d.limit {
		return &jt.Error{Processor: "jsonata", Phase: "evaluate", Code: "U1001", Cause: fmt.Errorf("%w: expression depth", jt.ErrLimit)}
	}
	d.current++
	return nil
}
func (d *depthTracker) leave() {
	if d != nil && d.limit > 0 {
		d.current--
	}
}
