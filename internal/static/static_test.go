package static

import (
	"os"
	"path/filepath"
	"sort"
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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	target := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func treeOf(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// chdir mirrors how the CLI runs: the exporter resolves the static directory
// and every source file against the process working directory, so a test that
// only points the config at a temp dir would still write into the package dir.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRelativeDirReplacesOnlyTheFirstParentSegment(t *testing.T) {
	cases := []struct{ from, to, want string }{
		{"a/b/c.md", ".", "./../.."},
		{"c.md", ".", "."},
		{"a/c.md", ".", "./.."},
		{".", "sub", "sub"},
		{"sub", ".", "."},
		{"sub/deep", ".", "./.."},
	}
	for _, tc := range cases {
		if got := relativeDir(tc.from, tc.to); got != tc.want {
			t.Errorf("relativeDir(%q, %q) = %q, want %q", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestImagePathsCollectsEveryReferenceInSourceOrder(t *testing.T) {
	markdown := "![alt](one.png)\n" +
		"<img src=\"two.png\" width=\"10\">\n" +
		"<!-- .slide: data-background-image=\"three.png\" -->\n" +
		"![remote](https://example.org/four.png)\n"
	want := []string{"one.png", "https://example.org/four.png", "two.png", "three.png"}
	got := imagePaths(markdown)
	if len(got) != len(want) {
		t.Fatalf("imagePaths returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("imagePaths[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExportWritesTheWholeStaticSite(t *testing.T) {
	cwd := t.TempDir()
	chdir(t, cwd)
	writeFile(t, cwd, "slides.md", "---\ntitle: Deck\n---\n# Hello\n")
	writeFile(t, cwd, "sub/nested.md", "# Nested\n")
	cfg := loadConfig(t, cwd, ".", "--static", "_out")

	if err := Export(cfg); err != nil {
		t.Fatal(err)
	}

	names := treeOf(t, filepath.Join(cwd, "_out"))
	for _, want := range []string{
		"index.html",
		"slides.html",
		"sub/nested.html",
		"favicon.ico",
		"dist/reveal.js",
		"dist/theme/black.css",
		"plugin/markdown/markdown.js",
		"css/highlight/base16/zenburn.css",
	} {
		if !contains(names, want) {
			t.Errorf("static export is missing %s", want)
		}
	}

	markup, err := os.ReadFile(filepath.Join(cwd, "_out", "slides.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markup), "<title>Deck</title>") {
		t.Error("expected the front-matter title in the exported deck")
	}
	if !strings.Contains(string(markup), `href="./dist/theme/black.css"`) {
		t.Error("expected relative asset paths in the exported deck")
	}

	listing, err := os.ReadFile(filepath.Join(cwd, "_out", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(listing), `href="slides.html"`) {
		t.Error("expected the listing to link the exported deck")
	}
}

func TestExportCopiesImagesAndSkipsRemoteOnes(t *testing.T) {
	cwd := t.TempDir()
	chdir(t, cwd)
	writeFile(t, cwd, "slides.md", "![cat](assets/cat.png)\n![remote](https://example.org/x.png)\n")
	writeFile(t, cwd, "assets/cat.png", "not really a png")
	cfg := loadConfig(t, cwd, ".", "--static", "_out")

	if err := Export(cfg); err != nil {
		t.Fatal(err)
	}

	copied, err := os.ReadFile(filepath.Join(cwd, "_out", "assets", "cat.png"))
	if err != nil {
		t.Fatalf("expected the local image to be copied: %v", err)
	}
	if string(copied) != "not really a png" {
		t.Error("copied image does not match the source bytes")
	}
	if contains(treeOf(t, filepath.Join(cwd, "_out")), "x.png") {
		t.Error("a remote image must not be copied into the export")
	}
}

func TestExportOfASingleFileAlsoWritesIndexHTML(t *testing.T) {
	cwd := t.TempDir()
	chdir(t, cwd)
	writeFile(t, cwd, "slides.md", "# Only deck\n")
	cfg := loadConfig(t, cwd, "slides.md", "--static", "_out")

	if err := Export(cfg); err != nil {
		t.Fatal(err)
	}

	deck, err := os.ReadFile(filepath.Join(cwd, "_out", "slides.html"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(cwd, "_out", "index.html"))
	if err != nil {
		t.Fatalf("a single-file export must also produce index.html: %v", err)
	}
	if string(deck) != string(index) {
		t.Error("index.html must be a copy of the exported deck")
	}
}

func TestExportCopiesConfiguredAssetsAndStaticDirs(t *testing.T) {
	cwd := t.TempDir()
	chdir(t, cwd)
	writeFile(t, cwd, "slides.md", "# Deck\n")
	writeFile(t, cwd, "extra.css", "body { color: red }")
	writeFile(t, cwd, "extra.js", "console.log('hi')")
	writeFile(t, cwd, "media/logo.svg", "<svg/>")
	cfg := loadConfig(t, cwd, ".", "--static", "_out",
		"--css", "extra.css", "--scripts", "extra.js", "--static-dirs", "media")

	if err := Export(cfg); err != nil {
		t.Fatal(err)
	}

	names := treeOf(t, filepath.Join(cwd, "_out"))
	for _, want := range []string{"_assets/extra.css", "_assets/extra.js", "media/logo.svg"} {
		if !contains(names, want) {
			t.Errorf("static export is missing %s", want)
		}
	}
}

func TestQueueDeduplicatesByTarget(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "one.txt", "one")
	e := &exporter{cfg: loadConfig(t, cwd, "."), staticDir: filepath.Join(cwd, "_out"), written: map[string]bool{}}

	target := filepath.Join(cwd, "_out", "one.txt")
	first := e.queue(source(filepath.Join(cwd, "one.txt"), cwd), target, "one.txt")
	second := e.queue(source(filepath.Join(cwd, "one.txt"), cwd), target, "one.txt")

	if len(first) != 1 {
		t.Fatalf("first queue returned %d operations, want 1", len(first))
	}
	if len(second) != 0 {
		t.Errorf("second queue returned %d operations, want 0", len(second))
	}
}

func TestRunCopiesReportsAMissingSourceLikeFsExtra(t *testing.T) {
	cwd := t.TempDir()
	e := &exporter{cfg: loadConfig(t, cwd, "."), staticDir: filepath.Join(cwd, "_out"), written: map[string]bool{}}

	ops := e.queue(source(filepath.Join(cwd, "absent.css"), cwd), filepath.Join(cwd, "_out", "absent.css"), "absent.css")
	err := e.runCopies(ops)
	if err == nil {
		t.Fatal("expected a missing asset to fail the copy")
	}
	if got := jsutil.Inspect(err); !strings.Contains(got, "ENOENT") || !strings.Contains(got, "lstat 'absent.css'") {
		t.Errorf("error text %q does not match fs-extra's lstat failure", got)
	}
}
