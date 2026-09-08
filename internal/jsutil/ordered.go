// Package jsutil re-implements the JavaScript/Node.js runtime semantics that
// reveal-md depends on. Every helper here exists because the idiomatic Go
// equivalent is observably different from what the original JavaScript did.
//
// Nothing in this package should be "cleaned up" to look more like normal Go:
// the awkward parts are load-bearing.
package jsutil

import (
	"math"
	"strconv"
)

// Undefined is the JavaScript `undefined` value.
//
// Go has exactly one empty value (nil) where JavaScript has two (null and
// undefined), and the difference is observable throughout reveal-md:
//
//   - JSON.stringify omits object properties whose value is undefined but
//     emits `null` for properties whose value is null.
//   - lodash's _.defaults/_.defaultsDeep only fill in properties that are
//     undefined; a property explicitly set to null keeps its null.
//
// Collapsing the two would silently change the generated HTML.
type Undefined struct{}

// Undef is the singleton undefined value.
var Undef = Undefined{}

// IsUndefined reports whether v is the JavaScript undefined value.
func IsUndefined(v any) bool {
	_, ok := v.(Undefined)
	return ok
}

// Object models a JavaScript plain object: a string-keyed map that remembers
// the order its keys were created in.
//
// Key order is not cosmetic here. render.js serialises the merged options with
// JSON.stringify and embeds the result verbatim in the generated HTML, so the
// order of keys in this map is part of the program's byte-for-byte output.
// Go's built-in map type randomises iteration order and therefore cannot be
// used for any value that reaches the templates.
type Object struct {
	keys []string
	vals map[string]any
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{vals: make(map[string]any)}
}

// ObjectOf builds an object from alternating key/value arguments, in order.
func ObjectOf(kv ...any) *Object {
	o := NewObject()
	for i := 0; i+1 < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			continue
		}
		o.Set(k, kv[i+1])
	}
	return o
}

// Len returns the number of own enumerable properties.
func (o *Object) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// Has reports whether the property exists (even if its value is undefined).
func (o *Object) Has(key string) bool {
	if o == nil {
		return false
	}
	_, ok := o.vals[key]
	return ok
}

// Get returns the property value, or Undef when the property is absent.
// This mirrors JavaScript, where reading a missing property yields undefined
// rather than an error.
func (o *Object) Get(key string) any {
	if o == nil {
		return Undef
	}
	v, ok := o.vals[key]
	if !ok {
		return Undef
	}
	return v
}

// GetString returns the property as a string when it is one.
func (o *Object) GetString(key string) (string, bool) {
	s, ok := o.Get(key).(string)
	return s, ok
}

// Set assigns a property, appending the key on first assignment and keeping
// its original position on re-assignment. This is JavaScript's behaviour:
// `obj.a = 1; obj.b = 2; obj.a = 3` still iterates a before b.
func (o *Object) Set(key string, val any) {
	if o.vals == nil {
		o.vals = make(map[string]any)
	}
	if _, exists := o.vals[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = val
}

// Delete removes a property.
func (o *Object) Delete(key string) {
	if o == nil {
		return
	}
	if _, exists := o.vals[key]; !exists {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Keys returns the own enumerable property names in JavaScript's
// [[OwnPropertyKeys]] order: array-index-like keys first, in ascending
// numeric order, then the remaining string keys in insertion order.
//
// Yes, this really is how JavaScript orders object keys, and yes, it is
// observable here: a YAML front matter block or a reveal.json file may use
// numeric keys, and those keys would then be serialised in a different
// position than a naive insertion-ordered map would place them.
func (o *Object) Keys() []string {
	if o == nil {
		return nil
	}
	var indexKeys []string
	var stringKeys []string
	for _, k := range o.keys {
		if isArrayIndexKey(k) {
			indexKeys = append(indexKeys, k)
		} else {
			stringKeys = append(stringKeys, k)
		}
	}
	if len(indexKeys) > 1 {
		// Ascending numeric order.
		for i := 1; i < len(indexKeys); i++ {
			for j := i; j > 0; j-- {
				a, _ := strconv.ParseUint(indexKeys[j-1], 10, 64)
				b, _ := strconv.ParseUint(indexKeys[j], 10, 64)
				if a <= b {
					break
				}
				indexKeys[j-1], indexKeys[j] = indexKeys[j], indexKeys[j-1]
			}
		}
	}
	return append(indexKeys, stringKeys...)
}

// isArrayIndexKey reports whether key is a canonical array index string,
// i.e. the result of String(n) for an integer n in [0, 2^32-2].
func isArrayIndexKey(key string) bool {
	if key == "" || len(key) > 10 {
		return false
	}
	if key == "0" {
		return true
	}
	if key[0] == '0' { // "01" is not canonical
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '0' || key[i] > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(key, 10, 64)
	return err == nil && n < math.MaxUint32
}

// Entries returns the properties in Keys() order.
func (o *Object) Entries() []Entry {
	keys := o.Keys()
	entries := make([]Entry, 0, len(keys))
	for _, k := range keys {
		entries = append(entries, Entry{Key: k, Value: o.vals[k]})
	}
	return entries
}

// Entry is a single key/value pair.
type Entry struct {
	Key   string
	Value any
}

// Clone returns a shallow copy, like Object.assign({}, o).
func (o *Object) Clone() *Object {
	c := NewObject()
	if o == nil {
		return c
	}
	for _, k := range o.Keys() {
		c.Set(k, o.vals[k])
	}
	return c
}

// Assign copies own enumerable properties from each source onto dst, in
// argument order, exactly like Object.assign. Later sources overwrite earlier
// ones, and undefined values DO overwrite (unlike _.defaults).
func Assign(dst *Object, sources ...*Object) *Object {
	if dst == nil {
		dst = NewObject()
	}
	for _, src := range sources {
		if src == nil {
			continue
		}
		for _, e := range src.Entries() {
			dst.Set(e.Key, e.Value)
		}
	}
	return dst
}

// DeepClone copies objects and arrays recursively. Scalars are immutable in
// this representation and are shared.
func DeepClone(v any) any {
	switch t := v.(type) {
	case *Object:
		c := NewObject()
		for _, e := range t.Entries() {
			c.Set(e.Key, DeepClone(e.Value))
		}
		return c
	case []any:
		c := make([]any, len(t))
		for i, item := range t {
			c[i] = DeepClone(item)
		}
		return c
	default:
		return v
	}
}

// Truthy implements JavaScript's truthiness rules, which differ from Go's
// zero-value conventions: empty string, 0, NaN, null and undefined are falsy,
// but empty objects and empty arrays are TRUTHY.
//
// This matters in render.js (`options.mermaid === false` vs a truthy `{}`) and
// throughout the Mustache templates, where a section renders when its value is
// truthy.
func Truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case Undefined:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0 && !math.IsNaN(t)
	case int:
		return t != 0
	case []any:
		return true // [] is truthy in JavaScript
	case *Object:
		return true // {} is truthy in JavaScript
	default:
		return true
	}
}
