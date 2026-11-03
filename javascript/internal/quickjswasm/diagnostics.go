package quickjswasm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

// Failure is produced only by the versioned trusted guest boundary.
type Failure struct {
	Kind, Stage, Name, Message, Stack, Source string
	Line, Column                              int
	Truncated                                 bool
}

func (f *Failure) Error() string { return f.Kind + ": " + f.Message }
func decodeFailure(ctx context.Context, data []byte) (*Failure, error) {
	invalid := func() (*Failure, error) { return nil, errors.New("invalid owned diagnostic protocol") }
	if len(data) > 65536 {
		return invalid()
	}
	l := budget.DefaultLimits()
	l.MaxInputBytes = 65536
	l.MaxDepth = 4
	l.MaxCollectionItems = 10
	b, err := budget.New(ctx, l)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	v, err := jsonvalue.Decode(data, b)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("owned diagnostic protocol: %v", err)
	}
	fields, ok := v.(map[string]any)
	if !ok {
		return invalid()
	}
	// Check duplicate names independently of lossless object materialization.
	d := json.NewDecoder(bytes.NewReader(data))
	if _, err = d.Token(); err != nil {
		return invalid()
	}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return invalid()
		}
		s, ok := key.(string)
		if !ok || seen[s] {
			return invalid()
		}
		seen[s] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return invalid()
		}
	}
	if _, err = d.Token(); err != nil {
		return invalid()
	}
	if _, err = d.Token(); err != io.EOF {
		return invalid()
	}
	allowed := map[string]bool{"version": true, "kind": true, "stage": true, "name": true, "message": true, "stack": true, "truncated": true, "source": true, "line": true, "column": true}
	for key := range fields {
		if !allowed[key] {
			return invalid()
		}
	}
	version, ok := fields["version"].(json.Number)
	if !ok || version != "2" {
		return invalid()
	}
	str := func(key string) (string, bool) {
		switch x := fields[key].(type) {
		case string:
			return x, true
		case jsonvalue.UTF16String:
			return string(x), true
		}
		return "", false
	}
	f := &Failure{}
	for _, field := range []struct {
		key string
		dst *string
	}{{"kind", &f.Kind}, {"stage", &f.Stage}, {"name", &f.Name}, {"message", &f.Message}, {"stack", &f.Stack}} {
		s, ok := str(field.key)
		if !ok {
			return invalid()
		}
		*field.dst = s
	}
	if f.Kind != "exception" && f.Kind != "conversion" && f.Kind != "limit" && f.Kind != "internal" {
		return invalid()
	}
	if f.Stage != "compile" && f.Stage != "evaluate" && f.Stage != "output" {
		return invalid()
	}
	if f.Truncated, ok = fields["truncated"].(bool); !ok {
		return invalid()
	}
	_, hasSource := fields["source"]
	_, hasLine := fields["line"]
	_, hasColumn := fields["column"]
	if hasSource || hasLine || hasColumn {
		if !hasSource || !hasLine || !hasColumn {
			return invalid()
		}
		if f.Source, ok = str("source"); !ok {
			return invalid()
		}
		for _, field := range []struct {
			key string
			dst *int
		}{{"line", &f.Line}, {"column", &f.Column}} {
			n, ok := fields[field.key].(json.Number)
			if !ok {
				return invalid()
			}
			i, err := strconv.Atoi(string(n))
			if err != nil || i < 0 {
				return invalid()
			}
			*field.dst = i
		}
	}
	if err = b.Check(); err != nil {
		return nil, err
	}
	return f, nil
}
