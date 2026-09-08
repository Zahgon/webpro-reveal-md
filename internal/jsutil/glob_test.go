package jsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchGlobMatchesMinimatch(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.md", "a.md", true},
		{"**/*.md", "sub/c.md", true},
		{"**/*.md", "sub/deep/d.md", true},
		{"**/*.md", "a.markdown", false},
		{"*.md", "sub/c.md", false},
		{"**/slides.md", "slides.md", true},
		{"**/slides.md", "sub/slides.md", true},
		{"**/slides.md", "sub/c.md", false},
		{"sub/*.md", "sub/c.md", true},
		{"sub/**", "sub/deep/d.md", true},
		{"**/*.md", ".hidden.md", false},
		{"**/*.md", ".config/x.md", false},
		{".*/*.md", ".config/x.md", true},
		{"?.md", "a.md", true},
		{"?.md", "ab.md", false},
		{"[ab].md", "a.md", true},
		{"[ab].md", "c.md", false},
		{"[!ab].md", "c.md", true},
		{"{a,b}.md", "b.md", true},
		{"{a,b}.md", "c.md", false},
		{"**/*.{md,markdown}", "sub/c.markdown", true},
		{"**/node_modules/**", "node_modules/x/y.md", true},
		{"**/node_modules/**", "a/node_modules/x.md", true},
		{"**/node_modules/**", "a/b.md", false},
	}
	for _, tc := range cases {
		if got := MatchGlob(tc.pattern, tc.path); got != tc.want {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestGlobSyncMatchesGlobPackage(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"a.md",
		"slides.md",
		"sub/c.md",
		"sub/slides.md",
		"sub/deep/d.md",
		"sub/cat.jpg",
		".hidden/secret.md",
		"node_modules/pkg/readme.md",
	}
	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	opts := GlobOptions{Cwd: root, Ignore: []string{"**/node_modules/**"}}

	got, err := GlobSync("**/*.md", opts)
	if err != nil {
		t.Fatal(err)
	}
	want := "a.md,slides.md,sub/c.md,sub/deep/d.md,sub/slides.md"
	if strings.Join(got, ",") != want {
		t.Errorf("GlobSync(**/*.md) = %v, want %v", got, want)
	}

	got, err = GlobSync("**/slides.md", opts)
	if err != nil {
		t.Fatal(err)
	}
	if want := "slides.md,sub/slides.md"; strings.Join(got, ",") != want {
		t.Errorf("GlobSync(**/slides.md) = %v, want %v", got, want)
	}
}

func TestLocaleCompareMatchesNode(t *testing.T) {
	input := []string{"b.md", "A.md", "a.md", "Z.md", "á.md", "10.md", "2.md", "_x.md", "B.md"}
	want := "_x.md,10.md,2.md,a.md,A.md,á.md,b.md,B.md,Z.md"
	SortLocale(input)
	if got := strings.Join(input, ","); got != want {
		t.Errorf("SortLocale = %v, want %v", got, want)
	}
}
