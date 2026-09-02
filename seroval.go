package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	serovalRefRe   = regexp.MustCompile(`^\$R\[(\d+)\](=?)`)
	serovalIdentRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*`)
	serovalNumRe   = regexp.MustCompile(`^-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?`)
)

// parseSerovalStream decodes a solid-start server function response: one or
// more `;0x<hex-len>;` frames whose concatenated bytes form a JS expression
// containing the serialized root value. Returns a plain Go value.
func parseSerovalStream(body string) (any, error) {
	var payload strings.Builder
	pos := 0
	for pos < len(body) {
		if !strings.HasPrefix(body[pos:], ";0x") {
			return nil, fmt.Errorf("seroval: bad frame header at offset %d", pos)
		}
		semi := strings.IndexByte(body[pos+3:], ';')
		if semi < 0 {
			return nil, fmt.Errorf("seroval: unterminated frame header")
		}
		n, err := strconv.ParseInt(body[pos+3:pos+3+semi], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("seroval: bad frame length: %w", err)
		}
		start := pos + 3 + semi + 1
		end := start + int(n)
		if end > len(body) {
			return nil, fmt.Errorf("seroval: frame length %d exceeds body size", n)
		}
		payload.WriteString(body[start:end])
		pos = end
	}
	expr := payload.String()
	i := strings.Index(expr, "$R[0]=")
	if i < 0 {
		return nil, fmt.Errorf("seroval: root reference $R[0] not found")
	}
	p := &serovalParser{s: expr, pos: i + len("$R[0]="), refs: map[int]any{}}
	return p.parseValue()
}

type serovalParser struct {
	s    string
	pos  int
	refs map[int]any
}

func (p *serovalParser) skipWS() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\n', '\t', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *serovalParser) parseValue() (any, error) {
	p.skipWS()
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("seroval: unexpected end of input")
	}
	switch c := p.s[p.pos]; {
	case c == '{':
		return p.parseObject()
	case c == '[':
		return p.parseArray()
	case c == '"' || c == '\'':
		return p.parseString(c)
	case c == '!':
		if strings.HasPrefix(p.s[p.pos:], "!0") {
			p.pos += 2
			return true, nil
		}
		if strings.HasPrefix(p.s[p.pos:], "!1") {
			p.pos += 2
			return false, nil
		}
		return nil, fmt.Errorf("seroval: unknown literal at %d", p.pos)
	case c == '$':
		return p.parseRef()
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	default:
		for _, lit := range []struct {
			text string
			val  any
		}{{"null", nil}, {"undefined", nil}, {"void 0", nil}, {"NaN", nil}} {
			if strings.HasPrefix(p.s[p.pos:], lit.text) {
				p.pos += len(lit.text)
				return lit.val, nil
			}
		}
		return nil, fmt.Errorf("seroval: unexpected character %q at %d", c, p.pos)
	}
}

func (p *serovalParser) parseRef() (any, error) {
	m := serovalRefRe.FindStringSubmatch(p.s[p.pos:])
	if m == nil {
		return nil, fmt.Errorf("seroval: bad reference at %d", p.pos)
	}
	n, _ := strconv.Atoi(m[1])
	p.pos += len(m[0])
	if m[2] == "=" {
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		p.refs[n] = v
		return v, nil
	}
	v, ok := p.refs[n]
	if !ok {
		return nil, fmt.Errorf("seroval: reference $R[%d] used before assignment", n)
	}
	return v, nil
}

func (p *serovalParser) parseObject() (any, error) {
	p.pos++ // consume '{'
	obj := map[string]any{}
	for {
		p.skipWS()
		if p.pos < len(p.s) && p.s[p.pos] == '}' {
			p.pos++
			return obj, nil
		}
		if p.pos >= len(p.s) {
			return nil, fmt.Errorf("seroval: unexpected end of input in object")
		}
		var key string
		if q := p.s[p.pos]; q == '"' || q == '\'' {
			k, err := p.parseString(q)
			if err != nil {
				return nil, err
			}
			key = k.(string)
		} else {
			m := serovalIdentRe.FindString(p.s[p.pos:])
			if m == "" {
				return nil, fmt.Errorf("seroval: bad object key at %d", p.pos)
			}
			key = m
			p.pos += len(m)
		}
		p.skipWS()
		if p.pos >= len(p.s) || p.s[p.pos] != ':' {
			return nil, fmt.Errorf("seroval: expected ':' at %d", p.pos)
		}
		p.pos++
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj[key] = v
		p.skipWS()
		if p.pos < len(p.s) && p.s[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.pos < len(p.s) && p.s[p.pos] == '}' {
			p.pos++
			return obj, nil
		}
		return nil, fmt.Errorf("seroval: expected ',' or '}' at %d", p.pos)
	}
}

func (p *serovalParser) parseArray() (any, error) {
	p.pos++ // consume '['
	arr := []any{}
	for {
		p.skipWS()
		if p.pos < len(p.s) && p.s[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		p.skipWS()
		if p.pos < len(p.s) && p.s[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.pos < len(p.s) && p.s[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		return nil, fmt.Errorf("seroval: expected ',' or ']' at %d", p.pos)
	}
}

func (p *serovalParser) parseString(quote byte) (any, error) {
	p.pos++
	var b strings.Builder
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if c == '\\' && p.pos+1 < len(p.s) {
			next := p.s[p.pos+1]
			switch next {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'u':
				if p.pos+6 <= len(p.s) {
					if code, err := strconv.ParseInt(p.s[p.pos+2:p.pos+6], 16, 32); err == nil {
						b.WriteRune(rune(code))
						p.pos += 6
						continue
					}
				}
				return nil, fmt.Errorf("seroval: bad unicode escape at %d", p.pos)
			default:
				b.WriteByte(next)
			}
			p.pos += 2
			continue
		}
		if c == quote {
			p.pos++
			return b.String(), nil
		}
		b.WriteByte(c)
		p.pos++
	}
	return nil, fmt.Errorf("seroval: unterminated string")
}

func (p *serovalParser) parseNumber() (any, error) {
	m := serovalNumRe.FindString(p.s[p.pos:])
	if m == "" {
		return nil, fmt.Errorf("seroval: bad number at %d", p.pos)
	}
	p.pos += len(m)
	if strings.ContainsAny(m, ".eE") {
		f, err := strconv.ParseFloat(m, 64)
		if err != nil {
			return nil, fmt.Errorf("seroval: bad float %q: %w", m, err)
		}
		return f, nil
	}
	i, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(m, 64)
		if ferr != nil {
			return nil, fmt.Errorf("seroval: bad int %q: %w", m, err)
		}
		return f, nil
	}
	return i, nil
}
