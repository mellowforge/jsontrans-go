package javascript

import (
	"context"
	"fmt"
	"github.com/dop251/goja"
	jt "github.com/mellowforge/jsontrans-go"
)

type gojaPrograms struct{ rule, bridge *goja.Program }

func compileGoja(source string) (any, error) {
	rule, err := goja.Compile("rule.js", source, true)
	if err != nil {
		return nil, err
	}
	codec, err := goja.Compile("bridge.js", bridge, true)
	if err != nil {
		return nil, err
	}
	return gojaPrograms{rule, codec}, nil
}
func (p *Program) runGoja(ctx context.Context, input, bindings string, l jt.Limits) (string, error) {
	vm := goja.New()
	vm.SetMaxCallStackSize(256)
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { vm.Interrupt(ctx.Err()); close(done) })
	defer func() {
		if !stop() {
			<-done
		}
	}()
	programs := p.goja.(gojaPrograms)
	codec, err := vm.RunProgram(programs.bridge)
	if err != nil {
		return "", err
	}
	rule, err := vm.RunProgram(programs.rule)
	if err != nil {
		return "", err
	}
	fn, ok := goja.AssertFunction(codec)
	if !ok {
		return "", fmt.Errorf("bridge not callable")
	}
	value, err := fn(goja.Undefined(), rule, vm.ToValue(input), vm.ToValue(bindings), vm.ToValue(l.MaxDepth), vm.ToValue(l.MaxCollectionItems), vm.ToValue(l.MaxOutputBytes), vm.ToValue(l.MaxSteps))
	if err != nil {
		return "", err
	}
	return value.String(), nil
}
