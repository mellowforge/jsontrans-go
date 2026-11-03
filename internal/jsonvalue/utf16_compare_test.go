package jsonvalue

import (
	"errors"
	"strings"
	"testing"
)

func TestCompareUTF16UsesCodeUnitOrder(t *testing.T) {
	const (
		grin      = "\U0001F600"   // UTF-16 D83D DE00
		fullTilde = "\uff5e"       // UTF-16 FF5E
		loneD800  = "\xed\xa0\x80" // WTF-8 isolated D800
		loneDBFF  = "\xed\xaf\xbf" // WTF-8 isolated DBFF
		splitGrin = "\xed\xa0\xbd\xed\xb8\x80"
	)
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"supplementary before late BMP", grin, fullTilde, -1},
		{"late BMP after supplementary", fullTilde, grin, 1},
		{"after common prefix", "x" + grin, "x\uffff", -1},
		{"lone high surrogate below pair", loneD800, grin, -1},
		{"lone high surrogate above pair", loneDBFF, grin, 1},
		{"private use after surrogate", "\ue000", loneD800, 1},
		{"unpaired spelling equals pair", splitGrin, grin, 0},
		{"ascii", "abc", "abd", -1},
		{"prefix", "ab", "abc", -1},
		{"equal", "a" + grin, "a" + grin, 0},
		{"empty", "", "a", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareUTF16(tt.a, tt.b, nil)
			if err != nil || got != tt.want {
				t.Fatalf("got %d, %v; want %d", got, err, tt.want)
			}
		})
	}
}

func TestCompareUTF16ChecksLongComparisons(t *testing.T) {
	prefix := strings.Repeat("a", 3*4096)
	stop := errors.New("stop")
	calls := 0
	_, err := CompareUTF16(prefix+"b", prefix+"c", func() error {
		calls++
		if calls == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Fatalf("got %v after %d checks, want stop", err, calls)
	}
}
