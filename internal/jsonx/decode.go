package jsonx

import (
	"fmt"
	"unicode/utf8"
)

// Decode parses JSON into the jsonx value model, preserving object key order and
// keeping numbers as verbatim Number literals. It exists mainly so tests can
// round-trip the real WikiComma output (decode then re-encode) and assert the
// bytes are unchanged, proving the encoder reproduces JS formatting.
func Decode(data []byte) (any, error) {
	p := &parser{s: data}
	p.ws()
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.ws()
	if p.i != len(p.s) {
		return nil, fmt.Errorf("jsonx: trailing data at offset %d", p.i)
	}
	return v, nil
}

type parser struct {
	s []byte
	i int
}

func (p *parser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) value() (any, error) {
	if p.i >= len(p.s) {
		return nil, fmt.Errorf("jsonx: unexpected end of input")
	}
	switch c := p.s[p.i]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		return p.string()
	case c == 't':
		return p.lit("true", true)
	case c == 'f':
		return p.lit("false", false)
	case c == 'n':
		return p.lit("null", nil)
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	default:
		return nil, fmt.Errorf("jsonx: unexpected character %q at offset %d", c, p.i)
	}
}

func (p *parser) lit(word string, val any) (any, error) {
	if p.i+len(word) > len(p.s) || string(p.s[p.i:p.i+len(word)]) != word {
		return nil, fmt.Errorf("jsonx: invalid literal at offset %d", p.i)
	}
	p.i += len(word)
	return val, nil
}

func (p *parser) object() (any, error) {
	o := NewObject()
	p.i++ // {
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return o, nil
	}
	for {
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != '"' {
			return nil, fmt.Errorf("jsonx: expected object key at offset %d", p.i)
		}
		key, err := p.string()
		if err != nil {
			return nil, err
		}
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return nil, fmt.Errorf("jsonx: expected ':' at offset %d", p.i)
		}
		p.i++
		p.ws()
		val, err := p.value()
		if err != nil {
			return nil, err
		}
		o.Set(key.(string), val)
		p.ws()
		if p.i >= len(p.s) {
			return nil, fmt.Errorf("jsonx: unterminated object")
		}
		if p.s[p.i] == ',' {
			p.i++
			continue
		}
		if p.s[p.i] == '}' {
			p.i++
			return o, nil
		}
		return nil, fmt.Errorf("jsonx: expected ',' or '}' at offset %d", p.i)
	}
}

func (p *parser) array() (any, error) {
	arr := []any{}
	p.i++ // [
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == ']' {
		p.i++
		return arr, nil
	}
	for {
		p.ws()
		val, err := p.value()
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)
		p.ws()
		if p.i >= len(p.s) {
			return nil, fmt.Errorf("jsonx: unterminated array")
		}
		if p.s[p.i] == ',' {
			p.i++
			continue
		}
		if p.s[p.i] == ']' {
			p.i++
			return arr, nil
		}
		return nil, fmt.Errorf("jsonx: expected ',' or ']' at offset %d", p.i)
	}
}

func (p *parser) string() (any, error) {
	start := p.i
	p.i++ // opening quote
	var sb []byte
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '"' {
			p.i++
			return string(sb), nil
		}
		if c == '\\' {
			p.i++
			if p.i >= len(p.s) {
				break
			}
			switch e := p.s[p.i]; e {
			case '"':
				sb = append(sb, '"')
			case '\\':
				sb = append(sb, '\\')
			case '/':
				sb = append(sb, '/')
			case 'b':
				sb = append(sb, '\b')
			case 'f':
				sb = append(sb, '\f')
			case 'n':
				sb = append(sb, '\n')
			case 'r':
				sb = append(sb, '\r')
			case 't':
				sb = append(sb, '\t')
			case 'u':
				r, err := p.unicodeEscape()
				if err != nil {
					return nil, err
				}
				var tmp [4]byte
				n := utf8.EncodeRune(tmp[:], r)
				sb = append(sb, tmp[:n]...)
				continue
			default:
				return nil, fmt.Errorf("jsonx: invalid escape \\%c at offset %d", e, p.i)
			}
			p.i++
			continue
		}
		sb = append(sb, c)
		p.i++
	}
	return nil, fmt.Errorf("jsonx: unterminated string starting at offset %d", start)
}

// unicodeEscape consumes \uXXXX (p.i points at 'u') and any trailing surrogate
// pair, returning the decoded rune.
func (p *parser) unicodeEscape() (rune, error) {
	hi, err := p.hex4()
	if err != nil {
		return 0, err
	}
	if hi >= 0xD800 && hi <= 0xDBFF {
		if p.i+2 <= len(p.s) && p.s[p.i] == '\\' && p.s[p.i+1] == 'u' {
			p.i += 2
			lo, err := p.hex4()
			if err != nil {
				return 0, err
			}
			if lo >= 0xDC00 && lo <= 0xDFFF {
				return ((hi - 0xD800) << 10) + (lo - 0xDC00) + 0x10000, nil
			}
			return utf8.RuneError, nil
		}
	}
	return hi, nil
}

// hex4 reads "uXXXX" where p.i points at the 'u'.
func (p *parser) hex4() (rune, error) {
	if p.i+5 > len(p.s) {
		return 0, fmt.Errorf("jsonx: truncated \\u escape at offset %d", p.i)
	}
	var r rune
	for k := 1; k <= 4; k++ {
		c := p.s[p.i+k]
		var d rune
		switch {
		case c >= '0' && c <= '9':
			d = rune(c - '0')
		case c >= 'a' && c <= 'f':
			d = rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = rune(c-'A') + 10
		default:
			return 0, fmt.Errorf("jsonx: invalid hex digit %q at offset %d", c, p.i+k)
		}
		r = r<<4 | d
	}
	p.i += 5
	return r, nil
}

func (p *parser) number() (any, error) {
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	for p.i < len(p.s) {
		c := p.s[p.i]
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			p.i++
			continue
		}
		break
	}
	return Number(p.s[start:p.i]), nil
}
