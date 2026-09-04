package jsutil

import (
	"strconv"
	"strings"
)

// This file ports yargs-parser 21.1.1 with the default configuration, which is
// what both lib/config.js and bin/reveal-md.js rely on. Go's flag package and
// every third-party alternative were rejected: reveal-md serialises the parsed
// argv straight into the generated HTML, so key ORDER, key DUPLICATION
// (camelCase twins) and value TYPES (numeric strings become numbers) are all
// observable in the output that tests compare byte for byte.
//
// A note on lib/config.js: it passes `{ boolean: true }`, but yargs-parser
// expects an array there and silently ignores a bare boolean. Confirmed in the
// oracle: the option has no effect, so the two call sites differ only in their
// alias maps.

// YargsOptions configures the parser. Aliases maps a key to its equivalents,
// exactly like yargs-parser's `alias` option.
type YargsOptions struct {
	Alias map[string][]string
}

// ParseArgv parses argv the way yargs-parser 21.1.1 does with default settings:
// dot-notation, number coercion, `--no-x` negation, short-flag grouping, `--`
// terminator, camel-case expansion and alias propagation.
//
// The result always starts with the "_" key holding the positional arguments,
// which is why an argument-free invocation serialises as {"_":[]}.
func ParseArgv(argv []string, opts YargsOptions) *Object {
	p := &yargsParser{
		out:        NewObject(),
		aliases:    expandAliases(opts.Alias),
		positional: []any{},
	}
	p.run(argv)
	return p.finish()
}

type yargsParser struct {
	out        *Object
	aliases    map[string][]string
	positional []any
}

// expandAliases closes the alias map over itself so that every member of an
// alias group maps to all the others, which is what yargs does internally.
func expandAliases(alias map[string][]string) map[string][]string {
	groups := map[string]map[string]bool{}
	keyOf := map[string]string{}

	merge := func(a, b string) {
		ga, aOK := keyOf[a]
		gb, bOK := keyOf[b]
		switch {
		case !aOK && !bOK:
			g := a
			groups[g] = map[string]bool{a: true, b: true}
			keyOf[a], keyOf[b] = g, g
		case aOK && !bOK:
			groups[ga][b] = true
			keyOf[b] = ga
		case !aOK && bOK:
			groups[gb][a] = true
			keyOf[a] = gb
		case ga != gb:
			for k := range groups[gb] {
				groups[ga][k] = true
				keyOf[k] = ga
			}
			delete(groups, gb)
		}
	}

	for k, vs := range alias {
		for _, v := range vs {
			merge(k, v)
		}
	}

	out := map[string][]string{}
	for member, g := range keyOf {
		for other := range groups[g] {
			if other != member {
				out[member] = append(out[member], other)
			}
		}
	}
	return out
}

func (p *yargsParser) run(argv []string) {
	for i := 0; i < len(argv); i++ {
		arg := argv[i]

		if arg == "--" {
			for _, rest := range argv[i+1:] {
				p.positional = append(p.positional, coerceValue(rest))
			}
			return
		}

		switch {
		case strings.HasPrefix(arg, "--"):
			i = p.parseLong(argv, i, arg[2:])
		case len(arg) > 1 && arg[0] == '-' && !isNumericArg(arg):
			i = p.parseShort(argv, i, arg[1:])
		default:
			p.positional = append(p.positional, coerceValue(arg))
		}
	}
}

func (p *yargsParser) parseLong(argv []string, i int, body string) int {
	if eq := strings.Index(body, "="); eq >= 0 {
		p.assign(body[:eq], coerceValue(body[eq+1:]))
		return i
	}
	if strings.HasPrefix(body, "no-") {
		p.assign(body[3:], false)
		return i
	}
	if next, ok := peekValue(argv, i); ok {
		p.assign(body, coerceValue(next))
		return i + 1
	}
	p.assign(body, true)
	return i
}

// parseShort handles grouped short flags (-abc), an attached value (-p1948)
// and a following value (-t solarized).
func (p *yargsParser) parseShort(argv []string, i int, body string) int {
	if eq := strings.Index(body, "="); eq >= 0 {
		p.assign(body[:eq], coerceValue(body[eq+1:]))
		return i
	}
	for j := 0; j < len(body); j++ {
		letter := string(body[j])
		rest := body[j+1:]
		if rest != "" {
			// A digit or "=" directly after the letter is its value.
			if rest[0] == '=' {
				p.assign(letter, coerceValue(rest[1:]))
				return i
			}
			if isDigitByte(rest[0]) {
				p.assign(letter, coerceValue(rest))
				return i
			}
			p.assign(letter, true)
			continue
		}
		if next, ok := peekValue(argv, i); ok {
			p.assign(letter, coerceValue(next))
			return i + 1
		}
		p.assign(letter, true)
	}
	return i
}

// peekValue reports whether the next argv entry is a value rather than
// another flag. Negative numbers count as values.
func peekValue(argv []string, i int) (string, bool) {
	if i+1 >= len(argv) {
		return "", false
	}
	next := argv[i+1]
	if next == "--" {
		return "", false
	}
	if strings.HasPrefix(next, "-") && len(next) > 1 && !isNumericArg(next) {
		return "", false
	}
	return next, true
}

func isNumericArg(s string) bool {
	if !strings.HasPrefix(s, "-") {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

// assign writes a key, its dot-notation path, its aliases and its camelCase
// twin, in the order yargs emits them: the literal key first, then aliases,
// then the camelCase form.
func (p *yargsParser) assign(key string, value any) {
	p.setPath(key, value)
	for _, a := range p.aliases[key] {
		p.setPath(a, value)
		if camel := toCamelCase(a); camel != a {
			p.setPath(camel, value)
		}
	}
	if camel := toCamelCase(key); camel != key {
		p.setPath(camel, value)
	}
}

// setPath supports yargs' dot-notation (--a.b 1 => {a: {b: 1}}) and turns a
// repeated flag into an array.
func (p *yargsParser) setPath(key string, value any) {
	if key == "" {
		return
	}
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		p.setKey(p.out, key, value)
		return
	}
	cur := p.out
	for _, seg := range parts[:len(parts)-1] {
		child, ok := cur.Get(seg).(*Object)
		if !ok {
			child = NewObject()
			cur.Set(seg, child)
		}
		cur = child
	}
	p.setKey(cur, parts[len(parts)-1], value)
}

func (p *yargsParser) setKey(obj *Object, key string, value any) {
	if !obj.Has(key) {
		obj.Set(key, value)
		return
	}
	switch existing := obj.Get(key).(type) {
	case []any:
		obj.Set(key, append(existing, value))
	default:
		obj.Set(key, []any{existing, value})
	}
}

// toCamelCase converts kebab-case to camelCase, leaving other keys untouched.
func toCamelCase(key string) string {
	if !strings.Contains(key, "-") {
		return key
	}
	parts := strings.Split(key, "-")
	var b strings.Builder
	b.WriteString(parts[0])
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

// coerceValue reproduces yargs' number detection. The rules are narrower than
// they look and were confirmed empirically:
//
//	'1948' => 1948, '0x10' => 16, '1e3' => 1000, '8.5' => 8.5
//	'007'  => '007' (leading zeros keep it a string)
//	''     => ''
func coerceValue(s string) any {
	if s == "" {
		return ""
	}
	if looksLikeNumber(s) {
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			if n, err := strconv.ParseInt(s[2:], 16, 64); err == nil {
				return float64(n)
			}
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return s
}

// looksLikeNumber mirrors yargs-parser's isNumber check, including its refusal
// to convert values with redundant leading zeros.
func looksLikeNumber(s string) bool {
	body := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if body == "" {
		return false
	}
	if strings.HasPrefix(body, "0x") || strings.HasPrefix(body, "0X") {
		_, err := strconv.ParseInt(body[2:], 16, 64)
		return err == nil
	}
	if len(body) > 1 && body[0] == '0' && isDigitByte(body[1]) {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// finish prepends the "_" key, matching yargs' output shape.
func (p *yargsParser) finish() *Object {
	result := NewObject()
	result.Set("_", p.positional)
	for _, e := range p.out.Entries() {
		result.Set(e.Key, e.Value)
	}
	return result
}
