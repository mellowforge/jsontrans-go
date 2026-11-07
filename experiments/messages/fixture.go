package messages

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"reflect"
	"strings"

	jt "github.com/mellowforge/jsontrans-go"
)

type Sample struct {
	Input []byte
	Want  []jt.JSONResult
}
type Fixture struct {
	Config  Config
	Samples []Sample
	Name    string
	Size    int
	Fanout  int
}

// NewFixture builds eight varied documents and independently computes outputs.
// Size is a target minimum; array fixtures intentionally contain many items.
func NewFixture(engine, kind string, size, fanout int) (Fixture, error) {
	if fanout < 1 || fanout > 16 || size < 128 || size > 1<<20 {
		return Fixture{}, fmt.Errorf("fixture bounds")
	}
	c := Config{Engine: engine, Configured: kind == "limits", Host: kind == "host"}
	var expr string
	switch engine {
	case "jsonata":
		switch kind {
		case "scalar", "limits":
			expr = "meta.id"
		case "object":
			expr = `{"name":customer.name,"code":meta.id}`
		case "array", "large-array":
			expr = "items[price >= 3].price"
		case "fallback":
			expr = "$uppercase(customer.name)"
		case "bindings":
			expr = "$subject & ':' & customer.name"
			c.Options.Bindings = map[string]any{"subject": "input.a"}
		case "host":
			expr = "$echo(meta.id)"
		case "output":
			expr = "$"
		default:
			return Fixture{}, fmt.Errorf("unknown fixture %s", kind)
		}
	case "jsonpath":
		switch kind {
		case "scalar", "limits":
			expr = "$.meta.id"
		case "object":
			expr = "$.customer"
		case "array", "large-array":
			expr = "$.items[?@.price >= 3].price"
		case "fallback":
			expr = "$.items[*].name"
		case "host":
			expr = "$.items[?echo(@.price) > 0].price"
		case "output":
			expr = "$"
		default:
			return Fixture{}, fmt.Errorf("unsupported fixture %s/%s", engine, kind)
		}
	case "javascript":
		if kind != "object" {
			return Fixture{}, fmt.Errorf("JavaScript baseline uses object fixture")
		}
		expr = "x=>({name:x.customer.name,code:x.meta.id})"
	default:
		return Fixture{}, fmt.Errorf("unknown engine %s", engine)
	}
	for slot := range fanout {
		selected := expr
		switch kind {
		case "scalar", "limits":
			if engine == "jsonata" {
				selected = fmt.Sprintf("metrics.v%d", slot)
			} else {
				selected = fmt.Sprintf("$.metrics.v%d", slot)
			}
		case "object":
			switch engine {
			case "jsonata":
				selected = fmt.Sprintf(`{"name":customer.name,"code":meta.id+%d}`, slot)
			case "jsonpath":
				selected = fmt.Sprintf("$.objects.o%d", slot)
			case "javascript":
				selected = fmt.Sprintf("x=>({name:x.customer.name,code:x.meta.id+%d})", slot)
			}
		case "array", "large-array":
			if engine == "jsonata" {
				selected = fmt.Sprintf("[items[price >= %d].price]", 3+slot)
			} else {
				selected = fmt.Sprintf("$.items[?@.price >= %d].price", 3+slot)
			}
		case "fallback":
			if engine == "jsonata" {
				selected = fmt.Sprintf(`$uppercase(customer.name) & ":%d"`, slot)
			} else {
				selected = fmt.Sprintf("$.items[0,%d].name", slot%4)
			}
		case "bindings":
			selected = fmt.Sprintf(`$subject & ':' & customer.name & ":%d"`, slot)
		case "host":
			if engine == "jsonata" {
				selected = fmt.Sprintf("$echo(metrics.v%d)", slot)
			} else {
				selected = fmt.Sprintf("$.items[?echo(@.price) > %d].price", slot+1)
			}
		}
		c.Expressions = append(c.Expressions, selected)
	}
	f := Fixture{Config: c, Name: engine + "/" + kind, Size: size, Fanout: fanout}
	for variant := 0; variant < 8; variant++ {
		count := 4 + variant%5
		if kind == "large-array" {
			count = size/64 + variant
		}
		items := make([]any, count)
		names := []any{}
		for i := range items {
			price := i + 1 + variant
			name := fmt.Sprintf("item-%d-世", i)
			items[i] = map[string]any{"price": price, "name": name, "active": i%2 == 0}
			names = append(names, name)
		}
		name := fmt.Sprintf("é-user-%d", variant)
		customer := map[string]any{"name": name, "code": variant}
		metrics := map[string]any{}
		for i := range 16 {
			metrics[fmt.Sprintf("v%d", i)] = variant*100 + i
		}
		data := map[string]any{"metrics": metrics, "meta": map[string]any{"id": variant}, "customer": customer, "items": items, "padding": ""}
		objects := map[string]any{}
		if kind == "object" {
			for slot := range 16 {
				objects[fmt.Sprintf("o%d", slot)] = map[string]any{"name": name, "code": variant + slot}
			}
			data["objects"] = objects
		}
		raw, err := json.Marshal(data)
		if err != nil {
			return Fixture{}, err
		}
		if len(raw) < size {
			// Some strings force JSON escaping; padding is still deterministic.
			padding := strings.Repeat("x", size-len(raw))
			if variant%2 == 1 && len(padding) > 10 {
				padding = padding[:len(padding)-10] + "<>&\n\\\"é"
			}
			data["padding"] = padding
			raw, err = json.Marshal(data)
			if err != nil {
				return Fixture{}, err
			}
		}
		sample := Sample{Input: raw, Want: make([]jt.JSONResult, fanout)}
		for slot := range fanout {
			var want any
			selected := []any{}
			for i := range items {
				price := i + 1 + variant
				if price >= 3+slot {
					selected = append(selected, price)
				}
			}
			if engine == "jsonpath" {
				switch kind {
				case "scalar", "limits":
					want = []any{variant*100 + slot}
				case "object":
					want = []any{objects[fmt.Sprintf("o%d", slot)]}
				case "array", "large-array":
					want = selected
				case "fallback":
					want = []any{names[0], names[slot%4]}
				case "host":
					values := []any{}
					for i := range items {
						price := i + 1 + variant
						if price > slot+1 {
							values = append(values, price)
						}
					}
					want = values
				case "output":
					want = []any{data}
				}
			} else {
				switch kind {
				case "scalar", "limits", "host":
					want = variant*100 + slot
				case "object":
					want = map[string]any{"name": name, "code": variant + slot}
				case "array", "large-array":
					want = selected
				case "fallback":
					want = strings.ToUpper(name) + fmt.Sprintf(":%d", slot)
				case "bindings":
					want = "input.a:" + name + fmt.Sprintf(":%d", slot)
				case "output":
					want = data
				}
			}
			encoded, err := json.Marshal(want)
			if err != nil {
				return Fixture{}, err
			}
			sample.Want[slot] = jt.JSONResult{Value: encoded, Defined: true}
		}
		f.Samples = append(f.Samples, sample)
	}
	return f, nil
}

func CheckOutputs(got, want []jt.JSONResult) error {
	if len(got) != len(want) {
		return fmt.Errorf("output count %d != %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Defined != want[i].Defined {
			return fmt.Errorf("output %d defined mismatch", i)
		}
		if !got[i].Defined {
			if len(got[i].Value) != 0 {
				return fmt.Errorf("missing output %d has bytes", i)
			}
			continue
		}
		if !json.Valid(got[i].Value) {
			return fmt.Errorf("output %d invalid JSON", i)
		}
		if !bytes.Equal(got[i].Value, want[i].Value) {
			// Permit object-key serialization order differences, preserving exact
			// number spelling, array order and isolated UTF-16 code units.
			b, err := budget.New(context.Background(), budget.Limits{})
			if err != nil {
				return err
			}
			a, ea := jsonvalue.Decode(got[i].Value, b)
			b.Close()
			b, err = budget.New(context.Background(), budget.Limits{})
			if err != nil {
				return err
			}
			expected, eb := jsonvalue.Decode(want[i].Value, b)
			b.Close()
			if ea != nil || eb != nil || !reflect.DeepEqual(a, expected) {
				return fmt.Errorf("output %d: got %s want %s", i, got[i].Value, want[i].Value)
			}
		}
	}
	return nil
}
