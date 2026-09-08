package jsutil

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Stringify implements JSON.stringify(value) with no replacer and no
// indentation, which is exactly how render.js serialises the reveal.js and
// mermaid option objects into the generated HTML.
//
// encoding/json cannot be used for this:
//   - it sorts map keys alphabetically, while JavaScript preserves insertion
//     order (see Object.Keys);
//   - it HTML-escapes <, > and & by default;
//   - it formats floats with Go's algorithm, not ECMAScript's Number::toString;
//   - it has no notion of undefined, which JSON.stringify drops from objects
//     but renders as null inside arrays.
//
// ok is false when the value itself serialises to undefined, in which case
// JSON.stringify returns undefined rather than a string.
func Stringify(v any) (s string, ok bool) {
	var b strings.Builder
	if !stringifyValue(&b, v) {
		return "", false
	}
	return b.String(), true
}

// StringifyOrEmpty returns the serialisation, or "" when the value is undefined.
func StringifyOrEmpty(v any) string {
	s, ok := Stringify(v)
	if !ok {
		return ""
	}
	return s
}

func stringifyValue(b *strings.Builder, v any) bool {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
		return true
	case Undefined:
		return false
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		return true
	case string:
		b.WriteString(QuoteJSON(t))
		return true
	case *JSDate:
		b.WriteString(QuoteJSON(t.ISOString()))
		return true
	case float64:
		b.WriteString(numberToJSON(t))
		return true
	case int:
		b.WriteString(numberToJSON(float64(t)))
		return true
	case []any:
		b.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			// Inside an array, undefined becomes null.
			if !stringifyValue(b, item) {
				b.WriteString("null")
			}
		}
		b.WriteByte(']')
		return true
	case *Object:
		b.WriteByte('{')
		first := true
		for _, e := range t.Entries() {
			var vb strings.Builder
			if !stringifyValue(&vb, e.Value) {
				// Object properties whose value is undefined are omitted.
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteString(QuoteJSON(e.Key))
			b.WriteByte(':')
			b.WriteString(vb.String())
		}
		b.WriteByte('}')
		return true
	default:
		return false
	}
}

// numberToJSON renders a float the way JSON.stringify does: NaN and the
// infinities become null, everything else goes through Number::toString.
func numberToJSON(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "null"
	}
	return NumberToString(f)
}

// NumberToString implements ECMAScript's Number::toString (ES2023 §6.1.6.1.20).
//
// Go's strconv formats floats differently from JavaScript at the boundaries:
// JavaScript switches to exponential notation only when the decimal exponent
// is below -6 or at least 21, and it always emits the shortest representation
// that round-trips. `strconv.FormatFloat(f, 'g', -1, 64)` switches to
// exponents far earlier and spells them differently ("1e+21" vs "1e+21" is a
// coincidence; "0.000001" vs "1e-06" is not).
func NumberToString(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	if f == 0 {
		// JavaScript prints negative zero as "0".
		return "0"
	}
	if f < 0 {
		return "-" + NumberToString(-f)
	}

	// Shortest round-tripping digits and decimal exponent.
	e := strconv.FormatFloat(f, 'e', -1, 64) // e.g. "1.024e+03"
	mantissa, expPart, _ := strings.Cut(e, "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	exp10, err := strconv.Atoi(expPart)
	if err != nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}

	k := len(digits) // number of significant digits
	n := exp10 + 1   // position of the decimal point relative to the digits

	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}

	sign := "+"
	ex := n - 1
	if ex < 0 {
		sign = "-"
		ex = -ex
	}
	if k == 1 {
		return digits + "e" + sign + strconv.Itoa(ex)
	}
	return digits[:1] + "." + digits[1:] + "e" + sign + strconv.Itoa(ex)
}

// QuoteJSON renders a string as a JSON string literal the way JSON.stringify
// does: only the mandatory escapes, non-ASCII emitted literally as UTF-8, and
// no HTML escaping.
func QuoteJSON(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			switch {
			case r < 0x20:
				b.WriteString(`\u`)
				const hex = "0123456789abcdef"
				b.WriteByte('0')
				b.WriteByte('0')
				b.WriteByte(hex[(r>>4)&0xF])
				b.WriteByte(hex[r&0xF])
			case r == utf8.RuneError:
				// Invalid UTF-8 decodes to U+FFFD, which is what Node's
				// Buffer#toString('utf8') produces as well.
				b.WriteRune(r)
			default:
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ParseJSON parses JSON into the same value model used by Stringify, keeping
// object key order. It is used for defaults.json and package metadata.
func ParseJSON(src string) (any, error) {
	p := &json5Parser{src: src, strict: true}
	return p.parseTop()
}
