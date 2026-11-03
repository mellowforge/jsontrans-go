// Portions derived from JSONata 2.2.2, Copyright IBM Corp. 2016, 2018. MIT license; see conformance/testdata/jsonata/LICENSE.
package jsonata

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func builtinInteger(m map[string]Callable) {
	m["formatInteger"] = NativeFunction{N: 2, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		n, ok := v.(float64)
		if !ok {
			return nil, builtinError("T0410", "formatInteger")
		}
		p, ok := builtinArg(a, 1).(string)
		if !ok {
			return nil, builtinError("T0410", "formatInteger")
		}
		return formatInteger(c, n, p)
	}}
	m["parseInteger"] = NativeFunction{N: 2, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		s, ok := v.(string)
		if !ok {
			return nil, builtinError("T0410", "parseInteger")
		}
		p, ok := builtinArg(a, 1).(string)
		if !ok {
			return nil, builtinError("T0410", "parseInteger")
		}
		return parseInteger(c, s, p)
	}}
}
func integerPicture(p string) (string, bool) {
	parts := strings.SplitN(p, ";", 2)
	return parts[0], len(parts) > 1 && strings.HasPrefix(parts[1], "o")
}
func formatInteger(c *CallContext, n float64, p string) (Value, error) {
	if err := builtinStep(c); err != nil {
		return nil, err
	}
	pic, ordinal := integerPicture(p)
	// As the reference: floor before taking the sign, and -0 prints as 0.
	n = math.Floor(n) + 0
	if n < 0 {
		r, e := formatInteger(c, -n, p)
		if e != nil {
			return nil, e
		}
		if err := builtinOutput(c, len(r.(string))+1); err != nil {
			return nil, err
		}
		return "-" + r.(string), nil
	}
	if math.IsInf(n, 0) || math.IsNaN(n) {
		switch pic {
		case "w", "W", "Ww", "A", "a", "I", "i":
			// The reference recurses without end on these; fail plainly.
			return nil, errors.New("formatInteger: number out of range")
		}
	}
	if pic == "w" || pic == "W" || pic == "Ww" {
		s := integerWords(n, false, ordinal)
		if pic == "w" {
			s = strings.ToLower(s)
		} else if pic == "W" {
			s = strings.ToUpper(s)
		} else {
			s = strings.ReplaceAll(s, " And ", " and ")
		}
		if err := builtinOutput(c, len(s)); err != nil {
			return nil, err
		}
		return s, nil
	}
	if pic == "A" || pic == "a" {
		var b []byte
		for n > 0 {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			n--
			if err := builtinOutput(c, len(b)+1); err != nil {
				return nil, err
			}
			b = append(b, byte(int(math.Mod(n, 26)))+'a')
			n = math.Floor(n / 26)
		}
		for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		s := string(b)
		if pic == "A" {
			s = strings.ToUpper(s)
		}
		return s, nil
	}
	if pic == "I" || pic == "i" {
		nums := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
		roman := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
		b := boundedStringBuilder{c: c}
		for i, v := range nums {
			for n >= float64(v) {
				if err := builtinStep(c); err != nil {
					return nil, err
				}
				b.WriteString(roman[i])
				if b.err != nil {
					return nil, b.err
				}
				n -= float64(v)
			}
		}
		s := b.String()
		if pic == "i" {
			s = strings.ToLower(s)
		}
		return s, nil
	}
	zero := rune(0)
	required := 0
	digits := 0
	type sep struct {
		at int
		r  rune
	}
	seps := []sep{}
	for _, r := range pic {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		if r == '#' {
			digits++
		} else if unicode.IsDigit(r) {
			z := r
			for z > 0 && unicode.IsDigit(z-1) {
				z--
			}
			if zero != 0 && zero != z {
				return nil, builtinError("D3131", "formatInteger")
			}
			zero = z
			required++
			digits++
		} else {
			seps = append(seps, sep{digits, r})
		}
	}
	if zero == 0 {
		return nil, builtinError("D3130", "formatInteger")
	}
	s := jsNumberString(n)
	if err := builtinOutput(c, required); err != nil {
		return nil, err
	}
	if err := builtinCollection(c, required); err != nil {
		return nil, err
	}
	if len(s) < required {
		s = strings.Repeat("0", required-len(s)) + s
	}
	if err := builtinCollection(c, len(s)); err != nil {
		return nil, err
	}
	rs := []rune(s)
	for i := range rs {
		if rs[i] >= '0' && rs[i] <= '9' {
			rs[i] = zero + (rs[i] - '0')
		}
	}
	if len(seps) > 0 {
		step := digits - seps[len(seps)-1].at
		regular := step > 0
		for i := len(seps) - 1; i >= 0; i-- {
			if seps[i].r != seps[len(seps)-1].r || digits-seps[i].at != step*(len(seps)-i) {
				regular = false
			}
		}
		var out []rune
		byteCount := 0
		appendRune := func(r rune) error {
			byteCount += utf8.RuneLen(r)
			if err := builtinOutput(c, byteCount); err != nil {
				return err
			}
			out = append(out, r)
			return nil
		}
		for i, r := range rs {
			remaining := len(rs) - i
			if i > 0 {
				if regular && remaining%step == 0 {
					if err := appendRune(seps[len(seps)-1].r); err != nil {
						return nil, err
					}
				} else if !regular {
					for _, sep := range seps {
						if remaining == digits-sep.at {
							if err := appendRune(sep.r); err != nil {
								return nil, err
							}
						}
					}
				}
			}
			if err := appendRune(r); err != nil {
				return nil, err
			}
		}
		rs = out
	}
	s = string(rs)
	if ordinal {
		x := int(math.Mod(n, 100))
		suffix := "th"
		if x < 11 || x > 13 {
			switch x % 10 {
			case 1:
				suffix = "st"
			case 2:
				suffix = "nd"
			case 3:
				suffix = "rd"
			}
		}
		s += suffix
	}
	if err := builtinOutput(c, len(s)); err != nil {
		return nil, err
	}
	return s, nil
}
func parseInteger(c *CallContext, s, p string) (Value, error) {
	for range s {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
	}
	pic, _ := integerPicture(p)
	if pic == "w" || pic == "W" || pic == "Ww" {
		return integerWordsParse(s), nil
	}
	if pic == "A" || pic == "a" {
		n := float64(0)
		for _, r := range strings.ToLower(s) {
			if r < 'a' || r > 'z' {
				return Undefined, nil
			}
			n = n*26 + float64(r-'a'+1)
		}
		return n, nil
	}
	if pic == "I" || pic == "i" {
		vals := map[rune]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
		n, prev := 0, 0
		for _, r := range strings.ToUpper(s) {
			v := vals[r]
			if v == 0 {
				return Undefined, nil
			}
			n += v
			if v > prev {
				n -= 2 * prev
			}
			prev = v
		}
		return float64(n), nil
	}
	zero := rune(0)
	for _, r := range pic {
		if unicode.IsDigit(r) {
			zero = r
			for zero > 0 && unicode.IsDigit(zero-1) {
				zero--
			}
			break
		}
	}
	if zero == 0 {
		return nil, builtinError("D3130", "parseInteger")
	}
	var b strings.Builder
	for _, r := range s {
		if r >= zero && r < zero+10 {
			b.WriteRune('0' + r - zero)
		}
	}
	if b.Len() == 0 {
		return Undefined, nil
	}
	n, err := strconv.ParseFloat(b.String(), 64)
	if err != nil {
		return Undefined, nil
	}
	return n, nil
}

var integerFew = []string{"Zero", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
var integerOrd = []string{"Zeroth", "First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh", "Eighth", "Ninth", "Tenth", "Eleventh", "Twelfth", "Thirteenth", "Fourteenth", "Fifteenth", "Sixteenth", "Seventeenth", "Eighteenth", "Nineteenth"}
var integerTens = []string{"Twenty", "Thirty", "Forty", "Fifty", "Sixty", "Seventy", "Eighty", "Ninety"}
var integerMags = []string{"Thousand", "Million", "Billion", "Trillion"}

func integerWords(n float64, prev, ordinal bool) string {
	prefix := ""
	if prev {
		prefix = " and "
	}
	if n <= 19 {
		if ordinal {
			return prefix + integerOrd[floatToInt(n)]
		}
		return prefix + integerFew[floatToInt(n)]
	}
	if n < 100 {
		t := int(n / 10)
		r := math.Mod(n, 10)
		s := prefix + integerTens[t-2]
		if r > 0 {
			return s + "-" + integerWords(r, false, ordinal)
		}
		if ordinal {
			return s[:len(s)-1] + "ieth"
		}
		return s
	}
	if prev {
		prefix = ", "
	} else {
		prefix = ""
	}
	if n < 1000 {
		h := int(n / 100)
		r := math.Mod(n, 100)
		s := prefix + integerFew[h] + " Hundred"
		if r > 0 {
			return s + integerWords(r, true, ordinal)
		}
		if ordinal {
			return s + "th"
		}
		return s
	}
	mag := int(math.Floor(math.Log10(n) / 3))
	if mag > 4 {
		mag = 4
	}
	factor := math.Pow(10, float64(mag*3))
	mant := math.Floor(n / factor)
	product := math.Float64frombits(math.Float64bits(mant * factor))
	r := n - product
	s := prefix + integerWords(mant, false, false) + " " + integerMags[mag-1]
	if r > 0 {
		return s + integerWords(r, true, ordinal)
	}
	if ordinal {
		return s + "th"
	}
	return s
}
func integerWordsParse(s string) Value {
	values := map[string]float64{}
	for i, w := range integerFew {
		values[strings.ToLower(w)] = float64(i)
	}
	for i, w := range integerOrd {
		values[strings.ToLower(w)] = float64(i)
	}
	for i, w := range integerTens {
		w = strings.ToLower(w)
		values[w] = float64((i + 2) * 10)
		values[w[:len(w)-1]+"ieth"] = float64((i + 2) * 10)
	}
	values["hundred"] = 100
	values["hundredth"] = 100
	for i, w := range integerMags {
		w = strings.ToLower(w)
		values[w] = math.Pow(1000, float64(i+1))
		values[w+"th"] = values[w]
	}
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ' ' || r == ',' || r == '-' || r == '\\' })
	segs := []float64{0}
	for _, w := range parts {
		if w == "and" {
			continue
		}
		v, ok := values[w]
		if !ok {
			return Undefined
		}
		last := len(segs) - 1
		if v < 100 {
			if segs[last] >= 1000 {
				segs = append(segs, 0)
				last++
			}
			segs[last] += v
		} else {
			segs[last] *= v
		}
	}
	sum := float64(0)
	for _, v := range segs {
		sum += v
	}
	return sum
}
