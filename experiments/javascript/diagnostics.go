package javascript

import (
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/experiments/javascript/internal/quickjswasm"
)

// SourceLocation describes original rule coordinates, when available. Runtime
// coordinates are reported data and are not authenticated execution locations.
type SourceLocation struct {
	Source       string
	Line, Column int
	ByteOffset   int // -1 when unavailable
	Reported     bool
}

// Exception retains bounded JS diagnostic text. Name and runtime stack are
// user-modifiable data. Strings use lossless WTF-8 for isolated UTF-16 units.
type Exception struct {
	Name, Message, Stack string
	Location             *SourceLocation
	Truncated            bool
}

func (e *Exception) Error() string {
	if e.Name != "" {
		return e.Name + ": " + e.Message
	}
	if e.Message != "" {
		return e.Message
	}
	return "JavaScript exception"
}
func ownedDiagnostic(phase string, err error) error {
	if err == nil {
		return nil
	}
	var existing *jt.Error
	if errors.As(err, &existing) && existing.Processor == "javascript" {
		return err
	}
	var f *quickjswasm.Failure
	cause := err
	if errors.As(err, &f) {
		phase = f.Stage
		switch f.Kind {
		case "exception":
			cause = &Exception{Name: f.Name, Message: f.Message, Stack: f.Stack, Truncated: f.Truncated}
		case "conversion":
			cause = errors.Join(jt.ErrConversion, err)
		case "limit":
			cause = errors.Join(jt.ErrLimit, err)
			if phase == jt.PhaseOutput {
				cause = errors.Join(jt.ErrConversion, cause)
			}
		}
	}
	return &jt.Error{Processor: "javascript", Phase: phase, Cause: cause}
}
func ownedPhase(backend Backend, phase string, err error) error {
	if backend == OwnedQuickJS {
		return ownedDiagnostic(phase, err)
	}
	return err
}
