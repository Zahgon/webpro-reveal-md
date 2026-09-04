package jsutil

import (
	"math"
	"testing"
)

// Every expectation in this file was recorded from the corresponding
// JavaScript library running in the oracle project.

func TestDefaultsMatchesLodash(t *testing.T) {
	got := StringifyOrEmpty(Defaults(NewObject(), ObjectOf("a", 1.0), ObjectOf("a", 2.0, "b", 3.0)))
	if want := `{"a":1,"b":3}`; got != want {
		t.Errorf("defaults\n got: %s\nwant: %s", got, want)
	}

	src := NewObject()
	src.Set("a", Undef)
	src.Set("b", nil)
	got = StringifyOrEmpty(Defaults(NewObject(), src, ObjectOf("a", 9.0, "b", 9.0, "c", 9.0)))
	if want := `{"a":9,"b":null,"c":9}`; got != want {
		t.Errorf("undefined is filled but null is not\n got: %s\nwant: %s", got, want)
	}

	got = StringifyOrEmpty(Defaults(NewObject(), ObjectOf("z", 1.0, "a", 2.0), ObjectOf("b", 3.0, "z", 9.0)))
	if want := `{"z":1,"a":2,"b":3}`; got != want {
		t.Errorf("key order\n got: %s\nwant: %s", got, want)
	}
}

func TestDefaultsDeepMatchesLodash(t *testing.T) {
	cases := []struct {
		name    string
		sources []*Object
		want    string
	}{
		{
			"nested objects merge",
			[]*Object{ObjectOf("a", ObjectOf("x", 1.0)), ObjectOf("a", ObjectOf("x", 2.0, "y", 3.0), "b", 4.0)},
			`{"a":{"x":1,"y":3},"b":4}`,
		},
		{
			"arrays merge index-wise",
			[]*Object{ObjectOf("a", []any{1.0}), ObjectOf("a", []any{7.0, 8.0, 9.0})},
			`{"a":[1,8,9]}`,
		},
		{
			"empty array inherits the default",
			[]*Object{ObjectOf("a", []any{}), ObjectOf("a", []any{7.0, 8.0})},
			`{"a":[7,8]}`,
		},
		{
			"null blocks an object default",
			[]*Object{ObjectOf("a", nil), ObjectOf("a", ObjectOf("x", 1.0))},
			`{"a":null}`,
		},
		{
			"false blocks a true default",
			[]*Object{ObjectOf("a", false), ObjectOf("a", true)},
			`{"a":false}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StringifyOrEmpty(DefaultsDeep(NewObject(), tc.sources...)); got != tc.want {
				t.Fatalf("\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestParseIntJSMatchesLodash(t *testing.T) {
	cases := []struct {
		in   any
		want float64
	}{
		{"3", 3}, {"3x", 3}, {"08", 8}, {" 42", 42}, {"-7", -7},
	}
	for _, tc := range cases {
		if got := ParseIntJS(tc.in); got != tc.want {
			t.Errorf("ParseIntJS(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, in := range []any{"x", "", Undef, nil} {
		if got := ParseIntJS(in); !math.IsNaN(got) {
			t.Errorf("ParseIntJS(%v) = %v, want NaN", in, got)
		}
	}
}

func TestPickAndOmitPreserveOrder(t *testing.T) {
	src := ObjectOf("a", 1.0, "b", 2.0, "c", 3.0)
	if got, want := StringifyOrEmpty(Pick(src, []string{"c", "a", "zz"})), `{"c":3,"a":1}`; got != want {
		t.Errorf("pick\n got: %s\nwant: %s", got, want)
	}
	if got, want := StringifyOrEmpty(Omit(src, "b")), `{"a":1,"c":3}`; got != want {
		t.Errorf("omit\n got: %s\nwant: %s", got, want)
	}
}

func TestNumberToStringMatchesJS(t *testing.T) {
	// Assigned to variables so the addition happens at runtime in float64;
	// as an untyped constant expression Go would fold it to exactly 0.3.
	pointOne, pointTwo := 0.1, 0.2

	cases := []struct {
		in   float64
		want string
	}{
		{pointOne + pointTwo, "0.30000000000000004"},
		{1e21, "1e+21"},
		{1e-7, "1e-7"},
		{math.Copysign(0, -1), "0"},
		{1024, "1024"},
		{8.5, "8.5"},
		{960, "960"},
	}
	for _, tc := range cases {
		if got := NumberToString(tc.in); got != tc.want {
			t.Errorf("NumberToString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLegacyURLHostMatchesNode(t *testing.T) {
	cases := map[string]string{
		"black":             "",
		"solarized":         "",
		"white.css":         "",
		"./x.css":           "",
		"/abs/x.css":        "",
		"//cdn/x.css":       "",
		`C:\x.css`:          "",
		"https://cdn/x.css": "cdn",
		"http://a.b:8080/c": "a.b:8080",
	}
	for in, want := range cases {
		if got := LegacyURLHost(in); got != want {
			t.Errorf("LegacyURLHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsAbsoluteURLMatchesRevealMD(t *testing.T) {
	cases := map[string]bool{
		"https://x/y.css": true,
		"//cdn/x.css":     true,
		"://x":            false,
		"x.css":           false,
		"./x.css":         false,
		"/abs/x.css":      false,
	}
	for in, want := range cases {
		if got := IsAbsoluteURL(in); got != want {
			t.Errorf("IsAbsoluteURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPathHelpersMatchNode(t *testing.T) {
	relCases := []struct{ from, to, want string }{
		{"a/b", "a/b/c", "c"},
		{"a/b/c", "a", "../.."},
		{".", "sub/c.md", "sub/c.md"},
		{"sub", ".", ".."},
		{"sub/deep", ".", "../.."},
		{".", ".", ""},
		{"a", "a", ""},
		{"/a/b", "/a/b/c", "c"},
		{"/a/b/c", "/a", "../.."},
		{"/a", "/b", "../b"},
		{"/", "/a", "a"},
	}
	for _, tc := range relCases {
		if got := PathRelative(tc.from, tc.to); got != tc.want {
			t.Errorf("PathRelative(%q,%q) = %q, want %q", tc.from, tc.to, got, tc.want)
		}
	}

	joinCases := []struct {
		parts []string
		want  string
	}{
		{[]string{"a", "b"}, "a/b"},
		{[]string{"a/", "/b"}, "a/b"},
		{[]string{"", "b"}, "b"},
		{[]string{"a", ""}, "a"},
		{[]string{"a", "..", "b"}, "b"},
		{[]string{".", "x.md"}, "x.md"},
		{[]string{"a", "./b"}, "a/b"},
		{[]string{"/a/b", "c.md"}, "/a/b/c.md"},
		{[]string{"/", "a"}, "/a"},
		{[]string{"/a", ""}, "/a"},
		{[]string{"", "/b"}, "/b"},
		{[]string{"/a/", "/b"}, "/a/b"},
		{[]string{"/a", "../b"}, "/b"},
		{[]string{"/a/b", "../../c"}, "/c"},
		{[]string{"/", ".."}, "/"},
		{[]string{"/a", ".."}, "/"},
		{[]string{"/", "/"}, "/"},
		{[]string{"a", ".."}, "."},
		{[]string{"a/"}, "a/"},
		{[]string{"/a/b/"}, "/a/b/"},
		{[]string{"/a", ".", "b"}, "/a/b"},
	}
	for _, tc := range joinCases {
		if got := PathJoin(tc.parts...); got != tc.want {
			t.Errorf("PathJoin(%q) = %q, want %q", tc.parts, got, tc.want)
		}
	}

	normalizeCases := map[string]string{
		"/a/b/../c": "/a/c",
		"//a//b":    "/a/b",
		"/":         "/",
		"a/b/":      "a/b/",
		"./a":       "a",
		"../a":      "../a",
		"/../a":     "/a",
		"":          ".",
	}
	for in, want := range normalizeCases {
		if got := PathNormalize(in); got != want {
			t.Errorf("PathNormalize(%q) = %q, want %q", in, got, want)
		}
	}

	if got, want := PathBasename("a/b/c.md"), "c.md"; got != want {
		t.Errorf("basename = %q, want %q", got, want)
	}
	if got, want := PathBasenameExt("a/b/c.md", ".md"), "c"; got != want {
		t.Errorf("basename with ext = %q, want %q", got, want)
	}
	if got, want := PathBasename("/"), ""; got != want {
		t.Errorf("basename of root = %q, want %q", got, want)
	}
	if got, want := PathBasename("a/"), "a"; got != want {
		t.Errorf("basename with trailing slash = %q, want %q", got, want)
	}
	for in, want := range map[string]string{"a/b/c.md": "a/b", "c.md": ".", "/a": "/"} {
		if got := PathDirname(in); got != want {
			t.Errorf("PathDirname(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"a.md": ".md", "a": "", "a.tar.gz": ".gz", ".md": ""} {
		if got := PathExtname(in); got != want {
			t.Errorf("PathExtname(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJSTrimMatchesJS(t *testing.T) {
	if got, want := JSTrim(" \t\n\u00a0\ufeff x \u2028"), "x"; got != want {
		t.Errorf("JSTrim = %q, want %q", got, want)
	}
}

func TestEncodeURIComponentMatchesJS(t *testing.T) {
	if got, want := EncodeURIComponent("a b.md"), "a%20b.md"; got != want {
		t.Errorf("EncodeURIComponent = %q, want %q", got, want)
	}
}
