package jsonata

import (
	"context"
	"time"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

// Value is a JSONata runtime value. JSON arrays are []any; Sequence represents
// the implicit result sequence and must not be confused with an explicit array.
type Value = any
type undefinedValue struct{}

var Undefined Value = undefinedValue{}

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
