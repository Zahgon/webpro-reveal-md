package jsutil

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// ParseJSON5 parses a JSON5 document (json5@2.2.3) into the Object/[]any/
// float64/string/bool/nil value model, preserving object key order.
//
// reveal-md reads reveal-md.json5 and reveal.json5 from the working directory,
// so the extensions over strict JSON all have to work: comments, unquoted
// identifier keys, single-quoted strings, trailing commas, hexadecimal and
// signed numbers, Infinity/NaN, and escaped line continuations.
func ParseJSON5(src string) (any, error) {
	p := &json5Parser{src: src}
	return p.parseTop()
}

type json5Parser struct {
	src    string
	pos    int
	strict bool // strict JSON: reject every JSON5 extension
}

func (p *json5Parser) parseTop() (any, error) {
	p.skipSpace()
	v, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos < len(p.src) {
		return nil, p.errorf("invalid character %q", p.peekRune())
	}
	return v, nil
}

func (p *json5Parser) errorf(format string, args ...any) error {
	line, col := 1, 1
	for i := 0; i < p.pos && i < len(p.src); i++ {
		if p.src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return fmt.Errorf("JSON5: %s at %d:%d", fmt.Sprintf(format, args...), line, col)
}

func (p *json5Parser) peekRune() rune {
	if p.pos >= len(p.src) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
	return r
}

func (p *json5Parser) skipSpace() {
	for p.pos < len(p.src) {
		r, size := utf8.DecodeRuneInString(p.src[p.pos:])
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			p.pos += size
		case !p.strict && (r == 0xFEFF || r == 0x00A0 || unicode.Is(unicode.Zs, r) ||
			r == 0x2028 || r == 0x2029):
			p.pos += size
		case !p.strict && r == '/' && p.pos+1 < len(p.src):
			switch p.src[p.pos+1] {
			case '/':
				p.pos += 2
				for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' {
					p.pos++
				}
			case '*':
				p.pos += 2
				for p.pos < len(p.src) {
					if p.src[p.pos] == '*' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/' {
						p.pos += 2
						break
					}
					p.pos++
				}
			default:
				return
			}
		default:
			return
		}
	}
}

func (p *json5Parser) parseValue() (any, error) {
	if p.pos >= len(p.src) {
		return nil, p.errorf("unexpected end of input")
	}
	switch c := p.src[p.pos]; {
	case c == '{':
		return p.parseObject()
	case c == '[':
		return p.parseArray()
	case c == '"':
		return p.parseString('"')
	case c == '\'' && !p.strict:
		p.pos++
		return p.parseStringBody('\'')
	case strings.HasPrefix(p.src[p.pos:], "true"):
		p.pos += 4
		return true, nil
	case strings.HasPrefix(p.src[p.pos:], "false"):
		p.pos += 5
		return false, nil
	case strings.HasPrefix(p.src[p.pos:], "null"):
		p.pos += 4
		return nil, nil
	default:
		return p.parseNumber()
	}
}

func (p *json5Parser) parseObject() (any, error) {
	p.pos++ // '{'
	obj := NewObject()
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == '}' {
		p.pos++
		return obj, nil
	}
	for {
		p.skipSpace()
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != ':' {
			return nil, p.errorf("expected ':'")
		}
		p.pos++
		p.skipSpace()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj.Set(key, val)
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, p.errorf("unexpected end of input")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
			p.skipSpace()
			if p.pos < len(p.src) && p.src[p.pos] == '}' {
				if p.strict {
					return nil, p.errorf("trailing comma")
				}
				p.pos++
				return obj, nil
			}
		case '}':
			p.pos++
			return obj, nil
		default:
			return nil, p.errorf("expected ',' or '}'")
		}
	}
}

func (p *json5Parser) parseKey() (string, error) {
	if p.pos >= len(p.src) {
		return "", p.errorf("unexpected end of input")
	}
	switch c := p.src[p.pos]; {
	case c == '"':
		s, err := p.parseString('"')
		return s, err
	case c == '\'' && !p.strict:
		p.pos++
		return p.parseStringBody('\'')
	case p.strict:
		return "", p.errorf("expected string key")
	}
	// Unquoted IdentifierName.
	start := p.pos
	for p.pos < len(p.src) {
		r, size := utf8.DecodeRuneInString(p.src[p.pos:])
		if r == '$' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == 0x200C || r == 0x200D || unicode.Is(unicode.Mn, r) ||
			unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Pc, r) {
			p.pos += size
			continue
		}
		if r == '\\' && p.pos+1 < len(p.src) && p.src[p.pos+1] == 'u' {
			p.pos += 2
			if p.pos+4 > len(p.src) {
				return "", p.errorf("invalid unicode escape")
			}
			p.pos += 4
			continue
		}
		break
	}
	if p.pos == start {
		return "", p.errorf("expected object key")
	}
	return p.src[start:p.pos], nil
}

func (p *json5Parser) parseArray() (any, error) {
	p.pos++ // '['
	arr := []any{}
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == ']' {
		p.pos++
		return arr, nil
	}
	for {
		p.skipSpace()
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, p.errorf("unexpected end of input")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
			p.skipSpace()
			if p.pos < len(p.src) && p.src[p.pos] == ']' {
				if p.strict {
					return nil, p.errorf("trailing comma")
				}
				p.pos++
				return arr, nil
			}
		case ']':
			p.pos++
			return arr, nil
		default:
			return nil, p.errorf("expected ',' or ']'")
		}
	}
}

func (p *json5Parser) parseString(quote byte) (string, error) {
	p.pos++
	return p.parseStringBody(quote)
}

func (p *json5Parser) parseStringBody(quote byte) (string, error) {
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == quote:
			p.pos++
			return b.String(), nil
		case c == '\\':
			p.pos++
			if p.pos >= len(p.src) {
				return "", p.errorf("unexpected end of input")
			}
			e := p.src[p.pos]
			switch e {
			case 'n':
				b.WriteByte('\n')
				p.pos++
			case 't':
				b.WriteByte('\t')
				p.pos++
			case 'r':
				b.WriteByte('\r')
				p.pos++
			case 'b':
				b.WriteByte('\b')
				p.pos++
			case 'f':
				b.WriteByte('\f')
				p.pos++
			case '/', '\\', '"', '\'':
				b.WriteByte(e)
				p.pos++
			case 'u':
				p.pos++
				r, err := p.readHex4()
				if err != nil {
					return "", err
				}
				if utf16.IsSurrogate(rune(r)) && p.pos+1 < len(p.src) &&
					p.src[p.pos] == '\\' && p.src[p.pos+1] == 'u' {
					save := p.pos
					p.pos += 2
					r2, err2 := p.readHex4()
					if err2 == nil {
						if combined := utf16.DecodeRune(rune(r), rune(r2)); combined != utf8.RuneError {
							b.WriteRune(combined)
							continue
						}
					}
					p.pos = save
				}
				b.WriteRune(rune(r))
			case 'v':
				if p.strict {
					return "", p.errorf("invalid escape")
				}
				b.WriteByte('\v')
				p.pos++
			case '0':
				if p.strict {
					return "", p.errorf("invalid escape")
				}
				b.WriteByte(0)
				p.pos++
			case 'x':
				if p.strict {
					return "", p.errorf("invalid escape")
				}
				p.pos++
				if p.pos+2 > len(p.src) {
					return "", p.errorf("invalid hex escape")
				}
				n, err := strconv.ParseUint(p.src[p.pos:p.pos+2], 16, 32)
				if err != nil {
					return "", p.errorf("invalid hex escape")
				}
				p.pos += 2
				b.WriteRune(rune(n))
			case '\n':
				if p.strict {
					return "", p.errorf("invalid escape")
				}
				p.pos++ // line continuation
			case '\r':
				if p.strict {
					return "", p.errorf("invalid escape")
				}
				p.pos++
				if p.pos < len(p.src) && p.src[p.pos] == '\n' {
					p.pos++
				}
			default:
				if p.strict {
					return "", p.errorf("invalid escape")
				}
				r, size := utf8.DecodeRuneInString(p.src[p.pos:])
				b.WriteRune(r)
				p.pos += size
			}
		case c == '\n' || c == '\r':
			return "", p.errorf("unterminated string")
		default:
			r, size := utf8.DecodeRuneInString(p.src[p.pos:])
			b.WriteRune(r)
			p.pos += size
		}
	}
	return "", p.errorf("unterminated string")
}

func (p *json5Parser) readHex4() (uint64, error) {
	if p.pos+4 > len(p.src) {
		return 0, p.errorf("invalid unicode escape")
	}
	n, err := strconv.ParseUint(p.src[p.pos:p.pos+4], 16, 32)
	if err != nil {
		return 0, p.errorf("invalid unicode escape")
	}
	p.pos += 4
	return n, nil
}

func (p *json5Parser) parseNumber() (any, error) {
	start := p.pos
	neg := false
	if p.pos < len(p.src) && (p.src[p.pos] == '-' || p.src[p.pos] == '+') {
		if p.src[p.pos] == '+' && p.strict {
			return nil, p.errorf("invalid number")
		}
		neg = p.src[p.pos] == '-'
		p.pos++
	}
	if !p.strict {
		if strings.HasPrefix(p.src[p.pos:], "Infinity") {
			p.pos += 8
			return math.Inf(map[bool]int{true: -1, false: 1}[neg]), nil
		}
		if strings.HasPrefix(p.src[p.pos:], "NaN") {
			p.pos += 3
			return math.NaN(), nil
		}
		if strings.HasPrefix(p.src[p.pos:], "0x") || strings.HasPrefix(p.src[p.pos:], "0X") {
			p.pos += 2
			hs := p.pos
			for p.pos < len(p.src) && isHexDigit(p.src[p.pos]) {
				p.pos++
			}
			if hs == p.pos {
				return nil, p.errorf("invalid number")
			}
			n, err := strconv.ParseUint(p.src[hs:p.pos], 16, 64)
			if err != nil {
				return nil, p.errorf("invalid number")
			}
			f := float64(n)
			if neg {
				f = -f
			}
			return f, nil
		}
	}
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.src) && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
			p.pos++
		}
		for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
	}
	text := p.src[start:p.pos]
	if text == "" || text == "-" || text == "+" {
		return nil, p.errorf("invalid number")
	}
	if p.strict {
		if strings.HasPrefix(text, ".") || strings.HasSuffix(text, ".") {
			return nil, p.errorf("invalid number")
		}
	}
	f, err := strconv.ParseFloat(strings.TrimPrefix(text, "+"), 64)
	if err != nil {
		return nil, p.errorf("invalid number %q", text)
	}
	return f, nil
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
