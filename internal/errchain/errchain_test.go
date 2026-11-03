package errchain

import (
	"errors"
	"fmt"
	"testing"
)

type owned struct {
	owner string
	inner error
}

func (o *owned) Error() string { return o.owner }
func (o *owned) Unwrap() error { return o.inner }

func TestFindStopsAtRejectedOwner(t *testing.T) {
	mine := func(o *owned) bool { return o.owner == "mine" }
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"direct", &owned{owner: "mine"}, "mine"},
		{"wrapped", fmt.Errorf("context: %w", &owned{owner: "mine"}), "mine"},
		{"joined", errors.Join(errors.New("plain"), &owned{owner: "mine"}), "mine"},
		{"behind other owner", &owned{owner: "other", inner: &owned{owner: "mine"}}, ""},
		{"plain", errors.New("plain"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Find(tt.err, mine)
			if tt.want == "" {
				if ok {
					t.Fatalf("found %v", got)
				}
				return
			}
			if !ok || got.owner != tt.want {
				t.Fatalf("got %v, %v", got, ok)
			}
		})
	}
}
