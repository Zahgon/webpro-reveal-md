package assets

import (
	"io/fs"
	"strings"
	"testing"
)

func TestOriginNamesTheEmbeddedRevealVersion(t *testing.T) {
	if got, want := Origin(), "embedded reveal.js "+RevealVersion; got != want {
		t.Errorf("Origin() = %q, want %q", got, want)
	}
}

func TestVersionsMatchTheVendoredPackages(t *testing.T) {
	for name, version := range map[string]string{
		"reveal.js":    RevealVersion,
		"mermaid":      MermaidVersion,
		"highlight.js": HighlightVersion,
	} {
		if version == "" {
			t.Errorf("%s version is empty", name)
		}
	}
}

func TestRevealExposesDistAndPlugin(t *testing.T) {
	reveal := Reveal()
	for _, name := range []string{"dist/reveal.js", "dist/reveal.css", "plugin/markdown/markdown.js", "plugin/highlight/highlight.js"} {
		if _, err := fs.Stat(reveal, name); err != nil {
			t.Errorf("Reveal() is missing %s: %v", name, err)
		}
	}
}

func TestSubFilesystemsAreRootedAtTheirPackage(t *testing.T) {
	cases := []struct {
		name  string
		fsys  fs.FS
		entry string
	}{
		{"RevealDist", RevealDist(), "reveal.js"},
		{"RevealPlugin", RevealPlugin(), "markdown/markdown.js"},
		{"HighlightStyles", HighlightStyles(), "base16/zenburn.css"},
		{"Mermaid", Mermaid(), "dist/mermaid.min.js"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := fs.Stat(tc.fsys, tc.entry); err != nil {
				t.Errorf("%s() is missing %s: %v", tc.name, tc.entry, err)
			}
		})
	}
}

func TestThemeNamesListsTheBundledThemes(t *testing.T) {
	names, err := ThemeNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("ThemeNames() returned nothing")
	}
	want := map[string]bool{"black.css": false, "white.css": false, "moon.css": false, "solarized.css": false}
	for _, name := range names {
		if _, ok := want[name]; ok {
			want[name] = true
		}
		if strings.Contains(name, "/") {
			t.Errorf("ThemeNames() returned a path, not a basename: %q", name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("ThemeNames() is missing %s", name)
		}
	}
}

func TestHighlightStylesContainsTheDefaultTheme(t *testing.T) {
	data, err := fs.ReadFile(HighlightStyles(), "base16/zenburn.css")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("base16/zenburn.css is empty")
	}
}
