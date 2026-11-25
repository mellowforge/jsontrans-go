// Portions derived from JSONata 2.2.2, Copyright IBM Corp. 2016, 2018. MIT license; see conformance/testdata/jsonata/LICENSE.
package jsonata

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
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
	picture, err := numberPictureCodeUnits(c, picture)
	if err != nil {
		return nil, err
	}
	props := map[string]string{"decimal-separator": ".", "grouping-separator": ",", "exponent-separator": "e", "minus-sign": "-", "percent": "%", "per-mille": "‰", "zero-digit": "0", "digit": "#", "pattern-separator": ";"}
	for k, v := range opts {
		if _, used := props[k]; !used {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			text, err := numberPictureCodeUnits(c, s)
			if err != nil {
				return nil, err
			}
			props[k] = text
		}
	}
	parts := strings.SplitN(picture, props["pattern-separator"], 3)
	if len(parts) > 2 {
		return nil, builtinError("D3080", "formatNumber")
	}
	// The reference validates both subpictures before selecting the sign.
	negative := !(n >= 0)
	chosen := 0
	if negative && len(parts) == 2 {
		chosen = 1
	}
	zeroText := props["zero-digit"]
	zero, _ := regexDecodeRune(zeroText)
	digitText := props["digit"]
	digit := singleNumberPictureUnit(digitText)
	decimalText := props["decimal-separator"]
	groupingText := props["grouping-separator"]
	exponentText := props["exponent-separator"]
	// String.fromCharCode wraps the digit family at U+FFFF.
	required := func(x rune) bool { return uint16(x-zero) < 10 }
	localize := func(ch rune) rune {
		if ch >= '0' && ch <= '9' {
			return rune(uint16(zero + ch - '0'))
		}
		return ch
	}
	var pic numberSubpicture
	for i, text := range parts {
		subpicture, err := parseNumberSubpicture(text, props, required)
		if err != nil {
			return nil, err
		}
		if i == chosen {
			pic = subpicture
		}
	}
	p := parts[chosen]
	ip, fp, exponent := pic.integer, pic.fraction, pic.hasExponent
	ipRunes, fpRunes := numberPictureRunes(ip), numberPictureRunes(fp)
	n = math.Abs(n)
	if strings.Contains(p, props["percent"]) {
		n *= 100
	}
	if strings.Contains(p, props["per-mille"]) {
		n *= 1000
	}
	minInt, minFrac, maxFrac := 0, 0, 0
	for _, x := range ipRunes {
		if required(x) {
			minInt++
		}
	}
	scaling := minInt
	for _, x := range fpRunes {
		if required(x) {
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
	if exponent && minInt == 0 && strings.Contains(ip, digitText) {
		minInt = 1
	}
	if minInt == 0 && minFrac == 0 {
		minFrac = 1
	}
	if exponent && (math.IsNaN(n) || math.IsInf(n, 0)) {
		// The reference loops without end scaling a non-finite mantissa.
		return nil, errors.New("formatNumber: number out of range")
	}
	ex := 0
	if exponent && n != 0 {
		// Scale by powers of ten one step at a time. The upper bound is
		// inclusive and each binary64 operation contributes to the result.
		lower, upper := numberPicturePower(scaling-1), numberPicturePower(scaling)
		for n < lower {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			n *= 10
			ex--
		}
		for n > upper {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			n /= 10
			ex++
		}
	}
	if maxFrac > 100 {
		return nil, builtinError("D3085", "formatNumber")
	}
	switch {
	case maxFrac == 0:
		n = math.RoundToEven(n)
	case math.IsInf(n, 0):
		// The reference's $round shifts through Infinity.toString(): NaN.
		n = math.NaN()
	default:
		n = builtinRound(n, maxFrac)
	}
	s := formatNumberPictureDigits(n, maxFrac)
	pieces := strings.SplitN(s, ".", 2)
	integer := pieces[0]
	fraction := ""
	if len(pieces) > 1 {
		fraction = pieces[1]
	}
	for len(fraction) > minFrac && strings.HasSuffix(fraction, "0") {
		fraction = fraction[:len(fraction)-1]
	}
	for len(fraction) < minFrac {
		fraction += "0"
	}
	if err := builtinCollection(c, minInt); err != nil {
		return nil, err
	}
	padInteger := max(0, minInt-len(integer))
	if singleNumberPictureUnit(zeroText) >= 0 && padInteger > 0 {
		integer = strings.Repeat("0", padInteger) + integer
		padInteger = 0
	}
	if minInt == 0 && integer == "0" && singleNumberPictureUnit(zeroText) >= 0 {
		integer = ""
	}
	if err := builtinCollection(c, len(integer)+len(fraction)); err != nil {
		return nil, err
	}
	placeholder := func(unit rune) bool { return unit == digit || required(unit) }
	groupingUnits := numberPictureRunes(groupingText)
	var positions []int
	count := 0
	for i := len(ipRunes) - 1; i >= 0; i-- {
		if placeholder(ipRunes[i]) {
			count++
		}
		if ipRunes[i] == groupingUnits[0] && len(ipRunes)-i >= len(groupingUnits) {
			if err := chargeNumberPictureUnits(c, len(groupingUnits)); err != nil {
				return nil, err
			}
			if slices.Equal(ipRunes[i:i+len(groupingUnits)], groupingUnits) {
				positions = append(positions, count)
			}
		}
	}
	step := 0
	if len(positions) > 0 {
		step = positions[0]
		for i, x := range positions {
			if x != step*(i+1) {
				step = 0
				break
			}
		}
	}

	result := numberPictureOutput{bounded: boundedStringBuilder{c: c}}
	if negative && len(parts) == 1 {
		result.writeString(props["minus-sign"])
	}
	result.writeString(pic.prefix)
	hasDecimal := strings.Contains(p, decimalText)
	fractionalAt := strings.Index(fp, groupingText)
	if len(positions) > 0 && step == 0 || fractionalAt >= 0 {
		pad := numberPictureRunes(zeroText)
		decimal := numberPictureRunes(decimalText)
		nonPadding := len(integer) + len(decimal) + len(fraction)
		if padInteger != 0 && len(pad) > (math.MaxInt-nonPadding)/padInteger {
			return nil, budget.ErrLimit
		}
		size := nonPadding + padInteger*len(pad)
		if err := checkNumberGroupingSize(c, size); err != nil {
			return nil, err
		}
		number := make([]rune, 0, size)
		for i := 0; i < padInteger; i++ {
			if err := chargeNumberPictureUnits(c, len(pad)); err != nil {
				return nil, err
			}
			number = append(number, pad...)
		}
		for _, unit := range integer {
			number = append(number, localize(unit))
		}
		number = append(number, decimal...)
		for _, unit := range fraction {
			number = append(number, localize(unit))
		}
		number, err = groupNumberPicture(c, number, decimal, groupingUnits, positions, step,
			ip, fp, groupingText, fractionalAt, hasDecimal, placeholder)
		if err != nil {
			return nil, err
		}
		for _, unit := range number {
			result.writeRune(unit)
		}
	} else {
		for i := 0; i < padInteger && result.bounded.err == nil; i++ {
			result.writeString(zeroText)
		}
		for i, x := range integer {
			remaining := len(integer) - i
			if i > 0 && step > 0 && remaining%step == 0 {
				result.writeString(groupingText)
			}
			result.writeRune(localize(x))
		}
		if len(fraction) > 0 {
			result.writeString(decimalText)
			if !hasDecimal {
				fraction = fraction[:len(fraction)-1]
			}
			for _, ch := range fraction {
				result.writeRune(localize(ch))
			}
		} else if singleNumberPictureUnit(decimalText) < 0 {
			// A multi-unit decimal separator cannot be the last single unit of
			// the result. The reference removes only the last unit when unused.
			decimalOutput := decimalText
			if !hasDecimal {
				decimalOutput = decimalText[:lastNumberPictureUnit(decimalText)]
			}
			result.writeString(decimalOutput)
		}
	}
	if exponent {
		es := strconv.Itoa(int(math.Abs(float64(ex))))
		width := numberPictureUnitCount(pic.exponent)
		if err := builtinCollection(c, width); err != nil {
			return nil, err
		}
		result.writeString(exponentText)
		if ex < 0 {
			result.writeString(props["minus-sign"])
		}
		for i := len(es); i < width; i++ {
			result.writeString(zeroText)
		}
		for _, x := range es {
			result.writeRune(localize(x))
		}
	}
	result.writeString(pic.suffix)
	return result.finish()
}

// formatNumberPictureDigits follows Math.abs(n).toFixed(precision) after
// decimal half-even rounding. Callers supply a magnitude and at most 100 digits.
func formatNumberPictureDigits(n float64, precision int) string {
	if math.IsNaN(n) || math.IsInf(n, 0) || n >= 1e21 {
		return jsNumberString(n)
	}

	var buffer [128]byte
	digits := strconv.AppendFloat(buffer[:0], n, 'f', precision, 64)
	binary := math.Float64bits(n)
	biasedExponent := int((binary >> 52) & 0x7ff)
	if biasedExponent == 0 {
		// Subnormals cannot reach an exact midpoint at this precision.
		return string(digits)
	}
	const implicitBit = 1 << 52
	significand := (binary & (implicitBit - 1)) | implicitBit
	trailing := bits.TrailingZeros64(significand)

	// toFixed rounds exact ties up, while strconv uses ties to even.
	// At a tie the reduced denominator after decimal scaling is 2.
	// Because 5^precision is 1 modulo 4, the odd significand modulo 4
	// tells us whether the lower integer (the value chosen by Go) is even.
	if biasedExponent-1075+trailing+precision == -1 && (significand>>trailing)%4 == 1 {
		digits[len(digits)-1]++
	}
	return string(digits)
}

type numberSubpicture struct {
	prefix, suffix    string
	integer, fraction string
	exponent          string
	hasExponent       bool
}

func parseNumberSubpicture(text string, props map[string]string, required func(rune) bool) (numberSubpicture, error) {
	digitText, decimalText := props["digit"], props["decimal-separator"]
	groupingText, exponentText := props["grouping-separator"], props["exponent-separator"]
	digit := singleNumberPictureUnit(digitText)
	decimal := singleNumberPictureUnit(decimalText)
	group := singleNumberPictureUnit(groupingText)
	exp := singleNumberPictureUnit(exponentText)
	separator := singleNumberPictureUnit(props["pattern-separator"])
	active := func(x rune) bool {
		return required(x) || x == digit || x == decimal || x == group || x == exp || x == separator
	}
	first, end := -1, len(text)
	for i := 0; i < len(text); {
		x, width := regexDecodeRune(text[i:])
		if x != exp && active(x) {
			if first < 0 {
				first = i
			}
			end = i + width
		}
		i += width
	}
	first = max(first, 0)
	part := numberSubpicture{prefix: text[:first], suffix: text[end:]}
	body := text[first:end]
	mantissa := body
	if at := strings.Index(text[first:], exponentText); at >= 0 && first+at <= end {
		// The exponent index is measured from the start of the full
		// subpicture, then applied to the active part. This is observable
		// when the picture has a passive prefix.
		units := numberPictureUnitCount(text[:first+at])
		mantissa = body[:numberPictureByteOffset(body, units)]
		part.exponent = body[numberPictureByteOffset(body, units+1):]
		part.hasExponent = true
	}
	var hasDecimal bool
	part.integer, part.fraction, hasDecimal = splitNumberPictureSymbol(mantissa, decimalText)
	if !hasDecimal {
		part.fraction = part.suffix
	}

	containsDigit := func(s string, optional bool) bool {
		for i := 0; i < len(s); {
			unit, width := regexDecodeRune(s[i:])
			if required(unit) || optional && unit == digit {
				return true
			}
			i += width
		}
		return false
	}
	var code string
	if strings.Index(text, decimalText) != strings.LastIndex(text, decimalText) {
		code = "D3081"
	}
	if strings.Index(text, props["percent"]) != strings.LastIndex(text, props["percent"]) {
		code = "D3082"
	}
	if strings.Index(text, props["per-mille"]) != strings.LastIndex(text, props["per-mille"]) {
		code = "D3083"
	}
	adjusted := strings.Contains(text, props["percent"]) || strings.Contains(text, props["per-mille"])
	if strings.Contains(text, props["percent"]) && strings.Contains(text, props["per-mille"]) {
		code = "D3084"
	}
	if !containsDigit(mantissa, true) {
		code = "D3085"
	}
	for i := 0; i < len(body); {
		unit, width := regexDecodeRune(body[i:])
		if !active(unit) {
			code = "D3086"
			break
		}
		i += width
	}
	if at := strings.Index(text, decimalText); at >= 0 && group >= 0 {
		_, width := regexDecodeRune(text[at:])
		before, after := false, false
		if at > 0 {
			before = text[lastNumberPictureUnit(text[:at]):at] == groupingText
		}
		if next := at + width; next < len(text) {
			unit, _ := regexDecodeRune(text[next:])
			after = unit == group
		}
		if before || after {
			code = "D3087"
		}
	} else if at < 0 && group >= 0 && strings.HasSuffix(part.integer, groupingText) {
		code = "D3088"
	}
	if strings.Contains(text, groupingText+groupingText) {
		code = "D3089"
	}
	if at := strings.Index(part.integer, digitText); at >= 0 && containsDigit(part.integer[:at], false) {
		code = "D3090"
	}
	if at := strings.LastIndex(part.fraction, digitText); at >= 0 && containsDigit(part.fraction[at:], false) {
		code = "D3091"
	}
	if part.hasExponent {
		if adjusted && part.exponent != "" {
			code = "D3092"
		}
		if part.exponent == "" {
			code = "D3093"
		}
		for i := 0; i < len(part.exponent); {
			unit, width := regexDecodeRune(part.exponent[i:])
			if !required(unit) {
				code = "D3093"
				break
			}
			i += width
		}
	}
	if code != "" {
		return numberSubpicture{}, builtinError(code, "formatNumber")
	}
	return part, nil
}

func groupNumberPicture(
	c *CallContext, number, decimal, grouping []rune, positions []int, step int,
	integer, fraction, groupText string, fractionalAt int, hasDecimal bool, placeholder func(rune) bool,
) ([]rune, error) {
	decimalAt, err := numberPictureSymbolIndex(c, number, decimal)
	if err != nil {
		return nil, err
	}
	number, err = groupNumberInteger(c, number, decimalAt, grouping, positions, step)
	if err != nil {
		return nil, err
	}
	decimalAt, err = numberPictureSymbolIndex(c, number, decimal)
	if err != nil {
		return nil, err
	}
	if fractionalAt >= 0 {
		number, err = groupNumberFraction(c, number, decimalAt, integer, fraction, groupText, grouping, fractionalAt, placeholder)
		if err != nil {
			return nil, err
		}
	}
	decimalAt, err = numberPictureSymbolIndex(c, number, decimal)
	if err != nil {
		return nil, err
	}
	if (!hasDecimal || decimalAt == len(number)-1) && len(number) > 0 {
		number = number[:len(number)-1]
	}
	return number, nil
}

func groupNumberInteger(
	c *CallContext, number []rune, decimalAt int, grouping []rune, positions []int, step int,
) ([]rune, error) {
	if len(positions) == 0 {
		return number, nil
	}
	if step == 0 && (len(grouping) != 1 || positions[len(positions)-1] > decimalAt) {
		// Irregular offsets are applied left to right. The reference advances
		// the decimal index by one even for multi-unit separators; a negative
		// slice index is relative to the end of the current number.
		for i := len(positions) - 1; i >= 0; i-- {
			var err error
			number, err = insertNumberGrouping(c, number, decimalAt-positions[i], grouping)
			if err != nil {
				return nil, err
			}
			decimalAt++
		}
		return number, nil
	}

	// Regular insertions and aligned single-unit irregular insertions can
	// be copied in one pass. This keeps long pictures linear.
	count := len(positions)
	if step > 0 {
		count = max(0, (decimalAt-1)/step)
	}
	if count == 0 {
		return number, nil
	}
	if len(grouping) > (math.MaxInt-len(number))/count {
		return nil, budget.ErrLimit
	}
	size := len(number) + count*len(grouping)
	if err := checkNumberGroupingSize(c, size); err != nil {
		return nil, err
	}
	if err := chargeNumberPictureUnits(c, size); err != nil {
		return nil, err
	}
	output := make([]rune, 0, size)
	from := 0
	for i := count - 1; i >= 0; i-- {
		distance := 0
		if step > 0 {
			distance = (i + 1) * step
		} else {
			distance = positions[i]
		}
		at := decimalAt - distance
		output = append(output, number[from:at]...)
		output = append(output, grouping...)
		from = at
	}
	return append(output, number[from:]...), nil
}

func groupNumberFraction(
	c *CallContext, number []rune, decimalAt int, integer, picture, groupText string,
	grouping []rune, groupingAt int, placeholder func(rune) bool,
) ([]rune, error) {
	at := numberPictureUnitCount(picture[:groupingAt])
	pictureUnits := numberPictureRunes(picture)
	digits, scanned := 0, 0
	from := numberPictureByteOffset(integer, at+1)
	// The first separator is found in the fractional picture. Subsequent
	// searches continue from that index in the integer picture.
	for {
		for i := scanned; i < min(at, len(pictureUnits)); i++ {
			if placeholder(pictureUnits[i]) {
				digits++
			}
		}
		scanned = min(at, len(pictureUnits))
		var err error
		number, err = insertNumberGrouping(c, number, decimalAt+1+digits, grouping)
		if err != nil {
			return nil, err
		}
		offset := strings.Index(integer[from:], groupText)
		if offset < 0 {
			return number, nil
		}
		found := from + offset
		at += 1 + numberPictureUnitCount(integer[from:found])
		_, width := regexDecodeRune(integer[found:])
		from = found + width
	}
}

func numberPictureSymbolIndex(c *CallContext, text, symbol []rune) (int, error) {
	if len(symbol) == 1 {
		return slices.Index(text, symbol[0]), nil
	}
	for i := 0; i <= len(text)-len(symbol); i++ {
		if text[i] != symbol[0] {
			continue
		}
		if err := chargeNumberPictureUnits(c, len(symbol)); err != nil {
			return -1, err
		}
		if slices.Equal(text[i:i+len(symbol)], symbol) {
			return i, nil
		}
	}
	return -1, nil
}

func insertNumberGrouping(c *CallContext, text []rune, at int, symbol []rune) ([]rune, error) {
	if at < 0 {
		at += len(text)
	}
	at = min(max(at, 0), len(text))
	if len(symbol) > math.MaxInt-len(text) {
		return nil, budget.ErrLimit
	}
	size := len(text) + len(symbol)
	if err := checkNumberGroupingSize(c, size); err != nil {
		return nil, err
	}
	// Charge the part moved by the insertion and any reallocation.
	work := size - at
	if size > cap(text) {
		work = size
	}
	if err := chargeNumberPictureUnits(c, work); err != nil {
		return nil, err
	}
	return slices.Insert(text, at, symbol...), nil
}

func chargeNumberPictureUnits(c *CallContext, units int) error {
	for units > 0 {
		if err := builtinStep(c); err != nil {
			return err
		}
		units -= 64
	}
	return nil
}

func checkNumberGroupingSize(c *CallContext, size int) error {
	if err := builtinCollection(c, size); err != nil {
		return err
	}
	// The trailing unit can be trimmed. All remaining code units use at
	// least one byte, including when two surrogates form a pair.
	return builtinOutput(c, size-1)
}

func numberPictureUnitCount(text string) int {
	units := 0
	for len(text) > 0 {
		_, width := regexDecodeRune(text)
		text = text[width:]
		units++
	}
	return units
}

func numberPictureByteOffset(text string, units int) int {
	offset := 0
	for units > 0 && offset < len(text) {
		_, width := regexDecodeRune(text[offset:])
		offset += width
		units--
	}
	return offset
}

// Active characters are compared with charAt in the reference. A locale
// override with more than one code unit cannot match one active character.
func singleNumberPictureUnit(symbol string) rune {
	unit, size := regexDecodeRune(symbol)
	if size != len(symbol) {
		return -1
	}
	return unit
}

func lastNumberPictureUnit(text string) int {
	i := len(text) - 1
	for i > 0 && !utf8.RuneStart(text[i]) {
		i--
	}
	return i
}

func splitNumberPictureSymbol(text, symbol string) (string, string, bool) {
	i := strings.Index(text, symbol)
	if i < 0 {
		return text, "", false
	}
	// JSONata adds one UTF-16 unit to the symbol's first index.
	_, width := regexDecodeRune(text[i:])
	return text[:i], text[i+width:], true
}

// Number-picture syntax uses UTF-16 indexing. Preserve lone surrogates as
// WTF-8 and expand supplementary scalars into encoded pairs for analysis.
func numberPictureCodeUnits(c *CallContext, s string) (string, error) {
	var expanded strings.Builder
	start := 0
	for i := 0; i < len(s); {
		if err := builtinStep(c); err != nil {
			return "", err
		}
		unit, size := regexDecodeRune(s[i:])
		if unit == utf8.RuneError && size == 1 {
			return "", fmt.Errorf("%w: invalid string encoding", jsontrans.ErrInvalidInput)
		}
		if unit > 0xffff {
			high, low := utf16.EncodeRune(unit)
			expanded.WriteString(s[start:i])
			expanded.WriteString(numberPictureUnit(high))
			expanded.WriteString(numberPictureUnit(low))
			start = i + size
		}
		i += size
	}
	if start == 0 {
		return s, nil
	}
	expanded.WriteString(s[start:])
	return expanded.String(), nil
}

func numberPictureRunes(s string) []rune {
	if utf8.ValidString(s) {
		return []rune(s)
	}
	var units []rune
	for len(s) > 0 {
		r, size := regexDecodeRune(s)
		units = append(units, r)
		s = s[size:]
	}
	return units
}

func numberPictureUnit(unit rune) string {
	if 0xd800 <= unit && unit <= 0xdfff {
		return string([]byte{byte(0xe0 | unit>>12), byte(0x80 | (unit>>6)&63), byte(0x80 | unit&63)})
	}
	return string(unit)
}

// Hold one high unit so pairs assembled from separate picture fragments
// count as four UTF-8 bytes in the bounded result.
type numberPictureOutput struct {
	bounded boundedStringBuilder
	high    rune
}

func (b *numberPictureOutput) writeString(text string) {
	if utf8.ValidString(text) {
		if text != "" {
			b.flushHigh()
			b.bounded.WriteString(text)
		}
		return
	}
	for len(text) > 0 && b.bounded.err == nil {
		unit, size := regexDecodeRune(text)
		b.writeRune(unit)
		text = text[size:]
	}
}

func (b *numberPictureOutput) writeRune(unit rune) {
	if b.high != 0 && 0xdc00 <= unit && unit <= 0xdfff {
		b.bounded.WriteRune(utf16.DecodeRune(b.high, unit))
		b.high = 0
		return
	}
	b.flushHigh()
	if 0xd800 <= unit && unit <= 0xdbff {
		b.high = unit
	} else {
		b.bounded.WriteString(numberPictureUnit(unit))
	}
}

func (b *numberPictureOutput) flushHigh() {
	if b.high != 0 {
		b.bounded.WriteString(numberPictureUnit(b.high))
		b.high = 0
	}
}

func (b *numberPictureOutput) finish() (Value, error) {
	b.flushHigh()
	if b.bounded.err != nil {
		return nil, b.bounded.err
	}
	return b.bounded.String(), nil
}
