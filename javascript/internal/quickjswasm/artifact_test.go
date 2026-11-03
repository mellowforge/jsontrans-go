package quickjswasm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
)

func TestArtifactPinnedAndCapabilityFree(t *testing.T) {
	data, err := os.ReadFile("quickjs.wasm")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("guest/lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		ArtifactSHA256 string `json:"artifact_sha256"`
	}
	if err = json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != lock.ArtifactSHA256 {
		t.Fatalf("artifact hash %s != %s", got, lock.ArtifactSHA256)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	module, err := r.CompileModule(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	defer module.Close(ctx)
	if len(module.ImportedFunctions()) != 0 || len(module.ImportedMemories()) != 0 {
		for _, f := range module.ImportedFunctions() {
			m, n, _ := f.Import()
			t.Errorf("unexpected import %s.%s", m, n)
		}
		t.Fatal("guest must have no host imports")
	}
	for _, name := range []string{"abi_version", "abi_init", "abi_alloc", "abi_free", "abi_compile", "abi_eval", "abi_output_ptr", "abi_output_len", "abi_status"} {
		if module.ExportedFunctions()[name] == nil {
			t.Errorf("missing ABI export %s", name)
		}
	}
}
