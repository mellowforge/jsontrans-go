// Portions derived from JSONata 2.2.2, Copyright IBM Corp. 2016, 2018. MIT license; see conformance/testdata/jsonata/LICENSE.
package jsonata

import (
	"errors"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
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
	if i := strings.LastIndexByte(p, ';'); i >= 0 {
		return p[:i], strings.HasPrefix(p[i+1:], "o")
	}
	return p, false
}

type integerGrouping struct {
	separator rune
	following int
}

type decimalIntegerPicture struct {
	zero     rune
	required int
	grouping []integerGrouping
}

// The JSONata 2.2.2 picture grammar enumerates decimal families. Characters
// from later Unicode digit families are literal grouping separators.
var integerDecimalFamilies = [...]rune{
	0x0030, 0x0660, 0x06f0, 0x07c0, 0x0966, 0x09e6, 0x0a66, 0x0ae6,
	0x0b66, 0x0be6, 0x0c66, 0x0ce6, 0x0d66, 0x0de6, 0x0e50, 0x0ed0,
	0x0f20, 0x1040, 0x1090, 0x17e0, 0x1810, 0x1946, 0x19d0, 0x1a80,
	0x1a90, 0x1b50, 0x1bb0, 0x1c40, 0x1c50, 0xa620, 0xa8d0, 0xa900,
	0xa9d0, 0xa9f0, 0xaa50, 0xabf0, 0xff10,
}

func integerDecimalZero(r rune) rune {
	if r < 0x0660 {
		if r >= '0' && r <= '9' {
			return '0'
		}
		return 0
	}
	i := sort.Search(len(integerDecimalFamilies), func(i int) bool {
		return integerDecimalFamilies[i]+9 >= r
	})
	if i < len(integerDecimalFamilies) && integerDecimalFamilies[i] <= r {
		return integerDecimalFamilies[i]
	}
	return 0
}

func readDecimalIntegerPicture(c *CallContext, picture, name string) (decimalIntegerPicture, error) {
	var result decimalIntegerPicture
	width := 0
	for len(picture) > 0 {
		if err := builtinStep(c); err != nil {
			return result, err
		}
		r, size := regexDecodeRune(picture)
		picture = picture[size:]
		z := integerDecimalZero(r)
		switch {
		case r == '#':
			width++
		case z != 0:
			if result.zero != 0 && result.zero != z {
				return result, builtinError("D3131", name)
			}
			result.zero = z
			result.required++
			width++
		default:
			if err := builtinCollection(c, len(result.grouping)+1); err != nil {
				return result, err
			}
			result.grouping = append(result.grouping, integerGrouping{r, width})
		}
	}
	for i := range result.grouping {
		result.grouping[i].following = width - result.grouping[i].following
	}
	return result, nil
}

func (p decimalIntegerPicture) regularGrouping() (integerGrouping, bool) {
	if len(p.grouping) == 0 {
		return integerGrouping{}, false
	}
	last := p.grouping[len(p.grouping)-1]
	step := last.following
	if step == 0 {
		return integerGrouping{}, false
	}
	for i, group := range p.grouping {
		if group.separator != last.separator ||
			group.following/step != len(p.grouping)-i || group.following%step != 0 {
			return integerGrouping{}, false
		}
	}
	return last, true
}

func integerTimezoneGrouping(c *CallContext, picture, name string) (string, error) {
	if picture == "" {
		return "", nil
	}
	picture, _ = integerPicture(strings.TrimSuffix(picture, "t"))
	format, err := readDecimalIntegerPicture(c, picture, name)
	if err != nil {
		return "", err
	}
	group, regular := format.regularGrouping()
	if !regular || format.zero == 0 {
		return "", nil
	}
	return regexUnitsString([]rune{group.separator}), nil
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
	format, err := readDecimalIntegerPicture(c, pic, "formatInteger")
	if err != nil {
		return nil, err
	}
	if format.zero == 0 {
		return nil, builtinError("D3130", "formatInteger")
	}
	return format.format(c, jsNumberString(n), ordinal)
}

func integerUnits(r rune, buffer *[2]rune) []rune {
	if r > 0xffff {
		buffer[0], buffer[1] = utf16.EncodeRune(r)
		return buffer[:2]
	}
	buffer[0] = r
	return buffer[:1]
}

func integerOrdinal(last, previous rune) string {
	if previous != '1' {
		switch last {
		case '1':
			return "st"
		case '2':
			return "nd"
		case '3':
			return "rd"
		}
	}
	return "th"
}

func (p decimalIntegerPicture) format(c *CallContext, digits string, ordinal bool) (Value, error) {
	n := max(len(digits), p.required)
	if err := builtinCollection(c, n); err != nil {
		return nil, err
	}
	if err := builtinOutput(c, n); err != nil {
		return nil, err
	}
	padding := n - len(digits)
	if p.zero == '0' && len(p.grouping) == 0 {
		if ordinal {
			previous := rune(0)
			if len(digits) >= 2 {
				previous = rune(digits[len(digits)-2])
			}
			digits += integerOrdinal(rune(digits[len(digits)-1]), previous)
		}
		if err := builtinOutput(c, padding+len(digits)); err != nil {
			return nil, err
		}
		return strings.Repeat("0", padding) + digits, nil
	}
	units := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		digit := p.zero
		if i >= padding {
			// The reference offsets every code point in the decimal text,
			// including the exponent marker and sign in scientific notation.
			digit = rune(digits[i-padding]) + p.zero - '0'
		}
		units = append(units, integerUnits(digit, &[2]rune{})...)
	}
	if group, ok := p.regularGrouping(); ok {
		separator := integerUnits(group.separator, &[2]rune{})
		size := len(units) + (len(units)-1)/group.following*len(separator)
		if err := builtinCollection(c, size); err != nil {
			return nil, err
		}
		if err := builtinOutput(c, size); err != nil {
			return nil, err
		}
		result := make([]rune, 0, size)
		for i, unit := range units {
			if i%64 == 0 {
				if err := builtinStep(c); err != nil {
					return nil, err
				}
			}
			if i > 0 && (len(units)-i)%group.following == 0 {
				result = append(result, separator...)
			}
			result = append(result, unit)
		}
		units = result
	} else {
		for _, group := range p.grouping {
			at := len(units) - group.following
			if at < 0 {
				// JavaScript's substr keeps the suffix starting that many
				// units from the end when a separator lies beyond the digits.
				units = units[max(0, len(units)+at):]
				at = 0
			}
			separator := integerUnits(group.separator, &[2]rune{})
			length := len(units) + len(separator)
			if err := builtinCollection(c, length); err != nil {
				return nil, err
			}
			if err := builtinOutput(c, length); err != nil {
				return nil, err
			}
			for i := at; i < len(units); i += 64 {
				if err := builtinStep(c); err != nil {
					return nil, err
				}
			}
			units = slices.Insert(units, at, separator...)
		}
	}
	if ordinal {
		previous := rune(0)
		if len(units) >= 2 {
			previous = units[len(units)-2]
		}
		for _, r := range integerOrdinal(units[len(units)-1], previous) {
			units = append(units, r)
		}
	}
	s := regexUnitsString(units)
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
	pic, ordinal := integerPicture(p)
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
	if !utf8.ValidString(s) || !utf8.ValidString(pic) {
		return parseDecimalIntegerPicture(c, s, pic, ordinal)
	}
	zero := rune(0)
	for _, r := range pic {
		if zero = integerDecimalZero(r); zero != 0 {
			break
		}
	}
	if zero == 0 {
		return nil, builtinError("D3130", "parseInteger")
	}
	if zero == '0' && strings.ContainsAny(s, "xX") {
		// The reference infers hexadecimal from the prefix after removing
		// picture separators. Digit filtering would lose that prefix.
		return parseDecimalIntegerPicture(c, s, pic, ordinal)
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
	if errors.Is(err, strconv.ErrRange) {
		// Re-read the signed integer prefix with picture semantics. The
		// accumulated digits can overflow even when the prefix is finite.
		return parseDecimalIntegerPicture(c, s, pic, ordinal)
	}
	if err != nil {
		return Undefined, nil
	}
	return n, nil
}

func parseDecimalIntegerPicture(c *CallContext, text, picture string, ordinal bool) (Value, error) {
	format, err := readDecimalIntegerPicture(c, picture, "parseInteger")
	if err != nil {
		return nil, err
	}
	if format.zero == 0 {
		return nil, builtinError("D3130", "parseInteger")
	}
	return format.parseUTF16(c, text, ordinal)
}

func (p decimalIntegerPicture) parseUTF16(c *CallContext, text string, ordinal bool) (Value, error) {
	units, err := regexInputUnits(c, text)
	if err != nil {
		return nil, err
	}
	if ordinal {
		units = units[:max(0, len(units)-2)]
	}

	remove := func(separator []rune) error {
		// Compact in place, with exact UTF-16 matching for each separator.
		out := units[:0]
		for i, nextCheck := 0, 0; i < len(units); {
			if i >= nextCheck {
				if err := builtinStep(c); err != nil {
					return err
				}
				nextCheck = i + 64
			}
			if len(separator) <= len(units)-i && slices.Equal(units[i:i+len(separator)], separator) {
				i += len(separator)
			} else {
				out = append(out, units[i])
				i++
			}
		}
		units = out
		return nil
	}
	if _, regular := p.regularGrouping(); regular {
		// The reference strips ASCII commas for regular groups, even
		// when the picture specifies a different separator.
		if err := remove([]rune{','}); err != nil {
			return nil, err
		}
	} else {
		for i := len(p.grouping) - 1; i >= 0; i-- {
			if err := remove(integerUnits(p.grouping[i].separator, &[2]rune{})); err != nil {
				return nil, err
			}
		}
	}

	for i, unit := range units {
		if i%64 == 0 {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
		}
		r := unit - p.zero + '0'
		if r < 0 || r > utf8.MaxRune {
			return nil, errors.New("parseInteger: invalid code point in decimal picture")
		}
		units[i] = r
	}
	for len(units) > 0 {
		r := units[0]
		if r == 0x85 || !(unicode.IsSpace(r) || r == 0xfeff) {
			break
		}
		units = units[1:]
	}
	negative := false
	if len(units) > 0 && (units[0] == '+' || units[0] == '-') {
		negative = units[0] == '-'
		units = units[1:]
	}
	hex := len(units) > 1 && units[0] == '0' && (units[1] == 'x' || units[1] == 'X')
	if hex {
		units = units[2:]
	}
	var digits strings.Builder
	for i, r := range units {
		if !(r >= '0' && r <= '9' || hex && (r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F')) {
			break
		}
		if i%64 == 0 {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
		}
		digits.WriteByte(byte(r))
	}
	if digits.Len() == 0 {
		return math.NaN(), nil
	}
	prefix := digits.String()
	if hex {
		prefix = "0x" + prefix + "p0"
	}
	n, err := strconv.ParseFloat(prefix, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return math.NaN(), nil
	}
	if negative {
		n = -n
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
