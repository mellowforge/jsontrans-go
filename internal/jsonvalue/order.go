package jsonvalue

import (
	"context"
	"reflect"
	"sort"
	"strconv"
	"sync"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

type orderKey struct{}
type objectOrderEntry struct {
	object map[string]any
	keys   []string
}

// ObjectOrderEntry is immutable provenance for a retained JSON object.
type ObjectOrderEntry struct {
	Object map[string]any
	Keys   []string
}
type objectOrder struct {
	mu      sync.RWMutex
	objects map[uintptr]objectOrderEntry
}

// WithObjectOrder records serialized member order for a single JSON request.
// Cloning propagates provenance; Go map inputs without provenance stay unordered.
func WithObjectOrder(ctx context.Context) context.Context {
	return context.WithValue(ctx, orderKey{}, &objectOrder{objects: make(map[uintptr]objectOrderEntry)})
}
func RememberObject(ctx context.Context, obj map[string]any, keys []string) {
	order, _ := ctx.Value(orderKey{}).(*objectOrder)
	if order == nil || obj == nil {
		return
	}
	entry := objectOrderEntry{obj, append([]string(nil), keys...)}
	order.mu.Lock()
	order.objects[uintptr(reflect.ValueOf(obj).UnsafePointer())] = entry
	order.mu.Unlock()
}
func ObjectKeys(ctx context.Context, value any) []string {
	order, _ := ctx.Value(orderKey{}).(*objectOrder)
	if order == nil {
		return nil
	}
	var obj map[string]any
	switch v := value.(type) {
	case map[string]any:
		obj = v
	case UTF16Object:
		obj = map[string]any(v)
	default:
		return nil
	}
	order.mu.RLock()
	entry, ok := order.objects[uintptr(reflect.ValueOf(obj).UnsafePointer())]
	order.mu.RUnlock()
	if !ok {
		return nil
	}
	return EnumerationKeys(entry.keys)
}

// VisitObjectOrders transfers retained provenance without allocating an
// unbounded snapshot before the receiving evaluation can enforce its budget.
func VisitObjectOrders(ctx context.Context, b *budget.Budget, visit func(ObjectOrderEntry) error) error {
	order, _ := ctx.Value(orderKey{}).(*objectOrder)
	if order == nil {
		return nil
	}
	order.mu.RLock()
	defer order.mu.RUnlock()
	for _, entry := range order.objects {
		if err := b.Step(); err != nil {
			return err
		}
		if err := b.Collection(len(entry.keys)); err != nil {
			return err
		}
		if err := visit(ObjectOrderEntry{entry.object, EnumerationKeys(entry.keys)}); err != nil {
			return err
		}
	}
	return b.Check()
}

// EnumerationKeys follows ECMAScript's integer-index-first enumeration rule;
// other keys retain their insertion order. The input slice is never modified.
func EnumerationKeys(keys []string) []string {
	indexed := make([]string, 0, len(keys))
	ordinary := make([]string, 0, len(keys))
	for _, key := range keys {
		n, err := strconv.ParseUint(key, 10, 32)
		if err == nil && n < 1<<32-1 && strconv.FormatUint(n, 10) == key {
			indexed = append(indexed, key)
		} else {
			ordinary = append(ordinary, key)
		}
	}
	sort.Slice(indexed, func(i, j int) bool {
		a, _ := strconv.ParseUint(indexed[i], 10, 32)
		b, _ := strconv.ParseUint(indexed[j], 10, 32)
		return a < b
	})
	return append(indexed, ordinary...)
}
