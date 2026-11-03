package jsonata

import (
	"context"
	"math"
	"time"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

// Value is a JSONata runtime value. JSON arrays are []any; Sequence represents
// the implicit result sequence and must not be confused with an explicit array.
type Value = any
type undefinedValue uint8

// Undefined is JSONata's missing value, distinct from JSON null. It is a
// constant, so no package can redefine what missing means.
const Undefined = undefinedValue(0)

// IsUndefined reports whether v is the missing value.
func IsUndefined(v Value) bool { _, ok := v.(undefinedValue); return ok }

type Sequence []Value

// Callable is implemented by native functions and JSONata closures.
type Callable interface {
	Arity() int
	Call(*CallContext, []Value) (Value, error)
}

// CallContext belongs to one evaluation. Native functions must cooperate with
// cancellation and use Budget for work proportional to input size.
type CallContext struct {
	Context  context.Context
	Input    Value
	Focus    Value
	Budget   *budget.Budget
	Now      time.Time
	Invoke   func(Callable, []Value) (Value, error)
	Evaluate func(string, Value) (Value, error)
	// ObjectKeys returns construction order when available, or nil when unknown.
	ObjectKeys func(Value) []string
	state      *evaluator
}

// NativeFunction is the adapter for implementations of standard functions.
type NativeFunction struct {
	N  int
	Fn func(*CallContext, []Value) (Value, error)
}

func (f NativeFunction) Arity() int                                       { return f.N }
func (f NativeFunction) Call(c *CallContext, args []Value) (Value, error) { return f.Fn(c, args) }

// floatToInt converts f identically on every platform: NaN is 0 and values
// beyond the int range saturate. Go leaves such conversions
// implementation-defined, and amd64 differs from arm64.
func floatToInt(f float64) int {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= float64(math.MaxInt):
		return math.MaxInt
	case f <= float64(math.MinInt):
		return math.MinInt
	}
	return int(f)
}

// floatToInt64 is floatToInt for int64 results.
func floatToInt64(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= float64(math.MaxInt64):
		return math.MaxInt64
	case f <= float64(math.MinInt64):
		return math.MinInt64
	}
	return int64(f)
}
