package jsonata

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

// dateMarkerText removes whitespace without decoding isolated UTF-16
// surrogates as replacement characters. An unchanged marker is borrowed.
func dateMarkerText(c *CallContext, text string) (string, error) {
	var out strings.Builder
	start, poll := 0, 0
	for i := 0; i < len(text); {
		if i >= poll {
			if err := builtinStep(c); err != nil {
				return "", err
			}
			poll = i + 64
		}
		r, size := regexDecodeRune(text[i:])
		if unicode.IsSpace(r) {
			out.WriteString(text[start:i])
			start = i + size
		}
		i += size
	}
	if start == 0 {
		return text, nil
	}
	out.WriteString(text[start:])
	return out.String(), nil
}

// Go's regexp package accepts scalar UTF-8. For matching, give each isolated
// surrogate an unused private-use scalar in both the subject and the picture.
// Keeping the mapping local also makes real private-use literals match exactly.
func projectDatePicture(c *CallContext, subject, picture string) (string, string, error) {
	if utf8.ValidString(subject) && utf8.ValidString(picture) {
		return subject, picture, nil
	}
	used := make(map[rune]bool)
	for _, text := range [...]string{subject, picture} {
		poll := 0
		for i := 0; i < len(text); {
			if i >= poll {
				if err := builtinStep(c); err != nil {
					return "", "", err
				}
				poll = i + 64
			}
			r, size := regexDecodeRune(text[i:])
			if isDatePrivateUse(r) {
				used[r] = true
			}
			i += size
		}
	}

	aliases := make(map[rune]rune)
	next := rune(0xe000)
	pick := func() (rune, error) {
		for i := 0; ; i++ {
			if i%64 == 0 {
				if err := builtinStep(c); err != nil {
					return 0, err
				}
			}
			switch next {
			case 0xf900:
				next = 0xf0000
			case 0xffffe:
				next = 0x100000
			case 0x10fffe:
				return 0, fmt.Errorf("%w: date picture UTF-16 projection", budget.ErrLimit)
			}
			r := next
			next++
			if !used[r] {
				return r, nil
			}
		}
	}
	project := func(text string) (string, error) {
		if utf8.ValidString(text) {
			return text, nil
		}
		var out strings.Builder
		poll := 0
		for i := 0; i < len(text); {
			if i >= poll {
				if err := builtinStep(c); err != nil {
					return "", err
				}
				poll = i + 64
			}
			r, size := regexDecodeRune(text[i:])
			switch {
			case r == utf8.RuneError && size == 1:
				return "", builtinError("D3110", "toMillis")
			case r >= 0xd800 && r <= 0xdfff:
				alias, ok := aliases[r]
				if !ok {
					var err error
					alias, err = pick()
					if err != nil {
						return "", err
					}
					aliases[r] = alias
				}
				out.WriteRune(alias)
			default:
				out.WriteString(text[i : i+size])
			}
			i += size
		}
		return out.String(), nil
	}
	s, err := project(subject)
	if err != nil {
		return "", "", err
	}
	p, err := project(picture)
	return s, p, err
}

func isDatePrivateUse(r rune) bool {
	return 0xe000 <= r && r <= 0xf8ff || 0xf0000 <= r && r <= 0xffffd || 0x100000 <= r && r <= 0x10fffd
}
