// Package quickjswasm is an experimental, project-owned QuickJS/Wazero boundary.
// Its bytecode API accepts only artifacts compiled by this package, never user bytes.
package quickjswasm

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

//go:embed quickjs.wasm
var guest []byte

//go:embed guest/lock.json
var manifest []byte

var ErrClosed = errors.New("owned QuickJS engine closed")

const maxTransfer = 16 << 20
const maxBytecode = 4 << 20

type Config struct {
	MemoryPages uint32
	StackBytes  uint32
	Timeout     time.Duration
}
type Limits struct {
	Depth, Items, Bytes int
	Work                int64
	Timeout             time.Duration
}
type Stats struct{ GuestBefore, GuestAfter uint32 }
type Engine struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	cache    wazero.CompilationCache
	config   Config
	ctx      context.Context
	cancel   context.CancelFunc
	closed   atomic.Bool
	once     sync.Once
	closeErr error
}

func New(ctx context.Context, c Config) (*Engine, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if c.MemoryPages == 0 {
		c.MemoryPages = 1024
	}
	if c.StackBytes != 0 {
		return nil, fmt.Errorf("%w: QuickJS stack quotas are disabled under WASI", jt.ErrUnsupported)
	}
	if c.Timeout == 0 {
		c.Timeout = time.Second
	}
	if c.MemoryPages < 32 || c.MemoryPages > 1024 || c.Timeout < 0 {
		return nil, fmt.Errorf("%w: owned guest pages 32..1024 and positive timeout", jt.ErrUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var lock struct {
		ArtifactSHA256 string `json:"artifact_sha256"`
	}
	if err := json.Unmarshal(manifest, &lock); err != nil {
		return nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(guest)) != lock.ArtifactSHA256 {
		return nil, errors.New("guest artifact integrity failure")
	}
	cache := wazero.NewCompilationCache()
	root, stop := context.WithCancel(context.Background())
	e := &Engine{cache: cache, config: c, ctx: root, cancel: stop}
	e.runtime = wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfig().WithCompilationCache(cache).WithMemoryLimitPages(c.MemoryPages).WithCloseOnContextDone(true))
	module, err := e.runtime.CompileModule(ctx, guest)
	if err == nil {
		err = validateModule(module, c.MemoryPages)
	}
	if err != nil {
		e.Close()
		return nil, err
	}
	e.compiled = module
	if err = ctx.Err(); err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}
func validateModule(m wazero.CompiledModule, pages uint32) error {
	if len(m.ImportedFunctions()) != 0 || len(m.ImportedMemories()) != 0 {
		return errors.New("guest imports forbidden")
	}
	expected := map[string]bool{"_initialize": true, "abi_version": true, "abi_init": true, "abi_alloc": true, "abi_free": true, "abi_compile": true, "abi_eval": true, "abi_output_ptr": true, "abi_output_len": true, "abi_status": true}
	for name := range m.ExportedFunctions() {
		if !expected[name] {
			return fmt.Errorf("unexpected guest export %s", name)
		}
	}
	for name := range expected {
		if m.ExportedFunctions()[name] == nil {
			return fmt.Errorf("missing guest export %s", name)
		}
	}
	memories := m.ExportedMemories()
	memory := memories["memory"]
	if len(memories) != 1 || memory == nil {
		return errors.New("unexpected guest memories")
	}
	maximum, has := memory.Max()
	if !has || maximum > pages {
		return errors.New("guest memory cap absent")
	}
	return nil
}
func (e *Engine) Close() error {
	e.once.Do(func() {
		e.closed.Store(true)
		e.cancel()
		e.closeErr = errors.Join(e.runtime.Close(context.Background()), e.cache.Close(context.Background()))
	})
	return e.closeErr
}
func (e *Engine) operation(ctx context.Context, timeout time.Duration) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("nil context")
	}
	if e.closed.Load() {
		return nil, nil, ErrClosed
	}
	if timeout == 0 {
		timeout = e.config.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	stop := context.AfterFunc(e.ctx, cancel)
	done := func() { stop(); cancel() }
	if err := ctx.Err(); err != nil {
		done()
		return nil, nil, err
	}
	return ctx, done, nil
}
func (e *Engine) instantiate(ctx context.Context) (api.Module, error) {
	if e.closed.Load() {
		return nil, ErrClosed
	}
	m, err := e.runtime.InstantiateModule(ctx, e.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		return nil, e.failure(ctx, err)
	}
	fail := func(err error) (api.Module, error) { m.Close(context.Background()); return nil, e.failure(ctx, err) }
	version, err := call(ctx, m, "abi_version")
	if err != nil {
		return fail(err)
	}
	if version != 2 {
		return fail(errors.New("guest ABI version mismatch"))
	}
	now := uint64(time.Now().UnixMicro())
	status, err := call(ctx, m, "abi_init", uint64(e.config.MemoryPages)*65536, 0, uint64(uint32(now)), now>>32)
	if err != nil {
		return fail(err)
	}
	if status != 0 {
		return fail(fmt.Errorf("%w: guest initialization", jt.ErrLimit))
	}
	return m, nil
}
func (e *Engine) failure(ctx context.Context, err error) error {
	if e.closed.Load() {
		return errors.Join(ErrClosed, ctx.Err(), err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func call(ctx context.Context, m api.Module, name string, args ...uint64) (uint64, error) {
	fn := m.ExportedFunction(name)
	if fn == nil {
		return 0, fmt.Errorf("missing ABI function %s", name)
	}
	out, err := fn.Call(ctx, args...)
	if err != nil {
		return 0, err
	}
	if len(out) == 0 {
		return 0, nil
	}
	return out[0], nil
}
func transfer(ctx context.Context, m api.Module, data []byte, maximum int) (uint64, error) {
	if len(data) > maximum {
		return 0, fmt.Errorf("%w: ABI transfer bytes", jt.ErrLimit)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	ptr, err := call(ctx, m, "abi_alloc", uint64(len(data)+1))
	if err != nil {
		return 0, err
	}
	if ptr == 0 {
		return 0, fmt.Errorf("%w: guest allocation", jt.ErrLimit)
	}
	if !m.Memory().Write(uint32(ptr), data) || !m.Memory().WriteByte(uint32(ptr)+uint32(len(data)), 0) {
		return 0, errors.New("ABI memory write out of bounds")
	}
	return ptr, ctx.Err()
}
func output(ctx context.Context, m api.Module, maximum int) ([]byte, error) {
	status, err := call(ctx, m, "abi_status")
	if err != nil {
		return nil, err
	}
	length, err := call(ctx, m, "abi_output_len")
	if err != nil {
		return nil, err
	}
	if status != 0 {
		maximum = 65536
	}
	if length > uint64(maximum) {
		return nil, fmt.Errorf("%w: ABI output bytes", jt.ErrLimit)
	}
	ptr, err := call(ctx, m, "abi_output_ptr")
	if err != nil {
		return nil, err
	}
	data, ok := m.Memory().Read(uint32(ptr), uint32(length))
	if !ok {
		return nil, errors.New("ABI memory read out of bounds")
	}
	owned := append([]byte(nil), data...)
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if status != 0 {
		if status != 1 {
			return nil, errors.New("invalid owned diagnostic status")
		}
		f, err := decodeFailure(ctx, owned)
		if err != nil {
			return nil, err
		}
		return nil, f
	}
	return owned, nil
}
func (e *Engine) Compile(ctx context.Context, source string) ([]byte, error) {
	if len(source) > maxTransfer {
		return nil, fmt.Errorf("%w: source transfer bytes", jt.ErrLimit)
	}
	ctx, done, err := e.operation(ctx, e.config.Timeout)
	if err != nil {
		return nil, err
	}
	defer done()
	m, err := e.instantiate(ctx)
	if err != nil {
		return nil, err
	}
	defer m.Close(context.Background())
	ptr, err := transfer(ctx, m, []byte(source), maxTransfer)
	if err != nil {
		return nil, e.failure(ctx, err)
	}
	if _, err = call(ctx, m, "abi_compile", ptr, uint64(len(source)), 0); err != nil {
		return nil, e.failure(ctx, err)
	}
	code, err := output(ctx, m, maxBytecode)
	return code, e.failure(ctx, err)
}
func (e *Engine) Eval(ctx context.Context, rule, bridge []byte, input, bindings string, l Limits) (string, error) {
	text, _, err := e.EvalStats(ctx, rule, bridge, input, bindings, l)
	return text, err
}
func (e *Engine) EvalStats(ctx context.Context, rule, bridge []byte, input, bindings string, l Limits) (text string, stats Stats, err error) {
	if len(rule) > maxBytecode || len(bridge) > maxBytecode || len(input) > maxTransfer || len(bindings) > maxTransfer {
		return "", stats, fmt.Errorf("%w: ABI transfer bytes", jt.ErrLimit)
	}
	if l.Depth <= 0 || uint64(l.Depth) > math.MaxUint32 || l.Items <= 0 || uint64(l.Items) > math.MaxUint32 || l.Bytes <= 0 || l.Bytes > maxTransfer || l.Work <= 0 || l.Work > math.MaxUint32 {
		return "", stats, fmt.Errorf("%w: ABI conversion limits", jt.ErrUnsupported)
	}
	ctx, done, err := e.operation(ctx, l.Timeout)
	if err != nil {
		return "", stats, err
	}
	defer done()
	m, err := e.instantiate(ctx)
	if err != nil {
		return "", stats, err
	}
	defer m.Close(context.Background())
	stats.GuestBefore = m.Memory().Size()
	args := make([]uint64, 0, 12)
	for i, data := range [][]byte{rule, bridge, []byte(input), []byte(bindings)} {
		maximum := maxTransfer
		if i < 2 {
			maximum = maxBytecode
		}
		ptr, x := transfer(ctx, m, data, maximum)
		if x != nil {
			return "", stats, e.failure(ctx, x)
		}
		args = append(args, ptr, uint64(len(data)))
	}
	args = append(args, uint64(l.Depth), uint64(l.Items), uint64(l.Bytes), uint64(l.Work))
	_, err = call(ctx, m, "abi_eval", args...)
	if err != nil {
		return "", stats, e.failure(ctx, err)
	}
	stats.GuestAfter = m.Memory().Size()
	data, err := output(ctx, m, l.Bytes+32)
	return string(data), stats, e.failure(ctx, err)
}
