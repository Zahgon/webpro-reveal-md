package jsutil

import "testing"

func render(t *testing.T, tmpl string, ctx *Object) string {
	t.Helper()
	out, err := RenderMustache(tmpl, ctx)
	if err != nil {
		t.Fatalf("RenderMustache(%q) error: %v", tmpl, err)
	}
	return out
}

// Expectations recorded from mustache 4.2.0 in the oracle project.
func TestMustacheEscaping(t *testing.T) {
	ctx := ObjectOf("x", `&<>"'`+"`"+`=/`)
	if got, want := render(t, "{{x}}", ctx), "&amp;&lt;&gt;&quot;&#39;&#x60;&#x3D;&#x2F;"; got != want {
		t.Errorf("escaped\n got: %s\nwant: %s", got, want)
	}
	if got, want := render(t, "{{{x}}}", ctx), `&<>"'`+"`"+`=/`; got != want {
		t.Errorf("unescaped\n got: %s\nwant: %s", got, want)
	}
}

func TestMustacheValueConversion(t *testing.T) {
	ctx := NewObject()
	ctx.Set("zero", 0.0)
	ctx.Set("f", false)
	ctx.Set("empty", "")
	ctx.Set("nul", nil)
	ctx.Set("num", 8.5)
	ctx.Set("big", 1e21)
	ctx.Set("small", 1e-7)
	ctx.Set("arr", []any{1.0, nil, "x", Undef})
	ctx.Set("obj", ObjectOf("a", 1.0))

	cases := map[string]string{
		"[{{nul}}][{{missing}}][{{empty}}][{{zero}}][{{f}}]": "[][][][0][false]",
		"{{num}}|{{big}}|{{small}}":                          "8.5|1e+21|1e-7",
		"{{arr}}":                                            "1,,x,",
		"{{obj}}":                                            "[object Object]",
	}
	for tmpl, want := range cases {
		if got := render(t, tmpl, ctx); got != want {
			t.Errorf("%q\n got: %s\nwant: %s", tmpl, got, want)
		}
	}
}

func TestMustacheStandaloneLines(t *testing.T) {
	ctx := ObjectOf("s", true)
	cases := []struct{ tmpl, want string }{
		{"A\n{{#s}}\nB\n{{/s}}\nC\n", "A\nB\nC\n"},
		{"A\n  {{#s}}  \nB\n  {{/s}}  \nC", "A\nB\nC"},
		{"A {{#s}}B{{/s}} C", "A B C"},
		{"A\r\n{{#s}}\r\nB\r\n{{/s}}\r\nC", "A\r\nB\r\nC"},
		{"A\n{{! hi }}\nB", "A\nB"},
	}
	for _, tc := range cases {
		if got := render(t, tc.tmpl, ctx); got != tc.want {
			t.Errorf("%q\n got: %q\nwant: %q", tc.tmpl, got, tc.want)
		}
	}
}

func TestMustacheSections(t *testing.T) {
	empty := ObjectOf("a", []any{})
	one := ObjectOf("a", []any{1.0})

	if got, want := render(t, "{{^a}}[yes]{{/a}}", empty), "[yes]"; got != want {
		t.Errorf("inverted over empty array: got %q want %q", got, want)
	}
	if got, want := render(t, "{{^a}}[yes]{{/a}}", one), ""; got != want {
		t.Errorf("inverted over non-empty array: got %q want %q", got, want)
	}
	if got, want := render(t, "{{^a}}[yes]{{/a}}", ObjectOf("a", 0.0)), "[yes]"; got != want {
		t.Errorf("inverted over 0: got %q want %q", got, want)
	}
	if got, want := render(t, "{{^a}}[yes]{{/a}}", ObjectOf("a", "")), "[yes]"; got != want {
		t.Errorf("inverted over empty string: got %q want %q", got, want)
	}
	if got, want := render(t, "{{#a}}[{{.}}]{{/a}}", ObjectOf("a", "str")), "[str]"; got != want {
		t.Errorf("scalar section: got %q want %q", got, want)
	}
	if got, want := render(t, "{{#a}}[{{b}}]{{/a}}", ObjectOf("a", ObjectOf("b", 1.0))), "[1]"; got != want {
		t.Errorf("object section: got %q want %q", got, want)
	}
}

func TestMustacheDottedLookup(t *testing.T) {
	ctx := ObjectOf("a", ObjectOf("b", ObjectOf("c", 5.0)))
	if got, want := render(t, "{{a.b.c}}", ctx), "5"; got != want {
		t.Errorf("dotted: got %q want %q", got, want)
	}
	if got, want := render(t, "[{{a.zz.c}}]", ctx), "[]"; got != want {
		t.Errorf("missing intermediate: got %q want %q", got, want)
	}
}
