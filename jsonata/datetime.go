// Portions derived from JSONata 2.2.2, Copyright IBM Corp. 2016, 2018. MIT license; see conformance/testdata/jsonata/LICENSE.
package jsonata

import (
	"errors"
	"fmt"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func builtinDateTime(m map[string]Callable) {
	m["toMillis"] = NativeFunction{N: 2, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		s, ok := v.(string)
		if !ok {
			return nil, builtinError("T0410", "toMillis")
		}
		pic := builtinArg(a, 1)
		if IsUndefined(pic) {
			for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700", "2006-01-02", "2006-01", "2006"} {
				if t, e := time.Parse(layout, s); e == nil {
					return float64(t.UnixMilli()), nil
				}
			}
			return nil, builtinError("D3110", "toMillis")
		}
		p, ok := pic.(string)
		if !ok {
			return nil, builtinError("T0410", "toMillis")
		}
		return dateParsePicture(c, s, p)
	}}

	m["fromMillis"] = NativeFunction{N: 3, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		n, ok := v.(float64)
		if !ok {
			return nil, builtinError("T0410", "fromMillis")
		}
		return dateFromMillis(c, n, builtinArg(a, 1), builtinArg(a, 2))
	}}
	m["now"] = NativeFunction{N: 2, Fn: func(c *CallContext, a []Value) (Value, error) {
		return dateFromMillis(c, float64(c.Now.UnixMilli()), builtinArg(a, 0), builtinArg(a, 1))
	}}
}
func dateFromMillis(c *CallContext, n float64, picture, timezone Value) (Value, error) {
	offset := 0
	if z, ok := timezone.(string); ok && z != "" {
		// As the reference: parseInt(timezone), then floor(v/100) hours and
		// v%100 minutes, so "-0530" is -06:30 and "+05:30" is +00:05.
		v, ok := jsParseInt(z)
		if !ok {
			return nil, builtinError("D3134", "fromMillis")
		}
		hours := v / 100
		if v < 0 && v%100 != 0 {
			hours--
		}
		offset = hours*60 + v%100
	}
	// JavaScript dates span ±8.64e15 ms after the offset is applied; beyond
	// that every component of the reference's Date is NaN.
	local := n + float64(offset)*60000
	invalid := math.IsNaN(local) || math.Abs(local) > 8.64e15
	t := time.UnixMilli(floatToInt64(n)).In(time.FixedZone("", offset*60))
	p, ok := picture.(string)
	if !ok && (invalid || t.Year() < 0 || t.Year() > 9999) {
		p, ok = isoDatePicture, true
	}
	if !ok {
		s := t.Format("2006-01-02T15:04:05.000")
		if offset == 0 {
			if err := builtinOutput(c, len(s)+1); err != nil {
				return nil, err
			}
			return s + "Z", nil
		}
		if err := builtinOutput(c, len(s)+6); err != nil {
			return nil, err
		}
		return s + t.Format("-07:00"), nil
	}
	b := boundedStringBuilder{c: c}
	for i := 0; i < len(p); i++ {
		if p[i] == '[' {
			if i+1 < len(p) && p[i+1] == '[' {
				i++
				continue
			}
			j := strings.IndexByte(p[i+1:], ']')
			if j < 0 {
				return nil, builtinError("D3135", "fromMillis")
			}
			i += j + 1
		} else if p[i] == ']' && i+1 < len(p) && p[i+1] == ']' {
			i++
		}
	}
	for i := 0; i < len(p); {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		if p[i] == '[' {
			if i+1 < len(p) && p[i+1] == '[' {
				b.WriteByte('[')
				i += 2
				continue
			}
			j := strings.IndexByte(p[i+1:], ']')
			if j < 0 {
				return nil, builtinError("D3135", "fromMillis")
			}
			j += i + 1
			marker := strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return -1
				}
				return r
			}, p[i+1:j])
			if marker == "" {
				return nil, builtinError("D3132", "fromMillis")
			}
			v, e := dateFormatMarker(c, t, marker, offset, invalid)
			if e != nil {
				return nil, e
			}
			b.WriteString(v)
			i = j + 1
		} else if p[i] == ']' && i+1 < len(p) && p[i+1] == ']' {
			b.WriteByte(']')
			i += 2
		} else {
			b.WriteByte(p[i])
			i++
		}
	}
	if b.err != nil {
		return nil, b.err
	}
	return b.String(), nil
}

// isoDatePicture is the reference's default $fromMillis picture.
const isoDatePicture = "[Y0001]-[M01]-[D01]T[H01]:[m01]:[s01].[f001][Z01:01t]"

// jsParseInt reads a leading signed decimal integer as JavaScript's parseInt
// does, ignoring the rest. It reports false when there are no digits or more
// than nine.
func jsParseInt(s string) (int, bool) {
	s = strings.TrimLeft(s, " \t\n\r")
	sign := 1
	if s != "" && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits > 9 {
		return 0, false
	}
	v, _ := strconv.Atoi(s[:digits])
	return sign * v, true
}

// errInvalidDate marks presentations of an invalid date that the reference
// cannot format either: it fails without a JSONata code.
var errInvalidDate = errors.New("fromMillis: date outside the representable range")

func dateFormatMarker(c *CallContext, t time.Time, marker string, offset int, invalid bool) (string, error) {
	comp := marker[0]
	pic := marker[1:]
	originalPic := pic
	width := ""
	if i := strings.LastIndex(pic, ","); i >= 0 {
		tail := pic[i+1:]
		if strings.ContainsAny(tail, "*-") || !strings.Contains(pic[:i], ",") {
			width = tail
			pic = pic[:i]
		}
	}
	if pic == "" {
		switch comp {
		case 'm', 's':
			pic = "01"
		case 'f':
			pic = "001"
		case 'F', 'M':
			if comp == 'F' {
				pic = "n"
			} else {
				pic = "1"
			}
		case 'P':
			pic = "n"
		case 'Z', 'z':
			pic = "01:01"
		default:
			pic = "1"
		}
	}
	if comp == 'Z' || comp == 'z' {
		if len(strings.ReplaceAll(strings.TrimSuffix(pic, "t"), ":", "")) > 4 {
			return "", builtinError("D3134", "fromMillis")
		}
		if strings.HasSuffix(pic, "t") && offset == 0 {
			return "Z", nil
		}
		sign := "+"
		if offset < 0 {
			sign = "-"
			offset = -offset
		}
		h, min := offset/60, offset%60
		z := ""
		switch strings.TrimSuffix(pic, "t") {
		case "0", "1":
			z = fmt.Sprintf("%s%d", sign, h)
			if min != 0 {
				z += fmt.Sprintf(":%02d", min)
			}
		case "0101", "0001":
			z = fmt.Sprintf("%s%02d%02d", sign, h, min)
		default:
			z = fmt.Sprintf("%s%02d:%02d", sign, h, min)
		}
		if comp == 'z' {
			z = "GMT" + z
		}
		return z, nil
	}
	if invalid {
		return invalidDateMarker(c, comp, pic, width)
	}
	var n int
	switch comp {
	case 'Y':
		n = t.Year()
	case 'X':
		n, _ = t.ISOWeek()
	case 'M':
		n = int(t.Month())
	case 'x':
		_, n = monthWeek(t)
	case 'w':
		n, _ = monthWeek(t)
	case 'D':
		n = t.Day()
	case 'd':
		n = t.YearDay()
	case 'F':
		n = int(t.Weekday())
		if n == 0 {
			n = 7
		}
	case 'W':
		_, n = t.ISOWeek()
	case 'H':
		n = t.Hour()
	case 'h':
		n = t.Hour() % 12
		if n == 0 {
			n = 12
		}
	case 'm':
		n = t.Minute()
	case 's':
		n = t.Second()
	case 'f':
		n = t.Nanosecond() / 1e6
	case 'P':
		s := "am"
		if t.Hour() >= 12 {
			s = "pm"
		}
		if pic == "N" {
			s = strings.ToUpper(s)
		}
		return s, nil
	case 'C', 'E':
		return "ISO", nil
	default:
		return "", builtinError("D3132", "fromMillis")
	}
	if strings.HasPrefix(pic, "N") || pic == "n" {
		var s string
		switch comp {
		case 'M', 'x':
			s = time.Month(n).String()
		case 'F':
			s = t.Weekday().String()
		default:
			return "", builtinError("D3133", "fromMillis")
		}
		if pic == "N" {
			s = strings.ToUpper(s)
		} else if pic == "n" {
			s = strings.ToLower(s)
		}
		if width != "" {
			parts := strings.Split(width, "-")
			if len(parts) == 2 {
				max, e := strconv.Atoi(parts[1])
				if e == nil && len(s) > max {
					s = s[:max]
				}
			}
		}
		return s, nil
	}
	if comp == 'Y' && width == "" {
		// §9.8.4.4 as the reference applies it: a picture with two or more
		// digits keeps that many trailing year digits.
		if digits := strings.Count(pic, "#") + countIntegerDigits(pic); digits >= 2 {
			n %= int(math.Pow10(min(digits, 18)))
		}
	}
	ord := strings.HasSuffix(pic, "o")
	if ord {
		pic = strings.TrimSuffix(pic, "o") + ";o"
	}
	if pic == "" {
		pic = "1"
	}
	if width != "" {
		parts := strings.Split(width, "-")
		min, e := strconv.Atoi(parts[0])
		if e != nil && parts[0] != "" && strings.Trim(parts[0], "0123456789") == "" {
			return "", fmt.Errorf("%w: picture width", budget.ErrLimit)
		}
		if e == nil && min > countIntegerDigits(pic) {
			if err := builtinCollection(c, min); err != nil {
				return "", err
			}
			if err := builtinOutput(c, min); err != nil {
				return "", err
			}
			pic = strings.Repeat("0", min-countIntegerDigits(pic)) + pic
		}
	}
	v, e := formatInteger(c, float64(n), pic)
	if e != nil {
		return "", e
	}
	s := v.(string)
	if width != "" {
		parts := strings.Split(width, "-")
		if len(parts) == 1 && strings.HasPrefix(originalPic, ",") {
			parts = append(parts, parts[0])
		}
		if len(parts) == 2 {
			max, e := strconv.Atoi(parts[1])
			if e == nil && len(s) > max {
				s = s[len(s)-max:]
			}
		}
	}
	return s, nil
}

// invalidDateMarker formats a component of an invalid date as the reference
// does: numbers are NaN, zero-padded to the picture's digits or minimum width.
func invalidDateMarker(c *CallContext, comp byte, pic, width string) (string, error) {
	switch comp {
	case 'P':
		if pic == "N" {
			return "AM", nil
		}
		return "am", nil
	case 'C', 'E':
		return "ISO", nil
	case 'f':
		return "NaN", nil
	case 'Y', 'X', 'M', 'x', 'w', 'D', 'd', 'F', 'W', 'H', 'h', 'm', 's':
	default:
		return "", builtinError("D3132", "fromMillis")
	}
	if pic == "o" {
		return "", builtinError("D3130", "formatInteger")
	}
	if strings.Trim(pic, "0123456789#") != "" {
		return "", errInvalidDate
	}
	pad := countIntegerDigits(pic)
	if width != "" {
		part := strings.Split(width, "-")[0]
		minimum, err := strconv.Atoi(part)
		if err != nil && part != "" && strings.Trim(part, "0123456789") == "" {
			return "", fmt.Errorf("%w: picture width", budget.ErrLimit)
		}
		if err == nil && minimum > pad {
			pad = minimum
		}
	}
	s := "NaN"
	if pad > len(s) {
		// Charge the padding before building it, as for valid dates.
		if err := builtinCollection(c, pad); err != nil {
			return "", err
		}
		if err := builtinOutput(c, pad); err != nil {
			return "", err
		}
		s = strings.Repeat("0", pad-len(s)) + s
	}
	return s, nil
}
func countIntegerDigits(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

func dateParsePicture(c *CallContext, s, p string) (Value, error) {
	var re strings.Builder
	re.WriteString("(?i)^")
	markers := []string{}
	for i := 0; i < len(p); {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		if p[i] == '[' {
			if i+1 < len(p) && p[i+1] == '[' {
				re.WriteString(`\[`)
				i += 2
				continue
			}
			j := strings.IndexByte(p[i+1:], ']')
			if j < 0 {
				return nil, builtinError("D3135", "toMillis")
			}
			j += i + 1
			marker := strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return -1
				}
				return r
			}, p[i+1:j])
			if marker == "" || !strings.ContainsRune("YMDdXxWwFHhmsfPZzCE", rune(marker[0])) {
				return nil, builtinError("D3132", "toMillis")
			}
			if len(marker) > 1 && marker[1] == 'N' && !strings.ContainsRune("MFxP", rune(marker[0])) {
				return nil, builtinError("D3133", "toMillis")
			}
			markers = append(markers, marker)
			pic := marker[1:]
			nextAdjacent := j+1 < len(p) && p[j+1] == '[' && (j+2 >= len(p) || p[j+2] != '[')
			switch {
			case marker[0] == 'Z' || marker[0] == 'z':
				re.WriteString(`(GMT[+-][0-9:]+|[+-][0-9:]+|Z)`)
			case marker[0] == 'P':
				re.WriteString(`(am|pm)`)
			case strings.HasPrefix(pic, "N") || strings.HasPrefix(pic, "n"):
				re.WriteString(`([[:alpha:]]+)`)
			case strings.HasPrefix(pic, "w") || strings.HasPrefix(pic, "W"):
				re.WriteString(`([[:alpha:], -]+?)`)
			case pic == "I" || pic == "i" || pic == "A" || pic == "a":
				re.WriteString(`([[:alpha:]]+)`)
			case nextAdjacent:
				width := countIntegerDigits(pic)
				if comma := strings.Index(pic, ","); comma >= 0 {
					w := strings.Split(pic[comma+1:], "-")
					if len(w) > 1 {
						if x, e := strconv.Atoi(w[1]); e == nil {
							width = x
						}
					}
				}
				if width == 0 {
					width = 1
				}
				re.WriteString(fmt.Sprintf(`([0-9]{%d})`, width))
			default:
				re.WriteString(`([0-9]+(?:st|nd|rd|th)?)`)
			}
			i = j + 1
		} else if p[i] == ']' && i+1 < len(p) && p[i+1] == ']' {
			re.WriteString(`\]`)
			i += 2
		} else {
			re.WriteString(regexp.QuoteMeta(p[i : i+1]))
			i++
		}
	}
	re.WriteString("$")
	hasISO := false
	for _, marker := range markers {
		if strings.ContainsRune("XxWw", rune(marker[0])) {
			hasISO = true
		}
	}
	if hasISO {
		return nil, builtinError("D3136", "toMillis")
	}
	compiled, e := regexp.Compile(re.String())
	if e != nil {
		return nil, e
	}
	matches := compiled.FindStringSubmatch(s)
	if matches == nil {
		return Undefined, nil
	}
	if len(markers) == 0 {
		return Undefined, nil
	}
	now := c.Now.UTC()
	year, month, day := now.Year(), int(now.Month()), now.Day()
	hour, min, sec, millis := 0, 0, 0, 0
	hasY, hasM, hasD, hasDay, hasHour, hasMinute, hasSecond := false, false, false, false, false, false, false
	dayYear := 0
	pm := ""
	offset := 0
	for i, marker := range markers {
		v := matches[i+1]
		comp := marker[0]
		pic := marker[1:]
		if j := strings.Index(pic, ","); j >= 0 {
			pic = pic[:j]
		}
		if pic == "" {
			pic = "1"
		}
		if strings.HasSuffix(pic, "o") {
			pic = strings.TrimSuffix(pic, "o") + ";o"
		}
		n := 0
		switch comp {
		case 'M', 'x':
			if strings.HasPrefix(pic, "N") || pic == "n" {
				for j := time.January; j <= time.December; j++ {
					if strings.HasPrefix(strings.ToLower(j.String()), strings.ToLower(v)) {
						n = int(j)
						break
					}
				}
			} else {
				x, _ := parseInteger(c, v, pic)
				f, _ := x.(float64)
				n = floatToInt(f)
			}
		case 'P':
			pm = strings.ToLower(v)
		case 'Z', 'z':
			z := strings.TrimPrefix(v, "GMT")
			if z != "Z" {
				sign := 1
				if strings.HasPrefix(z, "-") {
					sign = -1
				}
				z = strings.TrimLeft(z, "+-")
				z = strings.ReplaceAll(z, ":", "")
				num, _ := strconv.Atoi(z)
				if len(z) <= 2 {
					offset = sign * num * 60
				} else {
					offset = sign * (num/100*60 + num%100)
				}
			}
		case 'F', 'C', 'E':
			continue
		default:
			x, e := parseInteger(c, v, pic)
			if e != nil {
				return nil, e
			}
			f, ok := x.(float64)
			if !ok {
				return Undefined, nil
			}
			n = floatToInt(f)
		}
		switch comp {
		case 'Y':
			year = n
			hasY = true
		case 'M':
			month = n
			hasM = true
		case 'D':
			day = n
			hasD = true
		case 'd':
			dayYear = n
			hasDay = true
		case 'H', 'h':
			hour = n
			hasHour = true
		case 'm':
			min = n
			hasMinute = true
		case 's':
			sec = n
			hasSecond = true
		case 'f':
			if len(v) > 3 {
				v = v[:3]
			}
			for len(v) < 3 {
				v += "0"
			}
			millis, _ = strconv.Atoi(v)
		}
	}
	if hasY && !hasM && !hasD && !hasDay {
		month = 1
		day = 1
	}
	if hasM && !hasD {
		day = 1
	}
	if hasDay {
		month = 1
		day = dayYear
	}
	if hasY && hasD && !hasM || hasM && hasMinute && !hasHour || hasHour && hasSecond && !hasMinute {
		return nil, builtinError("D3136", "toMillis")
	}
	if pm != "" {
		hour %= 12
		if pm == "pm" {
			hour += 12
		}
	}
	// Like JavaScript's Date.UTC, times beyond ±8.64e15 ms are NaN. Bound the
	// year first so time.Date cannot overflow into an in-range result.
	if year < -300000 || year > 300000 {
		return math.NaN(), nil
	}
	t := time.Date(year, time.Month(month), day, hour, min, sec, millis*1e6, time.FixedZone("", offset*60))
	if ms := t.UnixMilli(); ms < -8.64e15 || ms > 8.64e15 {
		return math.NaN(), nil
	}
	return float64(t.UnixMilli()), nil
}

func monthWeek(t time.Time) (int, int) {
	start := func(y int, m time.Month) time.Time {
		fourth := time.Date(y, m, 4, 0, 0, 0, 0, t.Location())
		weekday := int(fourth.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return fourth.AddDate(0, 0, 1-weekday)
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	base := start(t.Year(), t.Month())
	month := int(t.Month())
	if day.Before(base) {
		prev := t.AddDate(0, -1, 0)
		base = start(prev.Year(), prev.Month())
		month = int(prev.Month())
	} else {
		next := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
		nextBase := start(next.Year(), next.Month())
		if !day.Before(nextBase) {
			base = nextBase
			month = int(next.Month())
		}
	}
	return int(day.Sub(base).Hours()/24)/7 + 1, month
}
