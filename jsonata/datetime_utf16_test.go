package jsonata

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
)

func TestReferenceUTF16DatePictures(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$fromMillis(0,"[Y0001]\ud800[M01]\udfff[D01]")`, `null`, `"1970\ud80001\udfff01"`},
		{`$fromMillis(0,"[Y00\ud80000]")`, `null`, `"19\ud80070"`},
		{`$toMillis(value,picture)`, `{"value":"\ue000\ud8001970-01-01😀\udfff","picture":"\ue000\ud800[Y0001]-[M01]-[D01]😀\udfff"}`, `0`},
		{`$toMillis(value,picture)`, `{"value":"\ue0001970-01-01","picture":"\ud800[Y0001]-[M01]-[D01]"}`, "undefined"},
		{`$toMillis(value,picture)`, `{"value":"\ud8011970-01-01","picture":"\ud800[Y0001]-[M01]-[D01]"}`, "undefined"},
		{`$toMillis(value,picture)`, `{"value":"\ud8001970-01-01","picture":"\ue000[Y0001]-[M01]-[D01]"}`, "undefined"},
		{`$toMillis(value,picture)`, `{"value":"\ufffd1970-01-01","picture":"\ud800[Y0001]-[M01]-[D01]"}`, "undefined"},
		{`$fromMillis(0,"[Z01\ud80001]","+0169")`, `null`, `"+01\ud80069"`},
		{`$fromMillis(0,"[z01\udfff01]","-0530")`, `null`, `"GMT-06\udfff30"`},
		{`$toMillis("1970-01-01GMT+01\ud80030", "[Y0001]-[M01]-[D01][z01\ud80001]")`, `null`, `-5400000`},
		{`$toMillis("1970-01-01-05\ud80030", "[Y0001]-[M01]-[D01][Z01\ud80001]")`, `null`, `16200000`},
		{`$toMillis("1970-01-01+99999999999999999999\ud80030", "[Y0001]-[M01]-[D01][Z01\ud80001]")`, `null`, `-3.5999999999999996e+26`},
		{`$toMillis("19\ud80070-01-01","[Y00\ud80000]-[M01]-[D01]")`, `null`, "undefined"},
	})
}

func TestReferenceUTF16DatePictureDiagnostics(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$fromMillis(0,"[Z\ud800]")`, `null`, "ERR D3134"},
		{`$fromMillis(0,"[Zw]")`, `null`, "ERR D3134"},
		{`$toMillis("1970-01-01+01","[Y0001]-[M01]-[D01][Z\ud800]")`, `null`, `-3600000`},
		{`$toMillis("1970-01-01","[Y0001]-[M0\ud800٠]-[D01]")`, `null`, "ERR D3131"},
	})
}

func TestUTF16DateTimezoneErrorsIdentifyTheDateFunction(t *testing.T) {
	ctx := context.Background()
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ expression, token string }{
		{`$fromMillis(0,"[Z٠\ud8000]")`, "fromMillis"},
		{`$toMillis("1970-01-01+01a30","[Y0001]-[M01]-[D01][Z٠\ud8000]")`, "toMillis"},
	} {
		program, err := engine.Compile(ctx, tc.expression)
		if err != nil {
			t.Fatal(err)
		}
		_, err = jsontrans.EvalJSON(ctx, program, []byte("null"), jsontrans.EvalOptions{})
		var native *jsontrans.Error
		if !errors.As(err, &native) || native.Code != "D3131" || native.Token != tc.token {
			t.Errorf("%s: got %v, want D3131 from %s", tc.expression, err, tc.token)
		}
	}
}

func TestUTF16DateProjectionChargesWork(t *testing.T) {
	limits := budget.DefaultLimits()
	limits.MaxSteps = 1
	b, err := budget.New(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	surrogate := regexUnitsString([]rune{0xd800})
	subject := strings.Repeat(surrogate, 256) + "1970-01-01"
	_, err = dateParsePicture(&CallContext{Budget: b}, subject, surrogate+"[Y0001]-[M01]-[D01]")
	if !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("got %v, want step limit", err)
	}
}
