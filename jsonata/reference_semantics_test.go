package jsonata

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go"
)

// referenceCase holds an expected result recorded from JSONata 2.2.2
// (conformance/oracle). want is "undefined", JSON text, or "ERR <code>";
// "ERR uncoded" is a native error the reference raises without a code.
type referenceCase struct {
	expr, input, want string
}

// checkReferenceCases evaluates each case through ordinary JSON evaluation,
// prepared evaluation and a single-program batch, so specialized plans and the
// interpreter are held to the same expectation.
func checkReferenceCases(t *testing.T, cases []referenceCase) {
	t.Helper()
	ctx := context.Background()
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.expr+" "+tc.input, func(t *testing.T) {
			program, err := engine.Compile(ctx, tc.expr)
			if err != nil {
				if got := describeError(err); got != tc.want {
					t.Errorf("compile: got %s (%v), want %s", got, err, tc.want)
				}
				return
			}
			ordinary, err := jsontrans.EvalJSON(ctx, program, []byte(tc.input), jsontrans.EvalOptions{})
			if got := describeJSONResult(ordinary, err); got != tc.want {
				t.Errorf("ordinary: got %s, want %s", got, tc.want)
			}
			document, err := jsontrans.PrepareJSON(ctx, []byte(tc.input), jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := jsontrans.EvalPrepared(ctx, program, document, jsontrans.EvalOptions{})
			if got := describeResult(prepared, err); got != tc.want {
				t.Errorf("prepared: got %s, want %s", got, tc.want)
			}
			batch, err := jsontrans.NewBatch([]jsontrans.Program{program})
			if err != nil {
				t.Fatal(err)
			}
			results, err := batch.EvalPrepared(ctx, document, nil, jsontrans.EvalOptions{})
			var batched jsontrans.Result
			if err == nil {
				batched = results[0]
			}
			if got := describeResult(batched, err); got != tc.want {
				t.Errorf("batch: got %s, want %s", got, tc.want)
			}
		})
	}
}

// withParentTracking places each expression beside a parent (%) path. Parent
// tracking elsewhere in a program must not change an expression's result.
func withParentTracking(cases []referenceCase) []referenceCase {
	var out []referenceCase
	for _, tc := range cases {
		if strings.HasPrefix(tc.want, "ERR") || strings.Contains(tc.expr, "%") {
			continue
		}
		want := `{"t":0}`
		if tc.want != "undefined" {
			want = `{"r":` + tc.want + `,"t":0}`
		}
		out = append(out, referenceCase{`{"r": ` + tc.expr + `, "t": $count(zz.%)}`, tc.input, want})
	}
	return out
}

func describeError(err error) string {
	var native *jsontrans.Error
	if !errors.As(err, &native) {
		return "ERR plain: " + err.Error()
	}
	if native.Code == "" {
		return "ERR uncoded"
	}
	return "ERR " + native.Code
}

func describeJSONResult(r jsontrans.JSONResult, err error) string {
	if err != nil {
		return describeError(err)
	}
	if !r.Defined {
		return "undefined"
	}
	return string(r.Value)
}

func describeResult(r jsontrans.Result, err error) string {
	if err != nil {
		return describeError(err)
	}
	if !r.Defined {
		return "undefined"
	}
	encoded, err := json.Marshal(r.Value)
	if err != nil {
		return "ERR marshal: " + err.Error()
	}
	return string(encoded)
}

// JSONata keeps a lone array produced by the final path step, including an
// empty one; earlier empty steps still end the path.
func TestReferenceEmptyArrayFinalPathStep(t *testing.T) {
	checkReferenceCases(t, emptyArrayFinalStepCases)
	checkReferenceCases(t, withParentTracking(emptyArrayFinalStepCases))
}

var emptyArrayFinalStepCases = []referenceCase{
	{`a.b`, `{"a":{"b":[]}}`, `[]`},
	{`a.b`, `{"a":[{"b":[]}]}`, `[]`},
	{`a.b`, `{"a":[{"b":[]},{"c":1}]}`, `[]`},
	{`a.b`, `{"a":[{"b":[]},{"b":[]}]}`, `undefined`},
	{`a.b`, `{"a":[{"b":[]},{"b":1}]}`, `1`},
	{`a.b.c`, `{"a":{"b":{"c":[]}}}`, `[]`},
	{`a.b.c`, `{"a":{"b":[]}}`, `undefined`},
	{`$.a`, `{"a":[]}`, `[]`},
	{`$$.a`, `{"a":[]}`, `[]`},
	{`a`, `{"a":[]}`, `[]`},
	{`a[0].b`, `{"a":[{"b":[]}]}`, `[]`},
	{`$sum(a.b)`, `{"a":{"b":[]}}`, `0`},
	{`$count(a.b)`, `{"a":{"b":[]}}`, `0`},
	{`$exists(a.b)`, `{"a":{"b":[]}}`, `true`},
	{`$type(a.b)`, `{"a":{"b":[]}}`, `"array"`},
	{`$length(a.b)`, `{"a":{"b":[]}}`, `ERR T0410`},
	{`Account.Order.Product.SKU`, `{"Account":{"Order":{"Product":{"SKU":[]}}}}`, `[]`},
	{`a.b[0]`, `{"a":{"b":[]}}`, `undefined`},
	{`a.(b)`, `{"a":{"b":[]}}`, `[]`},
	{`a.b@$x.$x`, `{"a":{"b":[]}}`, `undefined`},
	{`a#$i.b`, `{"a":{"b":[]}}`, `undefined`},
	{`a.b.$`, `{"a":{"b":[]}}`, `undefined`},
	{`a.b ~> $count()`, `{"a":{"b":[]}}`, `0`},
	{`a@$x`, `{"a":[]}`, `undefined`},
	{`$.a@$x`, `{"a":[]}`, `undefined`},
	{`a.b@$x`, `{"a":{"b":[]}}`, `undefined`},
	{`a.b#$i`, `{"a":{"b":[]}}`, `undefined`},
	{`a#$i`, `{"a":[]}`, `undefined`},
	{`a@$x.b`, `{"a":{"b":[]}}`, `undefined`},
	{`a#$i.b`, `{"a":[{"b":[]}]}`, `undefined`},
	{`a.b^($)`, `{"a":{"b":[]}}`, `undefined`},
	{`a.b[]`, `{"a":{"b":[]}}`, `[]`},
	{`a.b[]`, `{"a":[{"b":[]},{"b":[]}]}`, `undefined`},
	{`a.b[]`, `{"a":[{"b":[]},{"c":1}]}`, `[]`},
	{`a.b[]`, `{"a":{"b":[1]}}`, `[1]`},
	{`a.b[]`, `{"a":{"b":1}}`, `[1]`},
	{`a.b[]`, `{"a":{"b":[[]]}}`, `[[]]`},
	{`a.c[]`, `{"a":{}}`, `undefined`},
	{`a.b[].c`, `{"a":{"b":[]}}`, `undefined`},
	{`a[].b`, `{"a":{"b":[]}}`, `[]`},
	{`$count(a.b[])`, `{"a":{"b":[]}}`, `0`},
	{`a.%`, `{"a":[]}`, `undefined`},
	// Predicate shapes reach the prepared fast plan.
	{`$sum(a.b) = 0`, `{"a":{"b":[]}}`, `true`},
	{`$sum(a.b) > -1`, `{"a":{"b":[]}}`, `true`},
	{`$count(a.b) = 0`, `{"a":{"b":[]}}`, `true`},
	{`$exists(a.b) and true`, `{"a":{"b":[]}}`, `true`},
	{`$sum(a[0].b) = 0`, `{"a":[{"b":[]}]}`, `true`},
}

// JSONata orders strings by UTF-16 code units, as JavaScript does, which
// differs from UTF-8 byte order for U+E000..U+FFFF against supplementary
// characters and for isolated surrogates.
func TestReferenceStringOrderingUsesUTF16Units(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`a < b`, `{"a":"😀","b":"～"}`, `true`},
		{`a > b`, `{"a":"😀","b":"～"}`, `false`},
		{`a <= b`, `{"a":"😀","b":"～"}`, `true`},
		{`a >= b`, `{"a":"😀","b":"～"}`, `false`},
		{`a < b`, `{"a":"x😀","b":"x\uffff"}`, `true`},
		{`a < b`, `{"a":"\ud800","b":"😀"}`, `true`},
		{`a < b`, `{"a":"\udbff","b":"😀"}`, `false`},
		{`a < b`, `{"a":"\ue000","b":"\ud800"}`, `false`},
		{`a < b`, `{"a":"abc","b":"abd"}`, `true`},
		{`a < b`, `{"a":"ab","b":"abc"}`, `true`},
		{`$sort(a)`, `{"a":["～","😀","a"]}`, `["a","😀","～"]`},
		{`a^($)`, `{"a":["～","😀","a"]}`, `["a","😀","～"]`},
		{`a^(>$)`, `{"a":["～","😀","a"]}`, `["～","😀","a"]`},
		{`items[name < 'ｚ'].name`, `{"items":[{"name":"😀"},{"name":"～"}]}`, `"😀"`},
		{`a < 'ｚ'`, `{"a":"😀"}`, `true`},
	})
}

// Regex literals end at the first unescaped slash outside (), [] and {}
// nesting, and invalid patterns are rejected while compiling.
func TestReferenceRegexLiteralScanning(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$match("a/b", /(a/b)/).match`, `{}`, `"a/b"`},
		{`$match("a/b", /a{1}\/b/).match`, `{}`, `"a/b"`},
		{`$contains("a/b", /[/]/)`, `{}`, `true`},
		{`$contains("x", /\//)`, `{}`, `false`},
		{`$contains("a\\", /a\\/)`, `{}`, `true`},
		{`$replace("aXb", /x/i, "-")`, `{}`, `"a-b"`},
		{`//`, `{}`, `ERR S0301`},
		{`$match("a", //)`, `{}`, `ERR S0301`},
		{`/abc`, `{}`, `ERR S0302`},
		{`$match("a", /(a/)`, `{}`, `ERR S0302`},
		{`true ? "x" : $match("abc", /a{2,1}/)`, `{}`, `ERR uncoded`},
		{`true ? "x" : $match("abc", /(/)`, `{}`, `ERR S0302`},
		{`true ? "x" : $match("abc", /a)/)`, `{}`, `ERR S0302`},
	})
}

func TestReferenceRegexLiteralErrorPositions(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		expr     string
		code     string
		position int
	}{
		{`//`, "S0301", 1},
		{`$match("a", //)`, "S0301", 13},
		{`/abc`, "S0302", 4},
		{`$match("a", /(a/)`, "S0302", 17},
	} {
		_, err := engine.Compile(context.Background(), tc.expr)
		var native *jsontrans.Error
		if !errors.As(err, &native) || native.Code != tc.code || native.Position != tc.position {
			t.Errorf("%s: got %v, want %s at %d", tc.expr, err, tc.code, tc.position)
		}
	}
}

// A keep-array result ([] suffix) reaches functions as an ordinary array.
func TestReferenceKeepArrayFunctionArguments(t *testing.T) {
	checkReferenceCases(t, keepArrayArgumentCases)
	checkReferenceCases(t, withParentTracking(keepArrayArgumentCases))
}

var keepArrayArgumentCases = []referenceCase{
	{`$count(a[])`, `{"a":[1,2]}`, `2`},
	{`$count(a.b[])`, `{"a":{"b":[1,2]}}`, `2`},
	{`$count(a.b[])`, `{"a":{"b":[]}}`, `0`},
	{`$count(a.b[])`, `{"a":{"b":1}}`, `1`},
	{`$sum(a.b[])`, `{"a":{"b":[1,2]}}`, `3`},
	{`$exists(a.b[])`, `{"a":{"b":[]}}`, `true`},
	{`$type(a.b[])`, `{"a":{"b":[]}}`, `"array"`},
	{`a.b[] ~> $count()`, `{"a":{"b":[1,2]}}`, `2`},
}

// Array constructors flatten data arrays and keep-array results from their
// items, with or without parent tracking elsewhere in the program, and
// bindings made inside an item do not escape the constructor.
func TestReferenceArrayConstructorItems(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`[a.b, c.d.%.e]`, `{"a":{"b":[]},"c":{"d":{"x":1},"e":2}}`, `[2]`},
		{`[a.b, 1, c.%.e]`, `{"a":{"b":[]},"c":{"e":2}}`, `[1]`},
		{`[a.b, 1, c.%.e]`, `{"a":{"b":[1,2]},"c":{"e":2}}`, `[1,2,1]`},
		{`[a.b, 1, c.%.e]`, `{"a":{"b":[[]]},"c":{"e":2}}`, `[[],1]`},
		{`[a, 1, c.%.e]`, `{"a":[],"c":{"e":2}}`, `[1]`},
		{`[a.b, 1, c.%.e]`, `{"a":[{"b":[1]},{"b":[2,3]}],"c":{"e":2}}`, `[1,2,3,1]`},
		{`[a.b.%.x, 1]`, `{"a":{"b":[1,2],"x":[5,6]}}`, `[5,6,5,6,1]`},
		{`[a.b[], 1]`, `{"a":{"b":[]}}`, `[1]`},
		{`[a.b[], 1]`, `{"a":{"b":[1,2]}}`, `[1,2,1]`},
		{`[a.b[], 1]`, `{"a":{"b":3}}`, `[3,1]`},
		{`[a.b[], 1, c.%.e]`, `{"a":{"b":[1,2]},"c":{"e":2}}`, `[1,2,1]`},
		{`[a.b[], 1, c.%.e]`, `{"a":{"b":3},"c":{"e":2}}`, `[3,1]`},
		{`[a.b@$o, 1].$o`, `{"a":{"b":[1,2]}}`, `undefined`},
		{`a.[b@$o].$o`, `{"a":{"b":[1,2]}}`, `undefined`},
		{`a.[b, %.x]`, `{"a":{"b":1},"x":9}`, `[1,9]`},
		{`a.b.[%.x, 1]`, `{"a":{"b":{"y":1},"x":7}}`, `[7,1]`},
	})
}

// A parent step after a step that cannot supply a parent fails to compile.
func TestReferenceParentRequiresDerivableStep(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`[a.b].%`, `{"a":{"b":1}}`, `ERR S0217`},
		{`a.[b].%`, `{"a":{"b":1}}`, `ERR S0217`},
		{`a.b^($).%`, `{"a":{"b":[2,1]}}`, `ERR S0217`},
		{`a.$.%`, `{"a":{"b":1}}`, `ERR S0217`},
		{`a.$x.%`, `{"a":{"b":1}}`, `ERR S0217`},
		{`a.$string(b).%`, `{"a":{"b":1}}`, `ERR S0217`},
		{`a.{"k":b}.%`, `{"a":{"b":1}}`, `ERR S0217`},
		{`a.**.%`, `{"a":{"b":1}}`, `ERR S0217`},
		{`a.b.%`, `{"a":{"b":1}}`, `{"b":1}`},
		{`a.*.%`, `{"a":{"b":1}}`, `{"b":1}`},
		{`a.(b).%`, `{"a":{"b":1}}`, `{"b":1}`},
		{`a.b[0].%`, `{"a":{"b":[1]}}`, `{"b":[1]}`},
		{`a.b[].%`, `{"a":{"b":[1]}}`, `[{"b":[1]}]`},
		{`a.b@$x.%`, `{"a":{"b":1}}`, `{"b":1}`},
		{`a.b#$i.%`, `{"a":{"b":1}}`, `{"b":1}`},
		{`a."b".%`, `{"a":{"b":1}}`, `{"b":1}`},
		{`(a.b).%`, `{"a":{"b":1}}`, `{"b":1}`},
		{`a.(b; c).%`, `{"a":{"b":1,"c":2}}`, `{"b":1,"c":2}`},
		{`a.b.%.%`, `{"a":{"b":1}}`, `{"a":{"b":1}}`},
		{`a.(b.%)`, `{"a":{"b":1}}`, `{"b":1}`},
		{`a.b[%.b=1]`, `{"a":{"b":1}}`, `1`},
	})
}

func TestReferenceParentErrorPositions(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for expr, position := range map[string]int{
		`[a.b].%`: 1, `a.[b].%`: 3, `a.b^($).%`: 4, `a.$.%`: 3, `a.$x.%`: 4,
		`a.$string(b).%`: 10, `a.{"k":b}.%`: 3, `a.**.%`: 4,
	} {
		_, err := engine.Compile(context.Background(), expr)
		var native *jsontrans.Error
		if !errors.As(err, &native) || native.Code != "S0217" || native.Phase != "compile" || native.Position != position {
			t.Errorf("%s: got %v, want compile S0217 at %d", expr, err, position)
		}
	}
}

// Steps over @/# tuple streams flatten each step result, including arrays
// built by a constructor; without bindings constructed arrays stay nested.
func TestReferenceTupleStreamStepFlattening(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`a.b@$o.[$o, 1]`, `{"a":{"b":[1,2]}}`, `[1,1,2,1]`},
		{`a.b@$o.[$o]`, `{"a":{"b":[1,2]}}`, `[1,2]`},
		{`a.b#$i.[$i, 1]`, `{"a":{"b":[1,2]}}`, `[0,1,1,1]`},
		{`a.b.[$, 1]`, `{"a":{"b":[1,2]}}`, `[[1,1],[2,1]]`},
		{`a.b@$o.[$o, 1]`, `{"a":{"b":[1]}}`, `[1,1]`},
		{`a.b@$o.[$o, 1].$`, `{"a":{"b":[1,2]}}`, `[1,1,2,1]`},
		{`a.b@$o.[$o, 1][]`, `{"a":{"b":[1,2]}}`, `[1,1,2,1]`},
		{`a.b@$o.{"v": $o}`, `{"a":{"b":[1,2]}}`, `[{"v":1},{"v":2}]`},
		{`a.b@$o.($o)`, `{"a":{"b":[1,2]}}`, `[1,2]`},
		{`a.b@$o.c`, `{"a":{"b":[{"c":[1,2]},{"c":[3]}]}}`, `undefined`},
		{`a.b@$o.[c]`, `{"a":{"b":[{"c":[1,2]},{"c":[3]}]}}`, `undefined`},
		{`a.b#$i.[c]`, `{"a":{"b":[{"c":[1,2]},{"c":[3]}]}}`, `[1,2,3]`},
		{`a.b@$o.[$o.c, 1]`, `{"a":{"b":[{"c":5}]}}`, `[5,1]`},
	})
}

// Parent references inside group-by keys and values are never bound to the
// grouped step, so they are undefined; paths inside a pair still supply
// their own parents.
func TestReferenceParentInsideGroupBy(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`a.b{"k": %}`, `{"a":{"b":1}}`, `{}`},
		{`a.b{"k": %.x}`, `{"a":{"b":1,"x":5}}`, `{}`},
		{`a.b{"k": $}`, `{"a":{"b":1}}`, `{"k":1}`},
		{`a.b{$string($): %.x}`, `{"a":{"b":1,"x":5}}`, `{}`},
		{`a.b{%.x: $}`, `{"a":{"b":1,"x":5}}`, `{}`},
		{`a.b.c{"k": %.x}`, `{"a":{"b":{"c":1,"x":7}}}`, `{}`},
		{`a.b{"k": $sum(%.x)}`, `{"a":{"b":[1,2],"x":5}}`, `{}`},
		{`a.b{"k": c.%.x}`, `{"a":{"b":{"c":1,"x":9}}}`, `{"k":9}`},
		{`a.b.{"k": %.x}`, `{"a":{"b":1,"x":5}}`, `{"k":5}`},
		{`a.b.%{"k": x}`, `{"a":{"b":1,"x":5}}`, `{"k":5}`},
		{`a.b{"k": %%}`, `{"a":{"b":1}}`, `ERR S0211`},
	})
}

// Range endpoints must be finite integers; Infinity is not an integer.
func TestReferenceRangeRejectsNonFiniteEndpoints(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`[1/0..1/0]`, `{}`, `ERR T2003`},
		{`[1..1/0]`, `{}`, `ERR T2004`},
		{`[-1/0..1]`, `{}`, `ERR T2003`},
		{`[0/0..1]`, `{}`, `ERR T2003`},
		{`[1.5..3]`, `{}`, `ERR T2003`},
		{`[1..2.5]`, `{}`, `ERR T2004`},
	})
}

// Numeric arguments that are NaN, infinite, huge or fractional follow the
// reference's JavaScript conversions. NaN results are checked as x = x, which
// is false only for NaN.
func TestReferenceNonFiniteAndFractionalArguments(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`[1,2,3][0/0]`, `{}`, `undefined`},
		{`[1,2,3][1/0]`, `{}`, `ERR D1001`},
		{`[1,2,3][-1/0]`, `{}`, `ERR D1001`},
		{`[1,2,3][1e300]`, `{}`, `undefined`},
		{`$substring("foo", 1/0)`, `{}`, `""`},
		{`$substring("foo", -1/0)`, `{}`, `"foo"`},
		{`$substring("foo", 0/0)`, `{}`, `"foo"`},
		{`$substring("abc", 0/0, 2)`, `{}`, `""`},
		{`$substring("abc", 1, 0/0)`, `{}`, `""`},
		{`$substring("abc", 1, 1/0)`, `{}`, `"bc"`},
		{`$substring("abc", -1/0, 2)`, `{}`, `"ab"`},
		{`$substring("abc", 1.7, 1.2)`, `{}`, `"b"`},
		{`$substring("abcdef", -2.5, 2)`, `{}`, `"e"`},
		{`$split("a,b,c", ",", 0/0)`, `{}`, `[]`},
		{`$split("a,b,c", ",", 1/0)`, `{}`, `[]`},
		{`$split("a,b,c", ",", 1e300)`, `{}`, `[]`},
		{`$split("a,b,c", ",", 4294967296)`, `{}`, `[]`},
		{`$split("a,b,c", ",", 4294967297)`, `{}`, `["a"]`},
		{`$split("a,b,c", ",", 2.7)`, `{}`, `["a","b"]`},
		{`$split("a,b,c", /,/, 0/0)`, `{}`, `[]`},
		{`$split("a,b,c", /,/, 1/0)`, `{}`, `["a","b","c"]`},
		{`$split("a,b,c", /,/, 1.5)`, `{}`, `["a","b"]`},
		{`$split("a,b,c", /,/, 0.5)`, `{}`, `["a"]`},
		{`$count($match("aaa", /a/, 0/0))`, `{}`, `0`},
		{`$count($match("aaa", /a/, 1/0))`, `{}`, `3`},
		{`$count($match("aaa", /a/, 1.5))`, `{}`, `2`},
		{`$count($match("aaa", /a/, 0.5))`, `{}`, `1`},
		{`$replace("aaa", "a", "b", 0/0)`, `{}`, `"aaa"`},
		{`$replace("aaa", "a", "b", 1/0)`, `{}`, `"bbb"`},
		{`$replace("aaa", "a", "b", 1.5)`, `{}`, `"bba"`},
		{`$replace("aaa", /a/, "b", 0/0)`, `{}`, `"aaa"`},
		{`$replace("aaa", /a/, "b", 1.5)`, `{}`, `"bba"`},
		{`$replace("aaa", /a/, "b", 0.5)`, `{}`, `"baa"`},
		{`$round(1.5, 1/0) = $round(1.5, 1/0)`, `{}`, `false`},
		{`$round(1.5, 1e300) = $round(1.5, 1e300)`, `{}`, `false`},
		{`$round(1.5, 2.7) = $round(1.5, 2.7)`, `{}`, `false`},
		{`$round(1.5, 400) = $round(1.5, 400)`, `{}`, `false`},
		{`$round(1.5, 0/0)`, `{}`, `2`},
		{`$round(1.25, 1)`, `{}`, `1.2`},
		{`$round(123.456, -1)`, `{}`, `120`},
		{`$round(1.5, -400)`, `{}`, `0`},
		{`$formatBase(10, 2.9)`, `{}`, `"101"`},
		{`$formatBase(10, 36.5)`, `{}`, `"a"`},
		{`$formatBase(10, 2.5)`, `{}`, `"1010"`},
		{`$formatBase(2.5, 10)`, `{}`, `"2"`},
		{`$formatBase(10, 0/0)`, `{}`, `ERR uncoded`},
		{`$formatBase(10, 1/0)`, `{}`, `ERR D3100`},
	})
}

// Dates beyond JavaScript's range of ±8.64e15 ms are invalid: their numeric
// components format as NaN. Fixed-width years keep their last digits, and
// timezones are read as parseInt(tz) with floor(v/100) hours and v%100 minutes.
func TestReferenceDateRangeAndTimezoneArithmetic(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$fromMillis(8.64e15)`, `{}`, `"5760-09-13T00:00:00.000Z"`},
		{`$fromMillis(-8.64e15)`, `{}`, `"-1821-04-20T00:00:00.000Z"`},
		{`$fromMillis(8.64e15+1)`, `{}`, `"0NaN-NaN-NaNTNaN:NaN:NaN.NaNZ"`},
		{`$fromMillis(0/0)`, `{}`, `"0NaN-NaN-NaNTNaN:NaN:NaN.NaNZ"`},
		{`$fromMillis(1/0)`, `{}`, `"0NaN-NaN-NaNTNaN:NaN:NaN.NaNZ"`},
		{`$fromMillis(253402300800000)`, `{}`, `"0000-01-01T00:00:00.000Z"`},
		{`$fromMillis(0/0, "[Y0001]-[M01]-[D01] [H01]:[m01]:[s01].[f001] [P] [Z] [z] [E] [C] [d] [W] [X] [F1]", "+0100")`, `{}`, `"0NaN-NaN-NaN NaN:NaN:NaN.NaN am +01:00 GMT+01:00 ISO ISO NaN NaN NaN NaN"`},
		{`$fromMillis(0/0, "[Y]")`, `{}`, `"NaN"`},
		{`$fromMillis(0/0, "[M]")`, `{}`, `"NaN"`},
		{`$fromMillis(0/0, undefined, "-0530")`, `{}`, `"0NaN-NaN-NaNTNaN:NaN:NaN.NaN-06:30"`},
		{`$fromMillis(8.64e15, "[Y0001]")`, `{}`, `"5760"`},
		{`$fromMillis(8.64e15, "[Y]")`, `{}`, `"275760"`},
		{`$fromMillis(1e15, "[Y01]")`, `{}`, `"58"`},
		{`$fromMillis(1e15, "[Y,2]")`, `{}`, `"58"`},
		{`$fromMillis(-8.64e15, "[Y01]")`, `{}`, `"-21"`},
		{`$toMillis("99999999999999999999-01-01", "[Y]-[M]-[D]") = $toMillis("99999999999999999999-01-01", "[Y]-[M]-[D]")`, `{}`, `false`},
		{`$toMillis("275760-09-13", "[Y]-[M]-[D]")`, `{}`, `8640000000000000`},
		{`$toMillis("275760-09-14", "[Y]-[M]-[D]") = 0`, `{}`, `false`},
		{`$fromMillis(0, "[H01]:[m01] [Z]", "+0530")`, `{}`, `"05:30 +05:30"`},
		{`$fromMillis(0, "[H01]:[m01] [Z]", "-0530")`, `{}`, `"17:30 -06:30"`},
		{`$fromMillis(0, "[H01]:[m01] [Z]", "-0030")`, `{}`, `"22:30 -01:30"`},
		{`$fromMillis(0, "[H01]:[m01] [Z]", "+1400")`, `{}`, `"14:00 +14:00"`},
		{`$fromMillis(0, "[H01]:[m01] [Z]", "-1200")`, `{}`, `"12:00 -12:00"`},
		{`$fromMillis(0, "[H01]:[m01] [Z]", "+05")`, `{}`, `"00:05 +00:05"`},
		{`$fromMillis(0, "[H01]:[m01] [Z]", "-05")`, `{}`, `"22:55 -01:05"`},
		{`$fromMillis(0, "[H01]:[m01] [Z]", "+05:30")`, `{}`, `"00:05 +00:05"`},
		{`$fromMillis(0, undefined, "-0530")`, `{}`, `"1969-12-31T17:30:00.000-06:30"`},
	})
}

// NaN is falsy, as JavaScript's Boolean(NaN).
func TestReferenceNaNIsFalsy(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$boolean(0/0)`, `{}`, `false`},
		{`(0/0) ? 1 : 2`, `{}`, `2`},
		{`$not(0/0)`, `{}`, `true`},
		{`(0/0) and true`, `{}`, `false`},
		{`$boolean([0/0])`, `{}`, `false`},
	})
}

// The boolean cast rejects infinite numbers with D1001 in every boolean
// context, including any element of an array, and $round never returns -0.
func TestReferenceInfiniteTruthinessAndNegativeZero(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`(1/0) ? 1 : 2`, `{}`, `ERR D1001`},
		{`(1/0) and true`, `{}`, `ERR D1001`},
		{`true and (1/0)`, `{}`, `ERR D1001`},
		{`(1/0) or false`, `{}`, `ERR D1001`},
		{`$not(1/0)`, `{}`, `ERR D1001`},
		{`$boolean(1/0)`, `{}`, `ERR D1001`},
		{`$boolean([1/0])`, `{}`, `ERR D1001`},
		{`$boolean([0, 1/0])`, `{}`, `ERR D1001`},
		{`$boolean([1, 1/0])`, `{}`, `ERR D1001`},
		{`$exists(1/0)`, `{}`, `true`},
		{`[1,2,3][$ > 1/0]`, `{}`, `undefined`},
		{`$string($round(-0.4))`, `{}`, `"0"`},
		{`$string($round(-1, -1))`, `{}`, `"0"`},
	})
}

// Negative zero survives arithmetic but, as in JavaScript, prints as 0 in
// results and strings.
func TestReferenceNegativeZeroAtBoundaries(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`0 * -1`, `{}`, `0`},
		{`-0`, `{}`, `0`},
		{`$floor(-0)`, `{}`, `0`},
		{`[0 * -1]`, `{}`, `[0]`},
		{`{"a": 0 * -1}`, `{}`, `{"a":0}`},
		{`$string(0 * -1)`, `{}`, `"0"`},
		{`$string([0 * -1])`, `{}`, `"[0]"`},
		{`$string({"a": 0 * -1})`, `{}`, `"{\"a\":0}"`},
		{`0 * -1 & ""`, `{}`, `"0"`},
		{`1/(0 * -1) < 0`, `{}`, `true`},
		{`a`, `{"a":-0}`, `0`},
		{`a * 1`, `{"a":-0}`, `0`},
		{`$string(a)`, `{"a":-0}`, `"0"`},
		{`b.a`, `{"b":[{"a":-0},{"a":1}]}`, `[0,1]`},
		{`a = 0`, `{"a":-0}`, `true`},
	})
}
