package jsonata

import (
	"context"
	"errors"
	"testing"

	"github.com/mellowforge/jsontrans-go"
)

func TestStringLiteralsCannotBecomeInfixOperators(t *testing.T) {
	engine, _ := New()
	for _, expression := range []string{`"""#" ""`, `"" "@" ""`, `"" "+" ""`, `1 "and" 2`, `1 "(" 2`, `1 "~>" 2`, "1 `#` 2", "1 `and` 2", "1 `+` 2"} {
		_, err := engine.Compile(context.Background(), expression)
		var native *jsontrans.Error
		if !errors.As(err, &native) || native.Code != "S0201" {
			t.Errorf("%s: expected native S0201, got %v", expression, err)
		}
	}
}
