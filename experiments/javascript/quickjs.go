package javascript

import (
	"context"
	"fmt"
	"io"

	jt "github.com/mellowforge/jsontrans-go"
	qjs "github.com/mellowforge/jsontrans-go/experiments/javascript/internal/qjscompat"
)

func (e *Engine) newQuick(ctx context.Context) (*qjs.Runtime, error) {
	return qjs.New(qjs.Option{Context: ctx, CloseOnContextDone: true, CWD: e.dir, MemoryLimit: e.config.GuestMemoryBytes, MaxStackSize: e.config.GuestStackBytes, Stdout: io.Discard, Stderr: io.Discard})
}

// Recover only at the upstream ABI boundary: the wrapper converts traps into
// panics. Cleanup after a canceled context can also panic; the gate suite records it.
func catchQuick(err *error) {
	if v := recover(); v != nil {
		*err = fmt.Errorf("QJS ABI panic: %v", v)
	}
}
func (e *Engine) compileQuick(ctx context.Context, source string) (code, codec []byte, err error) {
	defer catchQuick(&err)
	rt, err := e.newQuick(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer rt.Close()
	code, err = rt.Compile("rule.js", qjs.Code(source))
	if err != nil {
		return nil, nil, err
	}
	codec, err = rt.Compile("bridge.js", qjs.Code(bridge))
	return code, codec, err
}
func (p *Program) runQuick(ctx context.Context, input, bindings string, l jt.Limits) (text string, err error) {
	defer catchQuick(&err)
	rt, err := p.engine.newQuick(ctx)
	if err != nil {
		return "", err
	}
	defer rt.Close()
	c := rt.Context()
	fn, err := c.Eval("bridge.js", qjs.Bytecode(p.quickBridge))
	if err != nil {
		return "", err
	}
	defer fn.Free()
	rule, err := c.Eval("rule.js", qjs.Bytecode(p.quick))
	if err != nil {
		return "", err
	}
	defer rule.Free()
	args := []*qjs.Value{rule, c.NewString(input), c.NewString(bindings), c.NewInt64(int64(l.MaxDepth)), c.NewInt64(int64(l.MaxCollectionItems)), c.NewInt64(int64(l.MaxOutputBytes)), c.NewInt64(l.MaxSteps)}
	defer func() {
		for _, v := range args[1:] {
			v.Free()
		}
	}()
	this := c.NewUndefined()
	defer this.Free()
	value, err := c.Invoke(fn, this, args...)
	if err != nil {
		return "", err
	}
	defer value.Free()
	return value.String(), nil
}
