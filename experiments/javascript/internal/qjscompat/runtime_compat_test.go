package qjs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// Wazero v1.12 has no public cache-reference counter. Inspect the pinned
// implementation in this serial test: successful evaluation alone cannot
// detect a leaked compilation reference retained by the process-wide cache.
func guestCompilationReferences(t *testing.T) int64 {
	t.Helper()
	compiled := reflect.ValueOf(compiledQJSModule).Elem()
	engine := compiled.FieldByName("compiledEngine").Elem().Elem()
	key := compiled.FieldByName("module").Elem().FieldByName("ID")
	cache := engine.FieldByName("compiledModules")
	if !cache.IsValid() {
		// Wazero falls back to its interpreter where native code is unavailable.
		cache = engine.FieldByName("compiledFunctions")
	}
	entry := cache.MapIndex(key)
	if !entry.IsValid() {
		t.Fatal("global guest compilation reference was released")
	}
	return entry.Elem().FieldByName("refCount").Int()
}

func TestFreshRuntimeReleasesLocalCompilationReference(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "host_setup_failure"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if err := createGlobalCompiledModule(ctx, true, true, ""); err != nil {
				t.Fatal(err)
			}
			option := Option{Context: ctx}
			if fail {
				// Rejected by the host-function builder, after guest compilation.
				option.ProxyFunction = struct{}{}
			}
			rt, err := New(option)
			if fail {
				if err == nil || rt != nil {
					t.Fatalf("invalid host function: runtime=%v error=%v", rt, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				defer rt.wrt.Close(ctx)
				defer rt.Close()
				value, err := rt.Eval("lifetime.js", Code("6 * 7"))
				if err != nil {
					t.Fatal(err)
				}
				defer value.Free()
				if got := value.String(); got != "42" {
					t.Fatalf("live guest result = %q", got)
				}
			}
			// Only the deliberate global cache owner remains after New returns.
			if got := guestCompilationReferences(t); got != 1 {
				t.Fatalf("guest compilation references = %d, want 1", got)
			}
		})
	}
}

func TestCopiedSourceAndArtifactProvenance(t *testing.T) {
	var manifest struct {
		Files        map[string]string `json:"files"`
		PatchedFiles map[string]string `json:"patched_files"`
	}
	data, err := os.ReadFile("upstream.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for name, expected := range manifest.Files {
		if patched := manifest.PatchedFiles[name]; patched != "" {
			expected = patched
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != expected {
			t.Errorf("%s hash %s differs from reviewed provenance %s", name, got, expected)
		}
	}
}

// The assessment reuses the compilation cache but instantiates a fresh runtime
// on every compile/evaluation. Function type IDs must belong to that runtime.
func TestFreshRuntimesShareCodeWithoutSharingTypeIDs(t *testing.T) {
	for range 3 {
		rt, err := New()
		if err != nil {
			t.Fatal(err)
		}
		value, err := rt.Eval("compat.js", Code("[1, 2, 3].reduce((a, b) => a + b, 0)"))
		if err != nil {
			rt.Close()
			t.Fatal(err)
		}
		got := value.String()
		value.Free()
		rt.Close()
		if got != "6" {
			t.Fatalf("result = %q", got)
		}
	}
}
