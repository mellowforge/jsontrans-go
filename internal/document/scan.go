package document

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

type scanner struct {
	d         *Document
	b         *budget.Budget
	i, depth  int
	nextCheck int
	err       error
	// Open containers share reusable scratch. A completed container copies its
	// direct items to the immutable document arena and releases its scratch tail.
	children []int
	members  []member
}

func Parse(ctx context.Context, input []byte, o Options) (*Document, error) {
	l, err := budget.Resolve(budget.DefaultLimits(), o.Limits)
	if err != nil {
		return nil, err
	}
	if len(input) > l.MaxInputBytes {
		return nil, fmt.Errorf("%w: input bytes", budget.ErrLimit)
	}
	var state budget.Budget
	b := &state
	err = budget.InitFastResolved(b, ctx, l)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if !utf8.Valid(input) {
		return nil, fmt.Errorf("%w: UTF-8", jsonvalue.ErrInvalid)
	}
	d := &Document{raw: string(input), nodes: make([]entry, 0, 16)}
	d.stats.Bytes = len(input)
	s := scanner{d: d, b: b, nextCheck: 1024}
	s.space()
	if _, err := s.value(); err != nil {
		return nil, err
	}
	s.space()
	if s.err != nil {
		return nil, s.err
	}
	if s.i != len(d.raw) {
		return nil, s.fail("trailing input")
	}
	if err := b.Check(); err != nil {
		return nil, err
	}
	d.stats.Nodes = len(d.nodes)
	d.stats.Binary64Overflow = d.nodes[0].overflow
	return d, nil
}
func (s *scanner) fail(message string) error {
	return fmt.Errorf("%w: %s at byte %d", jsonvalue.ErrInvalid, message, s.i)
}
func (s *scanner) space() {
	for s.i < len(s.d.raw) {
		switch s.d.raw[s.i] {
		case ' ', '\t', '\r', '\n':
			s.i++
			if s.i >= s.nextCheck {
				if err := s.progress(); err != nil {
					s.err = err
					return
				}
			}
		default:
			return
		}
	}
}
func (s *scanner) progress() error {
	s.nextCheck = s.i + 1024
	return s.b.Step()
}
func (s *scanner) value() (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if err := s.b.Enter(); err != nil {
		return 0, err
	}
	defer s.b.Leave()
	s.depth++
	defer func() { s.depth-- }()
	s.d.stats.Depth = max(s.d.stats.Depth, s.depth)
	if s.i >= len(s.d.raw) {
		return 0, s.fail("expected value")
	}
	id := len(s.d.nodes)
	s.d.nodes = append(s.d.nodes, entry{start: s.i})
	start := s.i
	ch := s.d.raw[s.i]
	switch ch {
	case '{':
		s.i++
		s.space()
		memberStart := len(s.members)
		var lookup map[string]int
		if s.i < len(s.d.raw) && s.d.raw[s.i] == '}' {
			s.i++
		} else {
			for {
				if err := s.b.Step(); err != nil {
					return 0, err
				}
				key, err := s.string()
				if err != nil {
					return 0, err
				}
				s.space()
				if s.i >= len(s.d.raw) || s.d.raw[s.i] != ':' {
					return 0, s.fail("expected colon")
				}
				s.i++
				s.space()
				child, err := s.value()
				if err != nil {
					return 0, err
				}
				found := -1
				if lookup != nil {
					if idx, ok := lookup[key]; ok {
						found = idx
					}
				} else {
					for i, m := range s.members[memberStart:] {
						if m.name == key {
							found = i
							break
						}
					}
				}
				if found >= 0 {
					s.members[memberStart+found].node = child
				} else {
					if err := s.b.Collection(len(s.members) - memberStart + 1); err != nil {
						return 0, err
					}
					s.members = append(s.members, member{key, child})
					if lookup == nil && len(s.members)-memberStart > 8 {
						lookup = make(map[string]int, len(s.members)-memberStart)
						for i, m := range s.members[memberStart:] {
							lookup[m.name] = i
						}
					} else if lookup != nil {
						lookup[key] = len(s.members) - memberStart - 1
					}
				}
				s.space()
				if s.i >= len(s.d.raw) {
					return 0, s.fail("unclosed object")
				}
				c := s.d.raw[s.i]
				s.i++
				if c == '}' {
					break
				}
				if c != ',' {
					return 0, s.fail("expected comma")
				}
				s.space()
			}
		}
		e := &s.d.nodes[id]
		e.kind = 'o'
		e.itemsStart = len(s.d.members)
		e.itemsLen = len(s.members) - memberStart
		s.d.members = append(s.d.members, s.members[memberStart:]...)
		// Clear decoded string references before reusing the scanner scratch.
		clear(s.members[memberStart:])
		s.members = s.members[:memberStart]
		if lookup != nil {
			if s.d.lookups == nil {
				s.d.lookups = make(map[int]map[string]int)
			}
			s.d.lookups[id] = lookup
		}
		for i, m := range s.d.members[e.itemsStart:] {
			if i&1023 == 1023 {
				if err := s.b.Step(); err != nil {
					return 0, err
				}
			}
			e.overflow = e.overflow || s.d.nodes[m.node].overflow
		}
		s.d.stats.MaxCollectionItems = max(s.d.stats.MaxCollectionItems, e.itemsLen)
	case '[':
		s.i++
		s.space()
		childStart := len(s.children)
		if s.i < len(s.d.raw) && s.d.raw[s.i] == ']' {
			s.i++
		} else {
			for {
				if err := s.b.Collection(len(s.children) - childStart + 1); err != nil {
					return 0, err
				}
				child, err := s.value()
				if err != nil {
					return 0, err
				}
				s.children = append(s.children, child)
				s.space()
				if s.i >= len(s.d.raw) {
					return 0, s.fail("unclosed array")
				}
				c := s.d.raw[s.i]
				s.i++
				if c == ']' {
					break
				}
				if c != ',' {
					return 0, s.fail("expected comma")
				}
				s.space()
			}
		}
		e := &s.d.nodes[id]
		e.kind = 'a'
		e.itemsStart = len(s.d.children)
		e.itemsLen = len(s.children) - childStart
		s.d.children = append(s.d.children, s.children[childStart:]...)
		s.children = s.children[:childStart]
		for i, child := range s.d.children[e.itemsStart:] {
			if i&1023 == 1023 {
				if err := s.b.Step(); err != nil {
					return 0, err
				}
			}
			e.overflow = e.overflow || s.d.nodes[child].overflow
		}
		s.d.stats.MaxCollectionItems = max(s.d.stats.MaxCollectionItems, e.itemsLen)
	case '"':
		text, err := s.string()
		if err != nil {
			return 0, err
		}
		s.d.nodes[id].kind = 's'
		s.d.nodes[id].itemsStart = -1
		if len(text) != s.i-start-2 {
			s.d.nodes[id].itemsStart = len(s.d.strings)
			s.d.strings = append(s.d.strings, text)
		}
	case 't', 'f', 'n':
		literal, kind := "null", byte('z')
		if ch == 't' {
			literal = "true"
			kind = 'b'
		} else if ch == 'f' {
			literal = "false"
			kind = 'b'
		}
		if len(s.d.raw)-s.i < len(literal) || s.d.raw[s.i:s.i+len(literal)] != literal {
			return 0, s.fail("invalid literal")
		}
		s.i += len(literal)
		s.d.nodes[id].kind = kind
	default:
		if ch != '-' && (ch < '0' || ch > '9') {
			return 0, s.fail("invalid value")
		}
		if ch == '-' {
			s.i++
			if s.i >= len(s.d.raw) {
				return 0, s.fail("invalid number")
			}
		}
		if s.d.raw[s.i] == '0' {
			s.i++
		} else {
			if s.d.raw[s.i] < '1' || s.d.raw[s.i] > '9' {
				return 0, s.fail("invalid number")
			}
			for s.i < len(s.d.raw) && s.d.raw[s.i] >= '0' && s.d.raw[s.i] <= '9' {
				s.i++
				if s.i >= s.nextCheck {
					if err := s.progress(); err != nil {
						return 0, err
					}
				}
			}
		}
		if s.i < len(s.d.raw) && s.d.raw[s.i] == '.' {
			s.i++
			begin := s.i
			for s.i < len(s.d.raw) && s.d.raw[s.i] >= '0' && s.d.raw[s.i] <= '9' {
				s.i++
				if s.i >= s.nextCheck {
					if err := s.progress(); err != nil {
						return 0, err
					}
				}
			}
			if begin == s.i {
				return 0, s.fail("invalid fraction")
			}
		}
		if s.i < len(s.d.raw) && (s.d.raw[s.i] == 'e' || s.d.raw[s.i] == 'E') {
			s.i++
			if s.i < len(s.d.raw) && (s.d.raw[s.i] == '+' || s.d.raw[s.i] == '-') {
				s.i++
				if s.i >= s.nextCheck {
					if err := s.progress(); err != nil {
						return 0, err
					}
				}
			}
			begin := s.i
			for s.i < len(s.d.raw) && s.d.raw[s.i] >= '0' && s.d.raw[s.i] <= '9' {
				s.i++
				if s.i >= s.nextCheck {
					if err := s.progress(); err != nil {
						return 0, err
					}
				}
			}
			if begin == s.i {
				return 0, s.fail("invalid exponent")
			}
		}
		n, _ := strconv.ParseFloat(s.d.raw[start:s.i], 64)
		e := &s.d.nodes[id]
		e.kind = 'n'
		e.number = n
		e.overflow = math.IsInf(n, 0) || math.IsNaN(n)
	}
	s.d.nodes[id].end = s.i
	return id, nil
}
func (s *scanner) string() (string, error) {
	if s.err != nil {
		return "", s.err
	}
	raw := s.d.raw
	if s.i >= len(raw) || raw[s.i] != '"' {
		return "", s.fail("expected string")
	}
	start := s.i
	s.i++
	escaped := false
	for s.i < len(raw) {
		if s.i >= s.nextCheck {
			if err := s.progress(); err != nil {
				return "", err
			}
		}
		ch := raw[s.i]
		s.i++
		if ch == '"' {
			if !escaped {
				return raw[start+1 : s.i-1], nil
			}
			return jsonvalue.DecodeString(raw[start:s.i])
		}
		if ch < 0x20 {
			return "", s.fail("control character in string")
		}
		if ch != '\\' {
			continue
		}
		escaped = true
		if s.i >= len(raw) {
			return "", s.fail("unterminated escape")
		}
		ch = raw[s.i]
		s.i++
		switch ch {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		case 'u':
			if len(raw)-s.i < 4 {
				return "", s.fail("short Unicode escape")
			}
			for j := 0; j < 4; j++ {
				c := raw[s.i+j]
				if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
					return "", s.fail("invalid Unicode escape")
				}
			}
			s.i += 4
		default:
			return "", s.fail("invalid escape")
		}
	}
	return "", s.fail("unterminated string")
}
