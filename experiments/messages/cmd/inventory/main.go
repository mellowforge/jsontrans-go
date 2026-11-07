package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	m "github.com/mellowforge/jsontrans-go/experiments/messages"
	"os"
)

func main() {
	rows := []map[string]any{}
	for _, e := range []string{"jsonata", "jsonpath", "javascript"} {
		for _, kind := range []string{"scalar", "object", "array", "fallback", "bindings", "limits", "host", "output", "large-array"} {
			if e == "jsonpath" && kind == "bindings" || e == "javascript" && kind != "object" {
				continue
			}
			sizes := []int{512, 1024, 8192, 65536}
			if kind == "large-array" {
				sizes = []int{1 << 20}
			}
			if e == "javascript" {
				sizes = []int{1024}
			}
			for _, size := range sizes {
				for _, n := range []int{1, 4, 16} {
					if kind == "large-array" && n > 1 || e == "javascript" && n > 4 {
						continue
					}
					f, err := m.NewFixture(e, kind, size, n)
					if err != nil {
						panic(err)
					}
					lengths := []int{}
					outputs := []int{}
					hash := sha256.New()
					for _, sample := range f.Samples {
						lengths = append(lengths, len(sample.Input))
						hash.Write(sample.Input)
						sum := 0
						for _, o := range sample.Want {
							sum += len(o.Value)
						}
						outputs = append(outputs, sum)
					}
					rows = append(rows, map[string]any{"engine": e, "kind": kind, "target_bytes": size, "fanout": n, "expressions": f.Config.Expressions, "input_bytes": lengths, "total_output_bytes": outputs, "corpus_sha256": hex.EncodeToString(hash.Sum(nil))})
				}
			}
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(rows); err != nil {
		panic(err)
	}
}
