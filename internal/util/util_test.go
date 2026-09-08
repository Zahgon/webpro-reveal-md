package util

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
)

func fixturesDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(filepath.Dir(wd)), "testdata", "fixtures")
}

func filesGlob(t *testing.T) string {
	t.Helper()
	cfg, err := config.Load(nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cfg.FilesGlob()
}

func assertSameFiles(t *testing.T, expected, actual []string) {
	t.Helper()
	sort.Strings(expected)
	sort.Strings(actual)
	if len(expected) != len(actual) {
		t.Fatalf("got %v, want %v", actual, expected)
	}
	for i := range expected {
		if expected[i] != actual[i] {
			t.Fatalf("got %v, want %v", actual, expected)
		}
	}
}

func TestShouldListAllMarkdownFilesWithDefaultConfig(t *testing.T) {
	expected := []string{"a.md", "slides.md", "sub/c.md", "sub/slides.md"}
	actual, err := GetFilePaths(fixturesDir(t), filesGlob(t))
	if err != nil {
		t.Fatal(err)
	}
	assertSameFiles(t, expected, actual)
}

func TestShouldListOnlyFilesMatchingTheGlobPattern(t *testing.T) {
	expected := []string{"slides.md", "sub/slides.md"}
	actual, err := GetFilePaths(fixturesDir(t), "**/slides.md")
	if err != nil {
		t.Fatal(err)
	}
	assertSameFiles(t, expected, actual)
}

func TestParseYamlFrontMatterSplitsOptionsFromMarkdown(t *testing.T) {
	options, markdown, err := ParseYamlFrontMatter("---\ntitle: Deck\ntheme: moon\n---\nSlide one\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := jsutil.StringifyOrEmpty(options); got != `{"title":"Deck","theme":"moon"}` {
		t.Errorf("options = %s", got)
	}
	if markdown != "\nSlide one\n" {
		t.Errorf("markdown = %q", markdown)
	}
}

func TestParseYamlFrontMatterStripsTheByteOrderMark(t *testing.T) {
	options, markdown, err := ParseYamlFrontMatter("\ufeff---\ntitle: Deck\n---\nBody\n")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := options.GetString("title"); got != "Deck" {
		t.Errorf("title = %q", got)
	}
	if markdown != "\nBody\n" {
		t.Errorf("markdown = %q", markdown)
	}
}

func TestParseYamlFrontMatterKeepsPlainMarkdownIntact(t *testing.T) {
	options, markdown, err := ParseYamlFrontMatter("# No front matter\n")
	if err != nil {
		t.Fatal(err)
	}
	if options.Len() != 0 {
		t.Errorf("options = %s", jsutil.StringifyOrEmpty(options))
	}
	if markdown != "# No front matter\n" {
		t.Errorf("markdown = %q", markdown)
	}
}

func TestParseYamlFrontMatterReportsBrokenYaml(t *testing.T) {
	if _, _, err := ParseYamlFrontMatter("---\ntitle: Deck\ntitle: Duplicate\n---\nBody\n"); err == nil {
		t.Fatal("expected duplicate keys to be rejected")
	}
}
