package jsutil

import (
	"math"
	"strings"
)

// Defaults implements _.defaults(object, ...sources).
//
// The rule is "assign the source value only when the destination property is
// undefined". A property explicitly set to null is NOT filled in, which is
// why Undefined has to exist as a distinct value in this codebase:
//
//	_.defaults({}, {a: undefined, b: null}, {a: 9, b: 9, c: 9})
//	  => { a: 9, b: null, c: 9 }
//
// Verified against lodash 4.17.21 in the oracle project.
func Defaults(dst *Object, sources ...*Object) *Object {
	if dst == nil {
		dst = NewObject()
	}
	for _, src := range sources {
		if src == nil {
			continue
		}
		for _, e := range src.Entries() {
			if !dst.Has(e.Key) || IsUndefined(dst.Get(e.Key)) {
				dst.Set(e.Key, e.Value)
			}
		}
	}
	return dst
}

// DefaultsDeep implements _.defaultsDeep(object, ...sources).
//
// Two behaviours here are surprising and both are observable in reveal-md's
// output, so they are reproduced deliberately:
//
//  1. Arrays are merged INDEX-WISE, not replaced:
//     _.defaultsDeep({}, {a: [1]}, {a: [7, 8, 9]}) => { a: [1, 8, 9] }
//     An empty array therefore inherits the whole default array.
//
//  2. A null value blocks the default entirely, even when the default is an
//     object: _.defaultsDeep({}, {a: null}, {a: {x: 1}}) => { a: null }
//
// Both verified against lodash 4.17.21 in the oracle project.
func DefaultsDeep(dst *Object, sources ...*Object) *Object {
	if dst == nil {
		dst = NewObject()
	}
	for _, src := range sources {
		if src == nil {
			continue
		}
		mergeDefaults(dst, src)
	}
	return dst
}

func mergeDefaults(dst *Object, src *Object) {
	for _, e := range src.Entries() {
		cur := dst.Get(e.Key)
		if !dst.Has(e.Key) || IsUndefined(cur) {
			dst.Set(e.Key, DeepClone(e.Value))
			continue
		}
		switch curVal := cur.(type) {
		case *Object:
			if srcObj, ok := e.Value.(*Object); ok {
				mergeDefaults(curVal, srcObj)
			}
		case []any:
			if srcArr, ok := e.Value.([]any); ok {
				dst.Set(e.Key, mergeDefaultsArray(curVal, srcArr))
			}
		}
	}
}

// mergeDefaultsArray fills holes and missing tail elements of dst from src,
// recursing into element objects, which is what lodash's array merge does.
func mergeDefaultsArray(dst, src []any) []any {
	out := make([]any, len(dst))
	copy(out, dst)
	for i, sv := range src {
		if i >= len(out) {
			out = append(out, DeepClone(sv))
			continue
		}
		if IsUndefined(out[i]) {
			out[i] = DeepClone(sv)
			continue
		}
		switch dv := out[i].(type) {
		case *Object:
			if so, ok := sv.(*Object); ok {
				mergeDefaults(dv, so)
			}
		case []any:
			if sa, ok := sv.([]any); ok {
				out[i] = mergeDefaultsArray(dv, sa)
			}
		}
	}
	return out
}

// Pick implements _.pick(object, keys): the result contains the requested
// keys in the order they were REQUESTED (not the order they appear in the
// source), and keys that are absent from the source are skipped entirely.
func Pick(src *Object, keys []string) *Object {
	out := NewObject()
	if src == nil {
		return out
	}
	for _, k := range keys {
		if src.Has(k) {
			out.Set(k, src.Get(k))
		}
	}
	return out
}

// Omit implements _.omit(object, keys), preserving the source's key order.
func Omit(src *Object, keys ...string) *Object {
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	out := NewObject()
	if src == nil {
		return out
	}
	for _, e := range src.Entries() {
		if !drop[e.Key] {
			out.Set(e.Key, e.Value)
		}
	}
	return out
}

// ParseIntJS implements _.parseInt(value), which delegates to JavaScript's
// global parseInt with radix 10: leading whitespace and an optional sign are
// consumed, digits are read until the first non-digit, and an empty digit run
// yields NaN.
//
//	'3' => 3, '3x' => 3, 'x' => NaN, '' => NaN, '08' => 8
func ParseIntJS(v any) float64 {
	s, ok := v.(string)
	if !ok {
		switch n := v.(type) {
		case float64:
			return math.Trunc(n)
		case int:
			return float64(n)
		default:
			return math.NaN()
		}
	}
	s = JSTrimStart(s)
	i := 0
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return math.NaN()
	}
	var n float64
	for _, c := range s[start:i] {
		n = n*10 + float64(c-'0')
	}
	if neg {
		n = -n
	}
	return n
}

// Flatten implements _.flatten (one level deep).
func Flatten(items []any) []any {
	out := []any{}
	for _, it := range items {
		if sub, ok := it.([]any); ok {
			out = append(out, sub...)
			continue
		}
		out = append(out, it)
	}
	return out
}

// Compact implements _.compact: drop every falsy element.
func Compact(items []any) []any {
	out := []any{}
	for _, it := range items {
		if Truthy(it) {
			out = append(out, it)
		}
	}
	return out
}

// jsWhitespace is the exact set of characters JavaScript's String#trim
// removes: WhiteSpace plus LineTerminator from the ECMAScript grammar.
// It is NOT the same set as Go's unicode.IsSpace, which excludes U+FEFF and
// includes characters JavaScript does not treat as whitespace.
const jsWhitespace = "\t\v\f \u00a0\ufeff\n\r\u2028\u2029" +
	"\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a" +
	"\u202f\u205f\u3000"

// JSTrim implements String.prototype.trim().
func JSTrim(s string) string { return strings.Trim(s, jsWhitespace) }

// JSTrimStart implements String.prototype.trimStart().
func JSTrimStart(s string) string { return strings.TrimLeft(s, jsWhitespace) }

// JSString implements the JavaScript String(value) conversion for the values
// this codebase can hold. Arrays join with "," and null/undefined elements
// become empty strings; plain objects become "[object Object]".
func JSString(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case Undefined:
		return "undefined"
	case bool:
		if t {
			return "true"
		}
		return "false"
	case string:
		return t
	case float64:
		return NumberToString(t)
	case int:
		return NumberToString(float64(t))
	case *JSDate:
		return t.String()
	case []any:
		parts := make([]string, len(t))
		for i, item := range t {
			if item == nil || IsUndefined(item) {
				parts[i] = ""
				continue
			}
			parts[i] = JSString(item)
		}
		return strings.Join(parts, ",")
	case *Object:
		return "[object Object]"
	default:
		return ""
	}
}
