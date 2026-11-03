package javascript

import (
	"context"
	publicjs "github.com/mellowforge/jsontrans-go/javascript"
	"testing"
)

func TestOwnedAssessmentUsesPublicProgram(t *testing.T) {
	e := newTestEngine(t, OwnedQuickJS)
	p, err := e.Compile(context.Background(), `()=>1`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(*publicjs.Program); !ok {
		t.Fatalf("owned assessment compiled %T, want public module Program", p)
	}
}
