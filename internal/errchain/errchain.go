// Package errchain searches wrapped error trees without crossing ownership
// boundaries.
package errchain

// Find returns the first T in err's tree that accept admits. A T that accept
// rejects is not searched, so a diagnostic owned by someone else hides what it
// wraps.
func Find[T error](err error, accept func(T) bool) (T, bool) {
	var zero T
	if err == nil {
		return zero, false
	}
	if x, ok := err.(T); ok {
		if accept(x) {
			return x, true
		}
		return zero, false
	}
	switch u := err.(type) {
	case interface{ Unwrap() error }:
		return Find(u.Unwrap(), accept)
	case interface{ Unwrap() []error }:
		for _, e := range u.Unwrap() {
			if x, ok := Find(e, accept); ok {
				return x, true
			}
		}
	}
	return zero, false
}
