package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	js "github.com/mellowforge/jsontrans-go/experiments/javascript"
	"github.com/tetratelabs/wazero"
)

func main() {
	backend := flag.String("backend", "quickjs", "quickjs or goja")
	probe := flag.String("probe", "cold-start", "isolated probe name")
	wasm := flag.String("audit-wasm", "", "inspect a pinned wasm artifact instead of executing a probe")
	flag.Parse()
	if *wasm != "" {
		data, err := os.ReadFile(*wasm)
		if err != nil {
			panic(err)
		}
		ctx := context.Background()
		rt := wazero.NewRuntime(ctx)
		defer rt.Close(ctx)
		mod, err := rt.CompileModule(ctx, data)
		if err != nil {
			panic(err)
		}
		defer mod.Close(ctx)
		imports := []string{}
		for _, f := range mod.ImportedFunctions() {
			module, name, _ := f.Import()
			imports = append(imports, module+"."+name)
		}
		memory := []any{}
		for name, m := range mod.ExportedMemories() {
			max, has := m.Max()
			memory = append(memory, map[string]any{"name": name, "min_pages": m.Min(), "max_pages": max, "has_max": has})
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "bytes": len(data), "imports": imports, "memory": memory})
		return
	}
	json.NewEncoder(os.Stdout).Encode(js.Probe(js.Backend(*backend), *probe))
}
