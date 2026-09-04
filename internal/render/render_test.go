package render

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

// emptyCwd gives each test a directory with no reveal-md.json or reveal.json,
// matching the source project's own test environment.
func emptyCwd(t *testing.T) string {
	t.Helper()
	jsutil.ResetStatCache()
	return t.TempDir()
}

func renderWith(t *testing.T, cwd, markdown string, extra *jsutil.Object) string {
	t.Helper()
	cfg, err := config.Load(nil, cwd)
	if err != nil {
		t.Fatal(err)
	}
	markup, err := Render(cfg, markdown, extra)
	if err != nil {
		t.Fatal(err)
	}
	return markup
}

func assertContains(t *testing.T, actual, want string) {
	t.Helper()
	if !strings.Contains(actual, want) {
		t.Errorf("expected output to contain %q", want)
	}
}

func assertMatches(t *testing.T, actual, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(actual) {
		t.Errorf("expected output to match %q", pattern)
	}
}

func TestShouldRenderBasicTemplate(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "", jsutil.NewObject())
	assertContains(t, actual, "<title>reveal-md</title>")
	assertContains(t, actual, `<link rel="stylesheet" href="/dist/theme/black.css"`)
	assertContains(t, actual, `<link rel="stylesheet" href="/css/highlight/base16/zenburn.css"`)
	assertMatches(t, actual, `<section data-markdown data-separator="\\r\?\\n---\\r\?\\n" data-separator-vertical="\\r\?\\n----\\r\?\\n">\s*<textarea data-template>\s*</textarea>\s*</section>`)
	assertContains(t, actual, `<script src="/dist/reveal.js"></script>`)
	assertContains(t, actual, `<script src="/plugin/markdown/markdown.js"></script>`)
	assertContains(t, actual, `var options = extend(defaultOptions, {"_":[]}, queryOptions);`)
}

func TestShouldRenderMarkdownContent(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "# header", jsutil.NewObject())
	assertMatches(t, actual, `<section data-markdown.*?>\s*<textarea data-template>\s*# header\s*</textarea>\s*</section>`)
}

func TestShouldRenderCustomScripts(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "# header", jsutil.ObjectOf(
		"scripts", "custom.js,also.js,http://example.org/script.js",
		"base", ".",
	))
	assertContains(t, actual, `<script src="./_assets/custom.js"></script>`)
	assertContains(t, actual, `<script src="./_assets/also.js"></script>`)
	assertContains(t, actual, `<script src="http://example.org/script.js"></script>`)
}

func TestShouldRenderCustomCSSAfterTheme(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "# header", jsutil.ObjectOf(
		"css", "style1.css,style2.css,http://example.org/style.css",
	))
	themeLink := `<link rel="stylesheet" href="/css/highlight/base16/zenburn.css" />`
	style1Link := `<link rel="stylesheet" href="/_assets/style1.css" />`
	style2Link := `<link rel="stylesheet" href="/_assets/style2.css" />`
	style3Link := `<link rel="stylesheet" href="http://example.org/style.css" />`
	assertContains(t, actual, themeLink)
	assertContains(t, actual, style1Link)
	assertContains(t, actual, style2Link)
	assertContains(t, actual, style3Link)
	if !(strings.Index(actual, style1Link) > strings.Index(actual, themeLink)) {
		t.Error("expected first custom stylesheet after the highlight theme")
	}
	if !(strings.Index(actual, style2Link) > strings.Index(actual, style1Link)) {
		t.Error("expected custom stylesheets in order")
	}
}

func TestShouldRenderAlternateThemeStylesheet(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "", jsutil.ObjectOf("theme", "white"))
	assertContains(t, actual, `<link rel="stylesheet" href="/dist/theme/white.css"`)
}

func TestShouldRenderRemoteThemeStylesheet(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "", jsutil.ObjectOf("theme", "https://example.org/style.css"))
	assertContains(t, actual, `<link rel="stylesheet" href="https://example.org/style.css"`)
}

func TestShouldRenderRootBasedDomainLessLinks(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "", jsutil.ObjectOf("static", true, "base", "."))
	if got := len(regexp.MustCompile(`href="\./`).FindAllString(actual, -1)); got != 5 {
		t.Errorf(`href="./ count = %d, want 5`, got)
	}
	if got := len(regexp.MustCompile(`src="\./`).FindAllString(actual, -1)); got != 7 {
		t.Errorf(`src="./ count = %d, want 7`, got)
	}
}

func TestShouldRenderRevealOptions(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "", jsutil.ObjectOf(
		"revealOptions", jsutil.ObjectOf("controls", false),
	))
	assertContains(t, actual, `var options = extend(defaultOptions, {"controls":false,"_":[]}, queryOptions);`)
}

func TestShouldRenderTitleFromYamlFrontMatter(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "---\ntitle: Foo Bar\n---\nSlide", jsutil.NewObject())
	assertMatches(t, actual, `<title>Foo Bar</title>`)
}

func TestShouldParseYamlFrontMatter(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "---\nseparator: <!--s-->\n---\nSlide A<!--s-->Slide B", nil)
	assertMatches(t, actual, `<section data-markdown data-separator="<!--s-->" .*?>\s*<textarea data-template>\s*Slide A<!--s-->Slide B\s*</textarea>\s*</section>`)
}

func TestShouldRenderOpenGraphMetadata(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "", jsutil.ObjectOf(
		"absoluteUrl", "http://example.com",
		"title", "Foo Bar",
	))
	assertContains(t, actual, `<meta property="og:title" content="Foo Bar" />`)
	assertContains(t, actual, `<meta property="og:image" content="http://example.com/featured-slide.jpg" />`)
}

func writeNodePreprocessor(t *testing.T, cwd string) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata", "preproc.js"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, "test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "test", "preproc.js"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	return "./test/preproc.js"
}

// writePortablePreprocessor produces the same slide splitting as the original's
// preproc.js without Node, so the preprocessor contract stays pinned on hosts
// that have no JavaScript runtime.
func writePortablePreprocessor(t *testing.T, cwd string) string {
	t.Helper()
	script := "#!/bin/sh\nawk 'NR>1 && /^#/ {print \"\\n---\\n\"} {print}'\n"
	if err := os.WriteFile(filepath.Join(cwd, "preproc.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return "./preproc.sh"
}

func TestShouldUsePreprocessorForMarkdown(t *testing.T) {
	cwd := emptyCwd(t)
	var preprocessor string
	switch _, err := exec.LookPath("node"); {
	case err == nil:
		preprocessor = writeNodePreprocessor(t, cwd)
	case runtime.GOOS == "windows":
		t.Skip("no preprocessor interpreter is available on this host")
	default:
		preprocessor = writePortablePreprocessor(t, cwd)
	}
	actual := renderWith(t, cwd, "# Slide A\n\ncontent\n\n# Slide B\n\ncontent", jsutil.ObjectOf(
		"preprocessor", preprocessor,
	))
	assertMatches(t, actual, `<section data-markdown.*?>\s*<textarea data-template>\s*# Slide A\s+content\s+---\s+# Slide B\s*content\s*</textarea>\s*</section>`)
}

func TestShouldUseExecutablePreprocessorWithoutNode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shebang scripts are not executable on Windows")
	}
	cwd := emptyCwd(t)
	script := "#!/bin/sh\nsed 's/^# /## /'\n"
	if err := os.WriteFile(filepath.Join(cwd, "pre.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	actual := renderWith(t, cwd, "# header", jsutil.ObjectOf("preprocessor", "./pre.sh"))
	assertContains(t, actual, "## header")
}

func TestShouldMergeRevealOptionsFromFrontMatterAndLocalOptions(t *testing.T) {
	revealOptions := jsutil.ObjectOf("height", float64(100), "transition", "none")
	actual := renderWith(t, emptyCwd(t),
		"---\nrevealOptions:\n  width: 300\n  height: 500\n---\nSlide",
		jsutil.ObjectOf("revealOptions", revealOptions))
	expected := `{"height":500,"transition":"none","_":[],"width":300}`
	assertContains(t, actual, `var options = extend(defaultOptions, `+expected+`, queryOptions);`)
}

func TestShouldRenderCorrectFavicon(t *testing.T) {
	actual := renderWith(t, emptyCwd(t), "", jsutil.ObjectOf("static", true, "base", "."))
	assertContains(t, actual, `<link rel="shortcut icon" href="./favicon.ico" />`)
}

func TestSanitizeStripsParentSegmentsRepeatedly(t *testing.T) {
	cases := map[string]string{
		"/deck/slides.md": "/deck/slides.md",
		"/../etc/passwd":  "//etc/passwd",
		"/a/../../b":      "/a///b",
		"...":             ".",
		"....":            "",
		"":                "",
	}
	for input, want := range cases {
		if got := Sanitize(input); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRenderFileRendersADeckFromDisk(t *testing.T) {
	cwd := emptyCwd(t)
	deck := filepath.Join(cwd, "slides.md")
	if err := os.WriteFile(deck, []byte("---\ntitle: On Disk\n---\n# Heading\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(nil, cwd)
	if err != nil {
		t.Fatal(err)
	}
	markup, err := RenderFile(cfg, deck, jsutil.ObjectOf("base", "."))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, markup, "<title>On Disk</title>")
	assertContains(t, markup, "# Heading")
	assertContains(t, markup, `href="./dist/theme/black.css"`)
}

func TestRenderFileFallsBackWhenTheDeckIsMissing(t *testing.T) {
	cwd := emptyCwd(t)
	cfg, err := config.Load(nil, cwd)
	if err != nil {
		t.Fatal(err)
	}
	markup, err := RenderFile(cfg, filepath.Join(cwd, "absent.md"), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, markup, "File not found.")
}
