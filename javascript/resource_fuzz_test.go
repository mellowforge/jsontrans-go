package javascript

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
)

// Vary real resource boundaries, not arbitrary executable source. Scalar input
// and empty bindings let tiny output limits reach the intended output phase.
func FuzzOwnedResourceLimits(f *testing.F) {
	const arraySource = `n=>Array.from({length:n},(_,i)=>i)`
	sources := []string{arraySource, `n=>{let x=0;for(let i=0;i<n;i++)x=[x];return x}`, `n=>'x'.repeat(n)`, `n=>new ArrayBuffer(n).byteLength`, `n=>{try{return new ArrayBuffer(n).byteLength}catch(e){return -1}}`}
	caps := []int{2, 8, 64}
	programs := make([][]jt.Program, len(caps))
	healthy := make([]jt.Program, len(caps))
	for index, mib := range caps {
		engine, err := New(context.Background(), Config{GuestMemoryBytes: mib << 20, Limits: jt.Limits{Timeout: semanticTestTimeout}})
		if err != nil {
			f.Fatal(err)
		}
		defer engine.Close()
		for mode, source := range sources {
			p, err := engine.Compile(context.Background(), source)
			if err != nil {
				f.Fatal(err)
			}
			programs[index] = append(programs[index], p)
			for _, seed := range [][2]uint16{{0, 0}, {1, 1}, {3, 8}, {16, 2}, {64, 63}, {1024, 1025}} {
				f.Add(uint8(index*len(sources)+mode), seed[0], seed[1])
			}
		}
		healthy[index], err = engine.Compile(context.Background(), `()=>42`)
		if err != nil {
			f.Fatal(err)
		}
	}
	f.Fuzz(func(t *testing.T, selector uint8, size, limit uint16) {
		engineIndex := (int(selector) / len(sources)) % len(caps)
		mode := int(selector) % len(sources)
		count := 0
		wantLimit, wantOOM := false, false
		options := jt.EvalOptions{}
		var expected any
		switch mode {
		case 0:
			count = int(size) % 65
			maxItems := 1 + int(limit)%64
			options.Limits = &jt.Limits{MaxCollectionItems: maxItems}
			wantLimit = count > maxItems
			values := make([]int, count)
			for i := range values {
				values[i] = i
			}
			expected = values
		case 1:
			count = int(size) % 17
			maxDepth := 1 + int(limit)%17
			options.Limits = &jt.Limits{MaxDepth: maxDepth}
			wantLimit = count >= maxDepth
			expected = 0
			for range count {
				expected = []any{expected}
			}
		case 2:
			count = int(size) % 1025
			maxBytes := 1 + int(limit)%1026
			options.Limits = &jt.Limits{MaxOutputBytes: maxBytes}
			wantLimit = count+2 > maxBytes
			expected = strings.Repeat("x", count)
		case 3:
			count = int(size) % 4097
			expected = count
		case 4:
			count = (caps[engineIndex] << 20) + 1 + int(size)
			wantOOM = true
		}
		got, err := programs[engineIndex][mode].Eval(context.Background(), count, options)
		var d *jt.Error
		var ex *Exception
		switch {
		case wantLimit:
			if got.Defined || got.Value != nil || !errors.Is(err, jt.ErrLimit) || !errors.Is(err, jt.ErrConversion) || !errors.As(err, &d) || d.Phase != jt.PhaseOutput || errors.As(err, &ex) {
				t.Fatalf("output boundary mode=%d size=%d limit=%d: %+v %v", mode, size, limit, got, err)
			}
		case wantOOM:
			if got.Defined || got.Value != nil || !errors.As(err, &ex) || !errors.As(err, &d) || d.Phase != jt.PhaseEvaluate || errors.Is(err, jt.ErrLimit) || errors.Is(err, jt.ErrConversion) || (ex.Name != "InternalError" && ex.Name != "") {
				t.Fatalf("guest cap %d MiB: %+v %v", caps[engineIndex], got, err)
			}
		default:
			wantJSON, _ := json.Marshal(expected)
			gotJSON, marshalErr := json.Marshal(got.Value)
			if err != nil || !got.Defined || marshalErr != nil || string(gotJSON) != string(wantJSON) {
				t.Fatalf("successful boundary mode=%d size=%d limit=%d: got %s, want %s, error %v", mode, size, limit, gotJSON, wantJSON, err)
			}
		}
		if got, err := healthy[engineIndex].Eval(context.Background(), nil, jt.EvalOptions{}); err != nil || !got.Defined || got.Value != json.Number("42") {
			t.Fatalf("recovery after resource boundary: %+v %v", got, err)
		}
	})
}
