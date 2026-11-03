package qjs

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

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
