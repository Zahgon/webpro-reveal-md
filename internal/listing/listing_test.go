package listing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
)

func loadConfig(t *testing.T, cwd string, argv ...string) *config.Config {
	t.Helper()
	jsutil.ResetStatCache()
	cfg, err := config.Load(argv, cwd)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeDeck(t *testing.T, dir, name, content string) {
	t.Helper()
	target := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFileMetaReadsFrontMatterFromTheMarkdownSibling(t *testing.T) {
	cwd := t.TempDir()
	writeDeck(t, cwd, "slides.md", "---\ntitle: Foo Bar\ntheme: moon\n---\nSlide")
	cfg := loadConfig(t, cwd, ".")

	meta := fileMeta(cfg, "slides.html")

	keys := meta.Keys()
	if len(keys) < 3 || keys[0] != "filePath" || keys[1] != "fileName" || keys[2] != "absPath" {
		t.Fatalf("expected filePath, fileName, absPath first, got %v", keys)
	}
	if got, _ := meta.GetString("filePath"); got != "slides.html" {
		t.Errorf("filePath = %q, want slides.html", got)
	}
	if got, _ := meta.GetString("fileName"); got != "slides.html" {
		t.Errorf("fileName = %q, want slides.html", got)
	}
	if got, _ := meta.GetString("title"); got != "Foo Bar" {
		t.Errorf("title = %q, want Foo Bar", got)
	}
	if got, _ := meta.GetString("theme"); got != "moon" {
		t.Errorf("theme = %q, want moon", got)
	}
}

// A missing sibling is reported and swallowed, exactly like the original's
// try/catch around getFileMeta, so the listing still renders.
func TestFileMetaKeepsGoingWhenTheMarkdownIsMissing(t *testing.T) {
	cwd := t.TempDir()
	cfg := loadConfig(t, cwd, ".")

	meta := fileMeta(cfg, "gone.html")

	if got, _ := meta.GetString("fileName"); got != "gone.html" {
		t.Errorf("fileName = %q, want gone.html", got)
	}
	if meta.Has("title") {
		t.Error("expected no title when the markdown cannot be read")
	}
}

func TestSortByFileNameUsesLocaleCompare(t *testing.T) {
	metas := []*jsutil.Object{
		jsutil.ObjectOf("fileName", "b.md"),
		jsutil.ObjectOf("fileName", "A.md"),
		jsutil.ObjectOf("fileName", "a.md"),
		jsutil.ObjectOf("fileName", "10.md"),
		jsutil.ObjectOf("fileName", "2.md"),
		jsutil.ObjectOf("fileName", "_x.md"),
	}

	sortByFileName(metas)

	want := []string{"_x.md", "10.md", "2.md", "a.md", "A.md", "b.md"}
	for i, expected := range want {
		if got, _ := metas[i].GetString("fileName"); got != expected {
			t.Errorf("position %d = %q, want %q", i, got, expected)
		}
	}
}

func TestRenderListFileListsEveryDeckInCollationOrder(t *testing.T) {
	cwd := t.TempDir()
	writeDeck(t, cwd, "b.md", "---\ntitle: Bravo\n---\nB")
	writeDeck(t, cwd, "a.md", "---\ntitle: Alpha\n---\nA")
	cfg := loadConfig(t, cwd, ".")

	markup, err := RenderListFile(cfg, []string{"b.html", "a.html"})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{`href="a.html"`, `href="b.html"`, "Alpha", "Bravo", "Last update:"} {
		if !strings.Contains(markup, want) {
			t.Errorf("expected listing to contain %q", want)
		}
	}
	if strings.Index(markup, "Alpha") > strings.Index(markup, "Bravo") {
		t.Error("expected Alpha before Bravo")
	}
	if !strings.Contains(markup, `./dist/theme/black.css`) {
		t.Error("expected the listing to link the default theme relative to itself")
	}
}

func TestRenderWalksTheInitialDirectory(t *testing.T) {
	cwd := t.TempDir()
	writeDeck(t, cwd, "top.md", "Top")
	writeDeck(t, cwd, "sub/nested.md", "Nested")
	writeDeck(t, cwd, "notes.txt", "ignored")
	cfg := loadConfig(t, cwd, ".")

	markup, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"top.md", "sub&#x2F;nested.md"} {
		if !strings.Contains(markup, want) {
			t.Errorf("expected listing to contain %q", want)
		}
	}
	if strings.Contains(markup, "notes.txt") {
		t.Error("expected non-markdown files to be excluded")
	}
}

func TestRenderListFileUsesTheConfiguredTitleAndTheme(t *testing.T) {
	cwd := t.TempDir()
	writeDeck(t, cwd, "slides.md", "Slide")
	cfg := loadConfig(t, cwd, ".", "--title", "My Decks", "--theme", "moon")

	markup, err := RenderListFile(cfg, []string{"slides.html"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(markup, "<title>My Decks</title>") {
		t.Error("expected the configured title")
	}
	if !strings.Contains(markup, "./dist/theme/moon.css") {
		t.Error("expected the configured theme")
	}
}
