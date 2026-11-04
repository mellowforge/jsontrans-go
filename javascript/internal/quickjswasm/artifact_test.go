package quickjswasm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
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
	// An import-section count covers functions, memories, tables, globals and
	// tags alike; the public metadata API exposes only some of those kinds.
	sections := bytes.NewReader(data[8:])
	for sections.Len() > 0 {
		id, err := sections.ReadByte()
		if err != nil {
			t.Fatal(err)
		}
		size, err := binary.ReadUvarint(sections)
		if err != nil || size > uint64(sections.Len()) {
			t.Fatalf("invalid Wasm section size: %d %v", size, err)
		}
		if id == 2 {
			count, err := binary.ReadUvarint(sections)
			if err != nil || count != 0 {
				t.Fatalf("guest has imports: count=%d error=%v", count, err)
			}
			break
		}
		if _, err := sections.Seek(int64(size), 1); err != nil {
			t.Fatal(err)
		}
	}
	if len(module.ImportedFunctions()) != 0 || len(module.ImportedMemories()) != 0 {
		for _, f := range module.ImportedFunctions() {
			m, n, _ := f.Import()
			t.Errorf("unexpected import %s.%s", m, n)
		}
		t.Fatal("guest must have no host imports")
	}
	i32 := api.ValueTypeI32
	for name, signature := range map[string]struct{ parameters, results []api.ValueType }{
		"_initialize":    {},
		"abi_version":    {nil, []api.ValueType{i32}},
		"abi_init":       {[]api.ValueType{i32, i32, i32, i32}, []api.ValueType{i32}},
		"abi_alloc":      {[]api.ValueType{i32}, []api.ValueType{i32}},
		"abi_free":       {[]api.ValueType{i32}, nil},
		"abi_compile":    {[]api.ValueType{i32, i32, i32}, []api.ValueType{i32}},
		"abi_eval":       {[]api.ValueType{i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32}, []api.ValueType{i32}},
		"abi_output_ptr": {nil, []api.ValueType{i32}},
		"abi_output_len": {nil, []api.ValueType{i32}},
		"abi_status":     {nil, []api.ValueType{i32}},
	} {
		f := module.ExportedFunctions()[name]
		if f == nil || !slices.Equal(f.ParamTypes(), signature.parameters) || !slices.Equal(f.ResultTypes(), signature.results) {
			t.Errorf("incorrect ABI signature for %s: %v", name, f)
		}
	}
}

func TestOwnedInvalidPrivateStageFailsInternally(t *testing.T) {
	e, err := New(context.Background(), Config{Timeout: semanticTestTimeout})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	rule, err := e.Compile(context.Background(), `()=>42`)
	if err != nil {
		t.Fatal(err)
	}
	// Only trusted internal callers can supply bridge bytecode. A malformed
	// bridge still must not convert its invented phase into a native diagnostic.
	codec, err := e.CompileNamed(context.Background(), `(r,i,b,d,n,x,w,o,stage)=>{stage[0]=2;throw new Error('bridge')}`, "bridge.js")
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Eval(context.Background(), rule, codec, `null`, `{}`, Limits{Depth: 256, Items: 100000, Bytes: 1024, Work: 1000000})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Kind != "internal" || failure.Message != "invalid trusted evaluation phase" {
		t.Fatalf("invalid private stage: %v", err)
	}
}
