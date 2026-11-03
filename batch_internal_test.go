package jsontrans

import (
	"context"
	"testing"
	"time"

	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

type regexLimitsProgram struct {
	handler *prepared.Handler
	seen    Limits
}

func (p *regexLimitsProgram) Limits() Limits                    { return Limits{RegexTimeout: 250 * time.Millisecond} }
func (p *regexLimitsProgram) NativePrepared() *prepared.Handler { return p.handler }
func (p *regexLimitsProgram) Eval(context.Context, any, EvalOptions) (Result, error) {
	panic("wrong dispatch")
}
func TestPreparedNativeRegexTimeoutBelongsToEngine(t *testing.T) {
	p := &regexLimitsProgram{}
	p.handler = prepared.New(p, func(req prepared.Request) (prepared.Result, error) {
		p.seen = req.Limits
		return prepared.Result{}, nil
	})
	checked := mustChecked(t, "forwarded", p, DefaultLimits())
	d, err := PrepareJSON(context.Background(), []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EvalPrepared(context.Background(), checked, d, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if p.seen.RegexTimeout != 250*time.Millisecond {
		t.Fatalf("wrapper replaced native regex timeout: %v", p.seen.RegexTimeout)
	}
	override := EvalOptions{Limits: &Limits{RegexTimeout: time.Second}}
	if _, err := EvalPrepared(context.Background(), checked, d, override); err != nil {
		t.Fatal(err)
	}
	if p.seen.RegexTimeout != time.Second {
		t.Fatalf("explicit regex override lost: %v", p.seen.RegexTimeout)
	}
}
