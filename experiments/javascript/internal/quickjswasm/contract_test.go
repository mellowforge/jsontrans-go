package quickjswasm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
)

func TestOwnedTimeoutOverrideReachesGuest(t *testing.T) {
	e, err := New(context.Background(), Config{Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx, done, err := e.operation(context.Background(), 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	deadline, _ := ctx.Deadline()
	if time.Until(deadline) < 150*time.Millisecond {
		t.Fatal("override was capped by engine default")
	}
}
func TestOwnedTimeSnapshotProfile(t *testing.T) {
	e, err := New(context.Background(), Config{Timeout: semanticTestTimeout})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	rule, err := e.Compile(context.Background(), `()=>[Date.now(),new Date().getTime(),new Date().getTimezoneOffset(),Math.random()]`)
	if err != nil {
		t.Fatal(err)
	}
	codec, err := e.Compile(context.Background(), `rule=>[0,JSON.stringify(rule())]`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	m, err := e.runtime.InstantiateModule(ctx, e.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(ctx)
	const micros uint64 = 1700000000123000
	if _, err = call(ctx, m, "abi_init", 64<<20, 0, uint64(uint32(micros&0xffffffff)), micros>>32); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		args := []uint64{}
		for _, data := range [][]byte{rule, codec, []byte(`null`), []byte(`{}`)} {
			ptr, err := transfer(ctx, m, data, maxTransfer)
			if err != nil {
				t.Fatal(err)
			}
			args = append(args, ptr, uint64(len(data)))
		}
		args = append(args, 256, 100000, 1024, 1000000)
		if _, err = call(ctx, m, "abi_eval", args...); err != nil {
			t.Fatal(err)
		}
		raw, err := output(ctx, m, 2048)
		if err != nil {
			t.Fatal(err)
		}
		var values []float64
		if err = json.Unmarshal(raw, &values); err != nil {
			t.Fatal(err)
		}
		if len(values) != 4 || values[0] != 1700000000123 || values[1] != 1700000000123 || values[2] != 0 || values[3] < 0 || values[3] >= 1 {
			t.Fatalf("snapshot %s", raw)
		}
	}
}
