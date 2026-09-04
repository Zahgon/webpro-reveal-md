package jsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectAccessorsBehaveLikeJavaScriptObjects(t *testing.T) {
	obj := ObjectOf("a", float64(1), "b", "two")
	if obj.Len() != 2 {
		t.Errorf("Len() = %d, want 2", obj.Len())
	}
	if got, ok := obj.GetString("b"); !ok || got != "two" {
		t.Errorf(`GetString("b") = %q, %v; want "two", true`, got, ok)
	}
	if _, ok := obj.GetString("a"); ok {
		t.Error(`GetString("a") reported a string for a number`)
	}
	if _, ok := obj.GetString("missing"); ok {
		t.Error(`GetString("missing") reported a value`)
	}

	obj.Delete("a")
	if obj.Has("a") {
		t.Error(`Delete("a") left the key behind`)
	}
	if obj.Len() != 1 {
		t.Errorf("Len() after delete = %d, want 1", obj.Len())
	}

	clone := obj.Clone()
	clone.Set("b", "changed")
	if got, _ := obj.GetString("b"); got != "two" {
		t.Errorf("Clone() aliased the source: b = %q", got)
	}

	target := ObjectOf("b", "original", "c", true)
	Assign(target, ObjectOf("b", "replaced"), nil, ObjectOf("d", float64(4)))
	if got := StringifyOrEmpty(target); got != `{"b":"replaced","c":true,"d":4}` {
		t.Errorf("Assign produced %s", got)
	}
}

func TestPathIsAbsoluteMatchesNode(t *testing.T) {
	cases := map[string]bool{
		"/a/b":  true,
		"/":     true,
		"a/b":   false,
		"":      false,
		"./a":   false,
		"../a":  false,
		"//a/b": true,
	}
	for input, want := range cases {
		if got := PathIsAbsolute(input); got != want {
			t.Errorf("PathIsAbsolute(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestFlattenAndCompactMatchLodash(t *testing.T) {
	flattened := Flatten([]any{float64(1), []any{float64(2), float64(3)}, "x"})
	if got := StringifyOrEmpty(flattened); got != `[1,2,3,"x"]` {
		t.Errorf("Flatten produced %s", got)
	}

	compacted := Compact([]any{float64(0), float64(1), "", "x", nil, false, true, Undef})
	if got := StringifyOrEmpty(compacted); got != `[1,"x",true]` {
		t.Errorf("Compact produced %s", got)
	}
}

func TestDecodeURIMatchesJavaScript(t *testing.T) {
	cases := map[string]string{
		"a%20b.md":     "a b.md",
		"caf%C3%A9.md": "café.md",
		"plain.md":     "plain.md",
		"a%2Fb.md":     "a/b.md",
	}
	for input, want := range cases {
		if got := DecodeURI(input); got != want {
			t.Errorf("DecodeURI(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDecodeURIComponentStrictRejectsMalformedEscapes(t *testing.T) {
	if got, err := DecodeURIComponentStrict("a%2Fb"); err != nil || got != "a/b" {
		t.Errorf(`DecodeURIComponentStrict("a%%2Fb") = %q, %v`, got, err)
	}
	for _, malformed := range []string{"%", "%2", "%zz", "a%2"} {
		if _, err := DecodeURIComponentStrict(malformed); err == nil {
			t.Errorf("DecodeURIComponentStrict(%q) accepted a malformed escape", malformed)
		}
	}
}

func TestJSDateSerialisesLikeJavaScript(t *testing.T) {
	epoch := NewJSDate(0)
	if got := epoch.ISOString(); got != "1970-01-01T00:00:00.000Z" {
		t.Errorf("ISOString() = %q", got)
	}
	if got := NewJSDate(1704164645000).ISOString(); got != "2024-01-02T03:04:05.000Z" {
		t.Errorf("ISOString() = %q", got)
	}
	if got := StringifyOrEmpty(NewJSDate(0)); got != `"1970-01-01T00:00:00.000Z"` {
		t.Errorf("Stringify(JSDate) = %s", got)
	}
	if got := JSString(epoch); got == "" {
		t.Error("JSString(JSDate) returned an empty string")
	}
	if got := ISONow(); len(got) != len("2024-01-02T03:04:05.000Z") || !strings.HasSuffix(got, "Z") {
		t.Errorf("ISONow() = %q", got)
	}
}

func TestDebugWritesOnlyWhenTheNamespaceIsEnabled(t *testing.T) {
	for spec, want := range map[string]bool{
		"":            false,
		"other":       false,
		"-reveal-md":  false,
		"*":           true,
		"reveal-md":   true,
		"reveal-md:*": false,
		"a,reveal-md": true,
	} {
		if got := debugSpecEnables(spec); got != want {
			t.Errorf("debugSpecEnables(%q) = %v, want %v", spec, got, want)
		}
	}
	DebugEnabled()
	Debug("message", ObjectOf("key", "value"))
}

func TestStatHelpersMemoiseTheirAnswers(t *testing.T) {
	ResetStatCache()
	dir := t.TempDir()
	file := filepath.Join(dir, "slides.md")
	if err := os.WriteFile(file, []byte("# deck"), 0o644); err != nil {
		t.Fatal(err)
	}

	if ok, err := IsDirectory(dir); err != nil || !ok {
		t.Errorf("IsDirectory(dir) = %v, %v", ok, err)
	}
	if ok, err := IsFile(file); err != nil || !ok {
		t.Errorf("IsFile(file) = %v, %v", ok, err)
	}
	if ok, err := IsDirectory(file); err != nil || ok {
		t.Errorf("IsDirectory(file) = %v, %v", ok, err)
	}

	missing := filepath.Join(dir, "absent.md")
	_, err := IsFile(missing)
	if err == nil {
		t.Fatal("IsFile(missing) returned no error")
	}
	if !strings.Contains(Inspect(err), "ENOENT") {
		t.Errorf("IsFile(missing) error = %s", Inspect(err))
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsFile(file); err != nil || !ok {
		t.Error("IsFile did not memoise its answer across a deletion")
	}
	ResetStatCache()
	if _, err := IsFile(file); err == nil {
		t.Error("ResetStatCache did not clear the memoised answer")
	}
}

func TestSystemErrorReproducesNodeInspectOutput(t *testing.T) {
	_, err := os.Stat("/no/such/reveal-md/path")
	if err == nil {
		t.Skip("the impossible path exists on this machine")
	}
	sysErr := NewSystemError(err, "stat", "/no/such/reveal-md/path")
	if sysErr.Code != "ENOENT" || sysErr.Errno != -2 || sysErr.Syscall != "stat" {
		t.Errorf("NewSystemError produced %+v", sysErr)
	}
	if got := sysErr.Error(); got != "ENOENT: no such file or directory, stat '/no/such/reveal-md/path'" {
		t.Errorf("Error() = %q", got)
	}
	want := "[Error: ENOENT: no such file or directory, stat '/no/such/reveal-md/path'] {\n" +
		"  errno: -2,\n  code: 'ENOENT',\n  syscall: 'stat',\n  path: '/no/such/reveal-md/path'\n}"
	if got := sysErr.Inspect(); got != want {
		t.Errorf("Inspect() =\n%s\nwant\n%s", got, want)
	}
	if got := Inspect(sysErr); got != want {
		t.Errorf("Inspect(err) did not use the SystemError form:\n%s", got)
	}
	if got := Inspect(errors.New("plain")); got != "plain" {
		t.Errorf("Inspect(plain error) = %q", got)
	}
	if NewSystemError(sysErr, "open", "/elsewhere") != sysErr {
		t.Error("NewSystemError rewrapped an existing SystemError")
	}

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatal("os.Stat did not return a PathError")
	}
}

func TestInspectStringQuotesLikeUtilInspect(t *testing.T) {
	cases := map[string]string{
		"plain":       "'plain'",
		"it's":        `"it's"`,
		`say "hi"`:    `'say "hi"'`,
		`both ' "`:    "`both ' \"`",
		"tab\there":   `'tab\there'`,
		"line\nbreak": `'line\nbreak'`,
	}
	for input, want := range cases {
		if got := InspectString(input); got != want {
			t.Errorf("InspectString(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestPhysicalCwdResolvesSymlinks(t *testing.T) {
	cwd, err := PhysicalCwd()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cwd) {
		t.Errorf("PhysicalCwd() = %q, want an absolute path", cwd)
	}
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != cwd {
		t.Errorf("PhysicalCwd() = %q, which still contains a symlink (%q)", cwd, resolved)
	}
}

func TestParseJSON5AcceptsTheDialectExtensions(t *testing.T) {
	source := `{
		// a comment
		unquoted: 'single quotes',
		hex: 0xFF,
		positive: +1.5,
		leading: .5,
		trailing: 5.,
		infinity: Infinity,
		list: [1, 2, 3,],
		nested: { a: [true, false, null] },
		continued: 'a\
b',
		escaped: '\x41\u0042',
	}`
	value, err := ParseJSON5(source)
	if err != nil {
		t.Fatalf("ParseJSON5: %v", err)
	}
	obj, ok := value.(*Object)
	if !ok {
		t.Fatalf("ParseJSON5 returned %T", value)
	}
	if got, _ := obj.GetString("unquoted"); got != "single quotes" {
		t.Errorf("unquoted = %q", got)
	}
	if got := obj.Get("hex"); got != float64(255) {
		t.Errorf("hex = %v", got)
	}
	if got := obj.Get("positive"); got != 1.5 {
		t.Errorf("positive = %v", got)
	}
	if got := obj.Get("leading"); got != 0.5 {
		t.Errorf("leading = %v", got)
	}
	if got := obj.Get("trailing"); got != float64(5) {
		t.Errorf("trailing = %v", got)
	}
	if got, _ := obj.GetString("continued"); got != "ab" {
		t.Errorf("continued = %q", got)
	}
	if got, _ := obj.GetString("escaped"); got != "AB" {
		t.Errorf("escaped = %q", got)
	}
	if got := StringifyOrEmpty(obj.Get("list")); got != "[1,2,3]" {
		t.Errorf("list = %s", got)
	}
	if got := StringifyOrEmpty(obj.Get("nested")); got != `{"a":[true,false,null]}` {
		t.Errorf("nested = %s", got)
	}
}

func TestParseJSON5RejectsMalformedInput(t *testing.T) {
	for _, source := range []string{"{", "{a:}", "[1,", `{"a" 1}`, "0x", "'unterminated", "{a: 1} trailing"} {
		if _, err := ParseJSON5(source); err == nil {
			t.Errorf("ParseJSON5(%q) accepted malformed input", source)
		}
	}
}

func TestParseJSONRejectsTheJSON5Extensions(t *testing.T) {
	if _, err := ParseJSON(`{"a":1}`); err != nil {
		t.Errorf("ParseJSON rejected valid JSON: %v", err)
	}
	for _, source := range []string{"{a:1}", "{'a':1}", "[1,]", "{/*c*/}", "0xFF"} {
		if _, err := ParseJSON(source); err == nil {
			t.Errorf("ParseJSON(%q) accepted a JSON5 extension", source)
		}
	}
}

func TestYAMLResolvesTaggedAndNumericScalars(t *testing.T) {
	document, err := LoadFront("---\n" +
		"tagged: !!str 42\n" +
		"binary: 0b101\n" +
		"octal: 017\n" +
		"hex: 0x1F\n" +
		"sexagesimal: 1:30\n" +
		"underscored: 1_000\n" +
		"infinite: .inf\n" +
		"taggedInt: !!int 0x1F\n" +
		"taggedFloat: !!float 1.5\n" +
		"taggedBool: !!bool True\n" +
		"taggedNull: !!null ~\n" +
		"unparsable: !!int nope\n" +
		"---\nbody\n")
	if err != nil {
		t.Fatalf("LoadFront: %v", err)
	}
	if got, _ := document.GetString("tagged"); got != "42" {
		t.Errorf("tagged = %q, want the string 42", got)
	}
	if got, _ := document.GetString("unparsable"); got != "nope" {
		t.Errorf("unparsable = %q, want the untouched string", got)
	}
	if got := document.Get("taggedBool"); got != true {
		t.Errorf("taggedBool = %v, want true", got)
	}
	if got := document.Get("taggedNull"); got != nil {
		t.Errorf("taggedNull = %v, want nil", got)
	}
	for key, want := range map[string]float64{
		"binary": 5, "octal": 15, "hex": 31, "sexagesimal": 90, "underscored": 1000,
		"taggedInt": 31, "taggedFloat": 1.5,
	} {
		if got := document.Get(key); got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if got := StringifyOrEmpty(document.Get("infinite")); got != "null" {
		t.Errorf("infinite serialised as %s, want null", got)
	}
}
