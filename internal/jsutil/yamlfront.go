package jsutil

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ContentKey is yaml-front-matter's default contentKeyName.
const ContentKey = "__content"

// frontMatterRe is a transcription of yaml-front-matter 4.1.1's regex:
//
//	/^(-{3}(?:\n|\r)([\w\W]+?)(?:\n|\r)-{3})?([\w\W]*)*/
//
// Two consequences are load-bearing and must not be "fixed":
//   - the closing --- is not followed by a newline match, so __content keeps
//     the line break that follows it ("---\ntitle: x\n---\nBody" yields
//     "\nBody");
//   - the opening delimiter is anchored at offset 0, so a document starting
//     with a blank line has no front matter at all.
var frontMatterRe = regexp.MustCompile(`^(?:-{3}(?:\n|\r)((?s:.+?))(?:\n|\r)-{3})?((?s:.*))`)

// LoadFront implements yamlFrontMatter.loadFront(text).
//
// The returned object always ends with the __content key, matching the
// property insertion order of the JavaScript implementation.
func LoadFront(text string) (*Object, error) {
	m := frontMatterRe.FindStringSubmatch(text)
	conf := NewObject()
	if m == nil {
		conf.Set(ContentKey, text)
		return conf, nil
	}
	body, content := m[1], m[2]
	if body != "" {
		if strings.HasPrefix(body, "{") {
			parsed, err := ParseJSON(body)
			if err != nil {
				return nil, err
			}
			if obj, ok := parsed.(*Object); ok {
				conf = obj
			}
		} else {
			parsed, err := YAMLLoad(body)
			if err != nil {
				return nil, err
			}
			if obj, ok := parsed.(*Object); ok {
				conf = obj
			}
		}
	}
	conf.Set(ContentKey, content)
	return conf, nil
}

// YAMLLoad implements js-yaml 3.14.1's load() for the value shapes that can
// appear in front matter.
//
// gopkg.in/yaml.v3 is used only as a tokenizer/structure parser: its own tag
// resolution follows YAML 1.2 core, whereas js-yaml 3 follows YAML 1.1, and
// the two disagree on real front-matter values (017 is 15 not 17, 1:30 is 90
// not a string, 0o17 is a string not 15). Every plain scalar is therefore
// re-resolved here by resolveScalar.
func YAMLLoad(src string) (any, error) {
	// js-yaml treats end-of-input as a line break, so a trailing folded or
	// literal block scalar keeps its clipped newline even when the source has
	// none; go-yaml follows the spec instead and drops it.
	if !strings.HasSuffix(src, "\n") {
		src += "\n"
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		return nil, err
	}
	if root.Kind == 0 || len(root.Content) == 0 {
		return nil, nil
	}
	return yamlNodeValue(root.Content[0])
}

func yamlNodeValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return yamlNodeValue(n.Content[0])
	case yaml.AliasNode:
		return yamlNodeValue(n.Alias)
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := yamlNodeValue(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.MappingNode:
		return yamlMappingValue(n)
	case yaml.ScalarNode:
		return resolveScalar(n), nil
	default:
		return nil, nil
	}
}

func yamlMappingValue(n *yaml.Node) (any, error) {
	out := NewObject()
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode, valNode := n.Content[i], n.Content[i+1]
		if isMergeKey(keyNode) {
			if err := applyMerge(out, valNode); err != nil {
				return nil, err
			}
			continue
		}
		kv, err := yamlNodeValue(keyNode)
		if err != nil {
			return nil, err
		}
		key := yamlKeyString(kv)
		if out.Has(key) {
			return nil, fmt.Errorf("duplicated mapping key at line %d, column %d", keyNode.Line, keyNode.Column)
		}
		v, err := yamlNodeValue(valNode)
		if err != nil {
			return nil, err
		}
		out.Set(key, v)
	}
	return out, nil
}

func isMergeKey(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Value == "<<" && n.Style == 0
}

func applyMerge(dst *Object, valNode *yaml.Node) error {
	sources := []*yaml.Node{valNode}
	if valNode.Kind == yaml.SequenceNode {
		sources = valNode.Content
	}
	for _, src := range sources {
		if src.Kind == yaml.AliasNode {
			src = src.Alias
		}
		if src.Kind != yaml.MappingNode {
			continue
		}
		merged, err := yamlMappingValue(src)
		if err != nil {
			return err
		}
		obj, ok := merged.(*Object)
		if !ok {
			continue
		}
		for _, e := range obj.Entries() {
			if !dst.Has(e.Key) {
				dst.Set(e.Key, e.Value)
			}
		}
	}
	return nil
}

func yamlKeyString(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case *JSDate:
		return t.String()
	default:
		return JSString(v)
	}
}

func resolveScalar(n *yaml.Node) any {
	if n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return n.Value
	}
	if n.Style&yaml.TaggedStyle != 0 {
		return resolveTagged(n)
	}
	return resolvePlainScalar(n.Value)
}

func resolveTagged(n *yaml.Node) any {
	switch n.Tag {
	case "!!str":
		return n.Value
	case "!!null":
		return nil
	case "!!bool":
		return resolveYamlBool(n.Value) == boolTrue
	case "!!int", "!!float":
		if v, ok := resolveYamlNumber(n.Value); ok {
			return v
		}
		return n.Value
	default:
		return resolvePlainScalar(n.Value)
	}
}

// resolvePlainScalar applies js-yaml 3's implicit resolvers in schema order:
// null, bool, int, float, timestamp; anything unresolved stays a string.
func resolvePlainScalar(s string) any {
	if s == "" {
		return nil
	}
	if resolveYamlNull(s) {
		return nil
	}
	switch resolveYamlBool(s) {
	case boolTrue:
		return true
	case boolFalse:
		return false
	}
	if v, ok := resolveYamlInt(s); ok {
		return v
	}
	if v, ok := resolveYamlFloat(s); ok {
		return v
	}
	if d, ok := resolveYamlTimestamp(s); ok {
		return d
	}
	return s
}

func resolveYamlNull(s string) bool {
	return s == "~" || s == "null" || s == "Null" || s == "NULL"
}

type boolResolution int

const (
	boolNone boolResolution = iota
	boolTrue
	boolFalse
)

// resolveYamlBool deliberately excludes yes/no/on/off: js-yaml resolves only
// the true/false spellings, so "yes" stays the string "yes".
func resolveYamlBool(s string) boolResolution {
	switch s {
	case "true", "True", "TRUE":
		return boolTrue
	case "false", "False", "FALSE":
		return boolFalse
	}
	return boolNone
}

func resolveYamlNumber(s string) (float64, bool) {
	if v, ok := resolveYamlInt(s); ok {
		return v, true
	}
	return resolveYamlFloat(s)
}

// resolveYamlInt ports resolveYamlInteger + constructYamlInteger from
// js-yaml 3.14.1, which follows YAML 1.1: a leading 0 means octal (017 is 15),
// 0o is NOT a prefix (0o17 stays a string), and colon-separated groups are
// base-60 (1:30 is 90).
func resolveYamlInt(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	index := 0
	sign := 1.0
	if s[index] == '-' || s[index] == '+' {
		if s[index] == '-' {
			sign = -1
		}
		index++
	}
	if index >= len(s) {
		return 0, false
	}
	if s[index] == '0' {
		if index+1 == len(s) {
			return 0, true
		}
		index++
		switch s[index] {
		case 'b':
			return parseRadix(s[index+1:], 2, sign)
		case 'x':
			return parseRadix(s[index+1:], 16, sign)
		}
		return parseRadix(s[index:], 8, sign)
	}
	if s[index] == '_' {
		return 0, false
	}
	digits := strings.Builder{}
	i := index
	for ; i < len(s); i++ {
		if s[i] == '_' {
			continue
		}
		if s[i] == ':' {
			break
		}
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		digits.WriteByte(s[i])
	}
	if digits.Len() == 0 || s[len(s)-1] == '_' {
		return 0, false
	}
	if i >= len(s) {
		v, err := strconv.ParseFloat(digits.String(), 64)
		if err != nil {
			return 0, false
		}
		return sign * v, true
	}
	rest := s[i:]
	if !base60Re.MatchString(rest) {
		return 0, false
	}
	value := 0.0
	base := 1.0
	parts := strings.Split(strings.ReplaceAll(s[index:], "_", ""), ":")
	for j := len(parts) - 1; j >= 0; j-- {
		p, err := strconv.ParseFloat(parts[j], 64)
		if err != nil {
			return 0, false
		}
		value += p * base
		base *= 60
	}
	return sign * value, true
}

var base60Re = regexp.MustCompile(`^(:[0-5]?[0-9])+$`)

func parseRadix(digits string, radix int, sign float64) (float64, bool) {
	clean := strings.ReplaceAll(digits, "_", "")
	if clean == "" || strings.HasSuffix(digits, "_") {
		return 0, false
	}
	value := 0.0
	for _, c := range clean {
		d := digitValue(byte(c))
		if d < 0 || d >= radix {
			return 0, false
		}
		value = value*float64(radix) + float64(d)
	}
	return sign * value, true
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// yamlFloatRe is js-yaml 3.14.1's YAML_FLOAT_PATTERN verbatim.
var yamlFloatRe = regexp.MustCompile(`^(?:[-+]?(?:0|[1-9][0-9_]*)(?:\.[0-9_]*)?(?:[eE][-+]?[0-9]+)?` +
	`|\.[0-9_]+(?:[eE][-+]?[0-9]+)?` +
	`|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*` +
	`|[-+]?\.(?:inf|Inf|INF)` +
	`|\.(?:nan|NaN|NAN))$`)

func resolveYamlFloat(s string) (float64, bool) {
	if !yamlFloatRe.MatchString(s) || strings.HasSuffix(s, "_") {
		return 0, false
	}
	value := strings.ToLower(strings.ReplaceAll(s, "_", ""))
	sign := 1.0
	if value[0] == '-' {
		sign = -1
	}
	if value[0] == '+' || value[0] == '-' {
		value = value[1:]
	}
	switch {
	case value == ".inf":
		return sign * math.Inf(1), true
	case value == ".nan":
		return math.NaN(), true
	case strings.Contains(value, ":"):
		parts := strings.Split(value, ":")
		result := 0.0
		base := 1.0
		for j := len(parts) - 1; j >= 0; j-- {
			p, err := strconv.ParseFloat(parts[j], 64)
			if err != nil {
				return 0, false
			}
			result += p * base
			base *= 60
		}
		return sign * result, true
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return sign * v, true
}

var (
	yamlDateRe      = regexp.MustCompile(`^([0-9][0-9][0-9][0-9])-([0-9][0-9])-([0-9][0-9])$`)
	yamlTimestampRe = regexp.MustCompile(`^([0-9][0-9][0-9][0-9])-([0-9][0-9]?)-([0-9][0-9]?)` +
		`(?:[Tt]|[ \t]+)([0-9][0-9]?):([0-9][0-9]):([0-9][0-9])(?:\.([0-9]*))?` +
		`(?:[ \t]*(Z|([-+])([0-9][0-9]?)(?::([0-9][0-9]))?))?$`)
)

func resolveYamlTimestamp(s string) (*JSDate, bool) {
	m := yamlDateRe.FindStringSubmatch(s)
	if m == nil {
		m = yamlTimestampRe.FindStringSubmatch(s)
	}
	if m == nil {
		return nil, false
	}
	year := atoiDefault(m[1])
	month := atoiDefault(m[2])
	day := atoiDefault(m[3])
	if len(m) == 4 || m[4] == "" {
		return NewJSDate(utcMillis(year, month, day, 0, 0, 0, 0)), true
	}
	hour := atoiDefault(m[4])
	minute := atoiDefault(m[5])
	second := atoiDefault(m[6])
	fraction := 0
	if m[7] != "" {
		frac := m[7]
		if len(frac) > 3 {
			frac = frac[:3]
		}
		for len(frac) < 3 {
			frac += "0"
		}
		fraction = atoiDefault(frac)
	}
	ms := utcMillis(year, month, day, hour, minute, second, fraction)
	if m[8] != "" && m[8] != "Z" {
		delta := float64(atoiDefault(m[10])*3600+atoiDefault(m[11])*60) * 1000
		if m[9] == "-" {
			delta = -delta
		}
		ms -= delta
	}
	return NewJSDate(ms), true
}

func atoiDefault(s string) int {
	if s == "" {
		return 0
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

func utcMillis(year, month, day, hour, minute, second, ms int) float64 {
	days := daysFromCivil(year, month, day)
	return float64(days)*86400000 +
		float64(hour)*3600000 +
		float64(minute)*60000 +
		float64(second)*1000 +
		float64(ms)
}

// daysFromCivil is Howard Hinnant's civil-from-days algorithm, giving the
// number of days between 1970-01-01 and the given proleptic Gregorian date
// without relying on time.Date's range clamping.
func daysFromCivil(y, m, d int) int {
	if m <= 2 {
		y--
	}
	era := y / 400
	if y < 0 {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}
