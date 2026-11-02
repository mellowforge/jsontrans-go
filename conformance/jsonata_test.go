package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

type jsonataCase struct {
	Expr      string          `json:"expr"`
	ExprFile  string          `json:"expr-file"`
	Dataset   *string         `json:"dataset"`
	Data      json.RawMessage `json:"data"`
	Bindings  json.RawMessage `json:"bindings"`
	Result    any             `json:"result"`
	Undefined bool            `json:"undefinedResult"`
	Code      string          `json:"code"`
	Token     string          `json:"token"`
	Error     *struct {
		Code  string `json:"code"`
		Token string `json:"token"`
	} `json:"error"`
	Unordered bool `json:"unordered"`
	TimeLimit int  `json:"timelimit"`
	Depth     int  `json:"depth"`
}

// UnmarshalJSON preserves unpaired UTF-16 surrogate escapes in expressions:
// Go's JSON decoder otherwise replaces them before the JSONata lexer sees them.
func (c *jsonataCase) UnmarshalJSON(raw []byte) error {
	type plain jsonataCase
	var aux struct {
		*plain
		Expr json.RawMessage `json:"expr"`
	}
	aux.plain = (*plain)(c)
	if err := json.Unmarshal(raw, &aux); err != nil {
		return err
	}
	if len(aux.Expr) == 0 {
		return nil
	}
	expr := preserveSurrogateEscapes(aux.Expr)
	return json.Unmarshal(expr, &c.Expr)
}
func preserveSurrogateEscapes(raw []byte) []byte {
	var out bytes.Buffer
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 >= len(raw) {
			out.WriteByte(raw[i])
			continue
		}
		if raw[i+1] != 'u' || i+5 >= len(raw) {
			out.WriteByte(raw[i])
			i++
			out.WriteByte(raw[i])
			continue
		}
		value, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			out.WriteByte(raw[i])
			continue
		}
		paired := false
		if value >= 0xd800 && value <= 0xdbff && i+11 < len(raw) && raw[i+6] == '\\' && raw[i+7] == 'u' {
			next, e := strconv.ParseUint(string(raw[i+8:i+12]), 16, 16)
			paired = e == nil && next >= 0xdc00 && next <= 0xdfff
		}
		if (value >= 0xd800 && value <= 0xdfff) && !paired {
			out.WriteByte('\\')
		}
		out.Write(raw[i : i+6])
		i += 5
		if paired {
			out.Write(raw[i+1 : i+7])
			i += 6
		}
	}
	return out.Bytes()
}

func TestJSONataConformance(t *testing.T) {
	root := filepath.Join("testdata", "jsonata")
	files, err := filepath.Glob(filepath.Join(root, "groups", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no JSONata fixtures found")
	}
	// Conformance fixtures are trusted, and some intentionally perform large
	// computations. Production defaults remain unchanged.
	engine, err := jsonata.New(jsonata.WithLimits(jsontrans.Limits{MaxSteps: 100_000_000, Timeout: 10 * time.Second}))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		rel, _ := filepath.Rel(filepath.Join(root, "groups"), file)
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var cases []jsonataCase
			if len(raw) > 0 && strings.TrimSpace(string(raw))[0] == '[' {
				err = json.Unmarshal(raw, &cases)
			} else {
				var c jsonataCase
				err = json.Unmarshal(raw, &c)
				cases = []jsonataCase{c}
			}
			if err != nil {
				t.Fatal(err)
			}
			for i, c := range cases {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					expr := c.Expr
					if c.ExprFile != "" {
						b, err := os.ReadFile(filepath.Join(filepath.Dir(file), c.ExprFile))
						if err != nil {
							t.Fatal(err)
						}
						expr = string(b)
					}
					ctx := jsonvalue.WithObjectOrder(context.Background())
					decodeBudget, err := budget.New(ctx, jsontrans.DefaultLimits())
					if err != nil {
						t.Fatal(err)
					}
					defer decodeBudget.Close()
					var input any = jsonata.Undefined
					if len(c.Data) > 0 {
						input, err = jsonvalue.Decode(c.Data, decodeBudget)
						if err != nil {
							t.Fatal(err)
						}
					} else if c.Dataset != nil && *c.Dataset != "" {
						data, err := os.ReadFile(filepath.Join(root, "datasets", *c.Dataset+".json"))
						if err != nil {
							t.Fatal(err)
						}
						input, err = jsonvalue.Decode(data, decodeBudget)
						if err != nil {
							t.Fatal(err)
						}
					}
					var bindings map[string]any
					if len(c.Bindings) > 0 {
						value, err := jsonvalue.Decode(c.Bindings, decodeBudget)
						if err != nil {
							t.Fatal(err)
						}
						switch x := value.(type) {
						case map[string]any:
							bindings = x
						case jsontrans.UTF16Object:
							bindings = map[string]any(x)
						case nil:
						default:
							t.Fatalf("invalid bindings %T", value)
						}
					}
					caseEngine := engine
					if c.TimeLimit > 0 && c.Depth > 0 {
						caseEngine, err = jsonata.New(jsonata.WithLimits(jsontrans.Limits{MaxSteps: 100_000_000, Timeout: 10 * time.Second, MaxDepth: 10000}), jsonata.WithEvaluationDepth(c.Depth))
						if err != nil {
							t.Fatal(err)
						}
					}
					p, err := caseEngine.CompileRaw(context.Background(), expr)
					var got jsontrans.Result
					if err == nil {
						options := jsontrans.EvalOptions{Bindings: bindings}
						if c.TimeLimit > 0 {
							options.Limits = &jsontrans.Limits{Timeout: time.Duration(c.TimeLimit) * time.Millisecond, MaxSteps: 100000000, MaxCollectionItems: 10000000, MaxDepth: 10000}
						}
						if jsonata.IsUndefined(input) {
							var v any
							v, err = p.EvalRaw(ctx, input, options)
							if err == nil && !jsonata.IsUndefined(v) {
								got = jsontrans.Result{Value: v, Defined: true}
							}
						} else {
							got, err = p.Eval(ctx, input, options)
						}
					}
					expected := c.Code
					if c.Error != nil && c.Error.Code != "" {
						expected = c.Error.Code
					}
					if expected != "" {
						if err == nil {
							t.Fatalf("expected error %s, got %#v", expected, got)
						}
						var native *jsontrans.Error
						if !errors.As(err, &native) || native.Code != expected {
							t.Fatalf("expected error %s, got %v", expected, err)
						}
						expectedToken := c.Token
						if c.Error != nil && c.Error.Token != "" {
							expectedToken = c.Error.Token
						}
						if expectedToken != "" && native.Token != expectedToken {
							t.Fatalf("expected token %q, got %q", expectedToken, native.Token)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if c.Undefined {
						if got.Defined {
							t.Fatalf("expected undefined, got %#v", got.Value)
						}
						return
					}
					if !got.Defined {
						t.Fatal("unexpected undefined")
					}
					actual, err := json.Marshal(got.Value)
					if err != nil {
						t.Fatal(err)
					}
					var normalized any
					if err = json.Unmarshal(actual, &normalized); err != nil {
						t.Fatal(err)
					}
					if c.Unordered {
						a, aok := normalized.([]any)
						b, bok := c.Result.([]any)
						if aok && bok {
							sort.Slice(a, func(i, j int) bool { return fmt.Sprint(a[i]) < fmt.Sprint(a[j]) })
							sort.Slice(b, func(i, j int) bool { return fmt.Sprint(b[i]) < fmt.Sprint(b[j]) })
						}
					}
					if !reflect.DeepEqual(normalized, c.Result) {
						t.Fatalf("got %#v, want %#v", normalized, c.Result)
					}
				})
			}
		})
	}
}
