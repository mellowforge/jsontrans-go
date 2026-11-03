package quickjswasm

import (
	"context"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"strings"
	"testing"
)

func TestDiagnosticProtocolValidation(t *testing.T) {
	good := `{"version":2,"kind":"exception","stage":"evaluate","name":"TypeError","message":"x\ud800","stack":"","truncated":false}`
	f, err := decodeFailure(context.Background(), []byte(good))
	if err != nil || f.Kind != "exception" || !strings.HasSuffix(f.Message, "\xed\xa0\x80") {
		t.Fatalf("record %#v %v", f, err)
	}
	for _, bad := range []string{
		strings.Replace(good, `"version":2`, `"version":1`, 1),
		strings.Replace(good, `"kind":"exception"`, `"kind":"other"`, 1),
		strings.Replace(good, `"stage":"evaluate"`, `"stage":"input"`, 1),
		strings.Replace(good, `"name":"TypeError",`, "", 1),
		strings.Replace(good, `"truncated":false`, `"truncated":"false"`, 1),
		strings.TrimSuffix(good, "}") + `,"extra":1}`,
		strings.TrimSuffix(good, "}") + `,"line":-1,"column":1,"source":"rule.js"}`,
		strings.TrimSuffix(good, "}") + `,"line":99999999999999999999999999999999,"column":1,"source":"rule.js"}`,
		strings.Repeat(" ", 65537) + good,
	} {
		if _, err := decodeFailure(context.Background(), []byte(bad)); err == nil {
			t.Errorf("accepted malformed record %.120s", bad)
		}
	}
}
func FuzzDiagnosticRecord(f *testing.F) {
	f.Add([]byte(`{"version":2,"kind":"exception","stage":"evaluate","name":"","message":"x","stack":"","truncated":false}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65537 {
			return
		}
		_, _ = decodeFailure(context.Background(), data)
	})
}

func TestMalformedDiagnosticCannotBecomeLimit(t *testing.T) {
	_, err := decodeFailure(context.Background(), []byte(`{"version":2,"kind":"exception","stage":"evaluate","name":"","message":"x","stack":"","truncated":false,"extra":[[[[[[0]]]]]]}`))
	if err == nil || errors.Is(err, jt.ErrLimit) {
		t.Fatalf("malformed diagnostic acquired resource status: %v", err)
	}
}
