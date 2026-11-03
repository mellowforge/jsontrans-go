package jsonata

// environment holds the immutable function snapshots owned by a compiled program.
// Lexical writes live in the evaluation's local map and shadow both snapshots.
type environment struct {
	builtins  map[string]Callable
	functions map[string]Callable
}

func (e *environment) lookup(local map[string]Value, name string) (Value, bool) {
	if v, ok := local[name]; ok {
		return v, true
	}
	if v, ok := e.functions[name]; ok {
		return v, true
	}
	v, ok := e.builtins[name]
	return v, ok
}
