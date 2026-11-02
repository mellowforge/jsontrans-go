// Portions derived from JSONata 2.2.2, Copyright IBM Corp. 2016, 2018. MIT license; see conformance/testdata/jsonata/LICENSE.
package jsonata

import (
	"math"
	"strconv"
	"strings"
)

func builtinNumberPictures(m map[string]Callable) {
	m["formatNumber"] = NativeFunction{N: 3, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		n, ok := v.(float64)
		if !ok {
			return nil, builtinError("T0410", "formatNumber")
		}
		pic, ok := builtinArg(a, 1).(string)
		if !ok {
			return nil, builtinError("T0410", "formatNumber")
		}
		opts, _ := builtinArg(a, 2).(map[string]any)
		return formatNumber(c, n, pic, opts)
	}}
}
func formatNumber(c *CallContext, n float64, picture string, opts map[string]any) (Value, error) {
	if err := builtinCollection(c, len(picture)); err != nil {
		return nil, err
	}
	for range picture {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
	}
	props := map[string]string{"decimal-separator": ".", "grouping-separator": ",", "exponent-separator": "e", "minus-sign": "-", "percent": "%", "per-mille": "‰", "zero-digit": "0", "digit": "#", "pattern-separator": ";"}
	for k, v := range opts {
		if s, ok := v.(string); ok && s != "" {
			props[k] = s
		}
	}
	parts := strings.Split(picture, props["pattern-separator"])
	if len(parts) > 2 {
		return nil, builtinError("D3080", "formatNumber")
	}
	p := parts[0]
	negative := n < 0
	if negative && len(parts) == 2 {
		p = parts[1]
	}
	zero := []rune(props["zero-digit"])[0]
	digit := []rune(props["digit"])[0]
	dec := []rune(props["decimal-separator"])[0]
	group := []rune(props["grouping-separator"])[0]
	exp := []rune(props["exponent-separator"])[0]
	r := []rune(p)
	active := func(x rune) bool { return x >= zero && x < zero+10 || x == digit || x == dec || x == group }
	first, last := -1, -1
	for i, x := range r {
		if active(x) {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return nil, builtinError("D3086", "formatNumber")
	}
	prefix, suffix := string(r[:first]), string(r[last+1:])
	body := string(r[first : last+1])
	code := ""
	if strings.Count(p, props["decimal-separator"]) > 1 {
		code = "D3081"
	}
	if strings.Count(p, props["percent"]) > 1 {
		code = "D3082"
	}
	if strings.Count(p, props["per-mille"]) > 1 {
		code = "D3083"
	}
	if strings.Contains(p, props["percent"]) && strings.Contains(p, props["per-mille"]) {
		code = "D3084"
	}
	bodyRunes := []rune(body)
	for _, x := range bodyRunes {
		if !active(x) && x != exp {
			code = "D3086"
		}
	}
	if strings.Contains(body, string(group)+string(dec)) || strings.Contains(body, string(dec)+string(group)) {
		code = "D3087"
	}
	if strings.HasSuffix(body, string(group)) {
		code = "D3088"
	}
	if strings.Contains(body, string(group)+string(group)) {
		code = "D3089"
	}
	ep := strings.SplitN(body, string(exp), 2)
	mant := ep[0]
	exponent := len(ep) > 1
	if exponent {
		if strings.Contains(p, props["percent"]) || strings.Contains(p, props["per-mille"]) {
			code = "D3092"
		}
		if ep[1] == "" {
			code = "D3093"
		}
		for _, x := range ep[1] {
			if x < zero || x >= zero+10 {
				code = "D3093"
			}
		}
	}
	dp := strings.SplitN(mant, string(dec), 2)
	ip := dp[0]
	fp := ""
	if len(dp) > 1 {
		fp = dp[1]
	}
	seenRequired := false
	for _, x := range ip {
		if x >= zero && x < zero+10 {
			seenRequired = true
		}
		if x == digit && seenRequired {
			code = "D3090"
		}
	}
	seenOptional := false
	for _, x := range fp {
		if x == digit {
			seenOptional = true
		}
		if x >= zero && x < zero+10 && seenOptional {
			code = "D3091"
		}
	}
	hasDigits := false
	for _, x := range mant {
		if x == digit || x >= zero && x < zero+10 {
			hasDigits = true
		}
	}
	if !hasDigits && code == "" {
		code = "D3085"
	}
	if code != "" {
		return nil, builtinError(code, "formatNumber")
	}
	n = math.Abs(n)
	if strings.Contains(p, props["percent"]) {
		n *= 100
	}
	if strings.Contains(p, props["per-mille"]) {
		n *= 1000
	}
	minInt, minFrac, maxFrac := 0, 0, 0
	for _, x := range ip {
		if x >= zero && x < zero+10 {
			minInt++
		}
	}
	scaling := minInt
	for _, x := range fp {
		if x >= zero && x < zero+10 {
			minFrac++
			maxFrac++
		} else if x == digit {
			maxFrac++
		}
	}
	if minInt == 0 && maxFrac == 0 {
		if exponent {
			minFrac = 1
			maxFrac = 1
		} else {
			minInt = 1
		}
	}
	if exponent && minInt == 0 && strings.Contains(ip, string(digit)) {
		minInt = 1
	}
	if minInt == 0 && minFrac == 0 {
		minFrac = 1
	}
	ex := 0
	if exponent && n != 0 {
		ex = int(math.Floor(math.Log10(n))) - scaling + 1
		n /= math.Pow10(ex)
	}
	if maxFrac > 100 {
		return nil, builtinError("D3085", "formatNumber")
	}
	scale := math.Pow10(maxFrac)
	if !math.IsInf(n*scale, 0) {
		n = math.RoundToEven(n*scale) / scale
	}
	s := strconv.FormatFloat(n, 'f', maxFrac, 64)
	pieces := strings.SplitN(s, ".", 2)
	integer := pieces[0]
	fraction := ""
	if len(pieces) > 1 {
		fraction = pieces[1]
	}
	for len(fraction) > minFrac && strings.HasSuffix(fraction, "0") {
		fraction = fraction[:len(fraction)-1]
	}
	if err := builtinCollection(c, minInt); err != nil {
		return nil, err
	}
	if len(integer) < minInt {
		integer = strings.Repeat("0", minInt-len(integer)) + integer
	}
	if minInt == 0 && integer == "0" {
		integer = ""
	}
	if err := builtinCollection(c, len(integer)+len(fraction)); err != nil {
		return nil, err
	}
	var positions []int
	count := 0
	for i := len([]rune(ip)) - 1; i >= 0; i-- {
		x := []rune(ip)[i]
		if x == group {
			positions = append(positions, count)
		} else if x == digit || x >= zero && x < zero+10 {
			count++
		}
	}
	regular := len(positions) > 0
	step := 0
	if regular {
		step = positions[0]
		for i, x := range positions {
			if x != step*(i+1) {
				regular = false
			}
		}
	}
	ib := boundedStringBuilder{c: c}
	for i, x := range integer {
		remaining := len(integer) - i
		if i > 0 {
			insert := regular && step > 0 && remaining%step == 0
			if !regular {
				for _, p := range positions {
					if remaining == p {
						insert = true
					}
				}
			}
			if insert {
				ib.WriteRune(group)
			}
		}
		ib.WriteRune(zero + (x - '0'))
	}
	fb := boundedStringBuilder{c: c}
	pos := 0
	for _, x := range fp {
		if x == group {
			if pos < len(fraction) {
				fb.WriteRune(group)
			}
		} else if x == digit || x >= zero && x < zero+10 {
			if pos < len(fraction) {
				fb.WriteRune(zero + rune(fraction[pos]-'0'))
				pos++
			}
		}
	}
	for pos < len(fraction) {
		fb.WriteRune(zero + rune(fraction[pos]-'0'))
		pos++
	}
	if ib.err != nil {
		return nil, ib.err
	}
	if fb.err != nil {
		return nil, fb.err
	}
	initialSize := len(prefix) + len(ib.String()) + len(suffix)
	if len(fraction) > 0 {
		initialSize += len(fb.String()) + len(string(dec))
	}
	if negative && len(parts) == 1 {
		initialSize += len(props["minus-sign"])
	}
	if err := builtinOutput(c, initialSize); err != nil {
		return nil, err
	}
	result := prefix + ib.String()
	if len(fraction) > 0 {
		result += string(dec) + fb.String()
	}
	if exponent {
		es := strconv.Itoa(int(math.Abs(float64(ex))))
		width := len([]rune(ep[1]))
		if err := builtinCollection(c, width); err != nil {
			return nil, err
		}
		if len(es) < width {
			es = strings.Repeat("0", width-len(es)) + es
		}
		eb := boundedStringBuilder{c: c}
		for _, x := range es {
			eb.WriteRune(zero + x - '0')
		}
		sign := ""
		if ex < 0 {
			sign = props["minus-sign"]
		}
		if eb.err != nil {
			return nil, eb.err
		}
		if err := builtinOutput(c, len(result)+len(string(exp))+len(sign)+len(eb.String())+len(suffix)); err != nil {
			return nil, err
		}
		result += string(exp) + sign + eb.String()
	}
	finalSize := len(result) + len(suffix)
	if negative && len(parts) == 1 {
		finalSize += len(props["minus-sign"])
	}
	if err := builtinOutput(c, finalSize); err != nil {
		return nil, err
	}
	result += suffix
	if negative && len(parts) == 1 {
		result = props["minus-sign"] + result
	}
	return result, nil
}
