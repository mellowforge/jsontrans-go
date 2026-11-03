package javascript

import (
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/javascript/internal/quickjswasm"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
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

var ruleFrame = regexp.MustCompile(`(?m)^    at (?:[^()\r\n]+ \()?rule\.js:([0-9]+):([0-9]+)\)?$`)

func mapRuleLocation(expression, source string, line, column int, reported bool) *SourceLocation {
	if source != "rule.js" || line < 2 || column < 1 {
		return nil
	}
	line--
	offset := 0
	remaining := expression
	for i := 1; i < line; i++ {
		n := strings.IndexByte(remaining, '\n')
		if n < 0 {
			return nil
		}
		offset += n + 1
		remaining = remaining[n+1:]
	}
	last := strings.IndexByte(remaining, '\n')
	if last < 0 {
		last = len(remaining)
	}
	if last > 0 && remaining[last-1] == '\r' {
		last--
	}
	if column-1 > last {
		return nil
	}
	at := offset + column - 1
	if at < len(expression) && !utf8.RuneStart(expression[at]) {
		return nil
	}
	location := &SourceLocation{Source: source, Line: line, Column: column, ByteOffset: -1, Reported: reported}
	// The pinned compiler reports column 1 for token errors anywhere on a line.
	// Preserve its coordinates, but do not invent byte offsets from coarse data.
	return location
}
func mapOwnedDiagnostic(expression string, err error) {
	var ex *Exception
	var d *jt.Error
	if !errors.As(err, &ex) || !errors.As(err, &d) {
		return
	}
	matches := ruleFrame.FindAllStringSubmatch(ex.Stack, 64)
	for _, m := range matches {
		line, e1 := strconv.Atoi(m[1])
		column, e2 := strconv.Atoi(m[2])
		if e1 != nil || e2 != nil {
			continue
		}
		l := mapRuleLocation(expression, "rule.js", line, column, d.Phase != jt.PhaseCompile)
		if l == nil {
			continue
		}
		ex.Location = l
		if l.ByteOffset >= 0 {
			d.Position = l.ByteOffset
		}
		return
	}
}
