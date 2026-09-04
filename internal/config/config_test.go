package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webpro/reveal-md/internal/jsutil"
)

func newConfig(t *testing.T, cwd string, argv ...string) *Config {
	t.Helper()
	jsutil.ResetStatCache()
	cfg, err := Load(argv, cwd)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	target := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestDefaultsAreTheShippedOnes(t *testing.T) {
	cfg := newConfig(t, t.TempDir())

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"path", cfg.Path(), "."},
		{"assetsDir", cfg.AssetsDir(), "_assets"},
		{"host", cfg.Host(), "localhost"},
		{"port", cfg.Port(), "1948"},
		{"glob", cfg.FilesGlob(), "**/*.md"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if got := cfg.StaticDir(); got != "undefined" {
		t.Errorf("StaticDir() without --static = %q, want the original's undefined", got)
	}
	if cfg.HasStatic() {
		t.Error("HasStatic() = true without --static")
	}
	if cfg.Watch() {
		t.Error("Watch() = true without --watch")
	}
}

func TestCommandLineOverridesTheDefaults(t *testing.T) {
	cwd := t.TempDir()
	cfg := newConfig(t, cwd, "deck", "--host", "0.0.0.0", "--port", "8000",
		"--assets-dir", "media", "--glob", "**/*.markdown", "--watch", "--static", "site")

	if got := cfg.Path(); got != "deck" {
		t.Errorf("Path() = %q, want %q", got, "deck")
	}
	if got := cfg.Host(); got != "0.0.0.0" {
		t.Errorf("Host() = %q", got)
	}
	if got := cfg.Port(); got != "8000" {
		t.Errorf("Port() = %q, want 8000", got)
	}
	if got := cfg.AssetsDir(); got != "media" {
		t.Errorf("AssetsDir() = %q", got)
	}
	if got := cfg.FilesGlob(); got != "**/*.markdown" {
		t.Errorf("FilesGlob() = %q", got)
	}
	if !cfg.Watch() {
		t.Error("Watch() = false with --watch")
	}
	if !cfg.HasStatic() {
		t.Error("HasStatic() = false with --static")
	}
	if got := cfg.StaticDir(); got != "site" {
		t.Errorf("StaticDir() = %q, want site", got)
	}
}

func TestStaticWithoutAValueFallsBackToStaticDir(t *testing.T) {
	cfg := newConfig(t, t.TempDir(), ".", "--static")
	if !cfg.HasStatic() {
		t.Fatal("HasStatic() = false")
	}
	if got := cfg.StaticDir(); got != "_static" {
		t.Errorf("StaticDir() = %q, want _static", got)
	}
}

func TestInitialDirIsTheDeckDirectoryForASingleFile(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "sub/slides.md", "# Slide")

	cfg := newConfig(t, cwd, "sub/slides.md")

	initialDir, err := cfg.InitialDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cwd, "sub"); initialDir != want {
		t.Errorf("InitialDir() = %q, want %q", initialDir, want)
	}
	initialPath, err := cfg.InitialPath()
	if err != nil {
		t.Fatal(err)
	}
	if initialPath != "slides.md" {
		t.Errorf("InitialPath() = %q, want slides.md", initialPath)
	}
}

func TestInitialDirIsTheDirectoryItself(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "slides.md", "# Slide")

	cfg := newConfig(t, cwd, ".")

	initialDir, err := cfg.InitialDir()
	if err != nil {
		t.Fatal(err)
	}
	if initialDir != cwd {
		t.Errorf("InitialDir() = %q, want %q", initialDir, cwd)
	}
	initialPath, err := cfg.InitialPath()
	if err != nil {
		t.Fatal(err)
	}
	if initialPath != "" {
		t.Errorf("InitialPath() = %q, want empty", initialPath)
	}
}

func TestOptionsMergeCommandLineOverLocalConfigOverDefaults(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "reveal-md.json", `{"theme":"moon","title":"From config"}`)

	cfg := newConfig(t, cwd, ".", "--theme", "solarized")

	options := cfg.Options()
	if got, _ := options.GetString("theme"); got != "solarized" {
		t.Errorf("theme = %q, want solarized (command line wins)", got)
	}
	if got, _ := options.GetString("title"); got != "From config" {
		t.Errorf("title = %q, want From config (local config wins over defaults)", got)
	}
	if got, _ := options.GetString("highlightTheme"); got != "base16/zenburn" {
		t.Errorf("highlightTheme = %q, want the shipped default", got)
	}
}

func TestSlideOptionsLetFrontMatterOverrideTheLocalConfig(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "reveal-md.json", `{"theme":"moon"}`)

	cfg := newConfig(t, cwd)

	options := cfg.SlideOptions(jsutil.ObjectOf("theme", "white"))
	if got, _ := options.GetString("theme"); got != "white" {
		t.Errorf("theme = %q, want white (front matter wins over the local config)", got)
	}
}

func TestRevealOptionsCarryUnknownCommandLineFlags(t *testing.T) {
	cfg := newConfig(t, t.TempDir(), ".", "--controls", "--width", "1280")

	options := cfg.RevealOptions(jsutil.ObjectOf("transition", "fade"))
	if got, _ := options.GetString("transition"); got != "fade" {
		t.Errorf("transition = %q, want fade", got)
	}
	if got := options.Get("controls"); got != true {
		t.Errorf("controls = %v, want true", got)
	}
	if got := options.Get("width"); got != float64(1280) {
		t.Errorf("width = %v, want 1280", got)
	}
}

func TestThemeURLResolvesBundledRemoteAndLocalThemes(t *testing.T) {
	cwd := t.TempDir()
	cfg := newConfig(t, cwd)

	cases := []struct {
		theme string
		want  string
	}{
		{"black", "/dist/theme/black.css"},
		{"moon", "/dist/theme/moon.css"},
		{"https://example.org/theme.css", "https://example.org/theme.css"},
		{"custom.css", "/_assets/custom.css"},
	}
	for _, tc := range cases {
		if got := cfg.ThemeURL(tc.theme, "_assets", ""); got != tc.want {
			t.Errorf("ThemeURL(%q) = %q, want %q", tc.theme, got, tc.want)
		}
	}
	if got := cfg.ThemeURL("black", "_assets", "."); got != "./dist/theme/black.css" {
		t.Errorf("ThemeURL with a base = %q", got)
	}
	if got := cfg.HighlightThemeURL("base16/zenburn"); got != "/css/highlight/base16/zenburn.css" {
		t.Errorf("HighlightThemeURL() = %q", got)
	}
}

func TestAssetPathsPrefixTheAssetsDirectory(t *testing.T) {
	cfg := newConfig(t, t.TempDir())

	paths, err := cfg.AssetPaths("one.css,two.css,https://example.org/three.css", "_assets", ".")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"./_assets/one.css", "./_assets/two.css", "https://example.org/three.css"}
	if len(paths) != len(want) {
		t.Fatalf("AssetPaths() = %v, want %v", paths, want)
	}
	for i, path := range paths {
		if path != want[i] {
			t.Errorf("AssetPaths()[%d] = %q, want %q", i, path, want[i])
		}
	}

	list, err := cfg.AssetList("one.js,two.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0] != "one.js" || list[1] != "two.js" {
		t.Errorf("AssetList() = %v", list)
	}
	if _, err := cfg.AssetList(jsutil.Undef); err == nil {
		t.Error("AssetList(undefined) = nil error, want the TypeError the original throws")
	}
}

func TestTemplatesComeFromTheBundleUnlessOverridden(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "custom.html", "custom template")
	cfg := newConfig(t, cwd)

	bundled, err := cfg.Template("template/reveal.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bundled, "{{{markdown}}}") {
		t.Error("bundled template does not look like reveal.html")
	}

	listing, err := cfg.ListingTemplate("template/listing.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listing, "{{#files}}") {
		t.Error("bundled listing template does not look like listing.html")
	}

	custom, err := cfg.Template("custom.html")
	if err != nil {
		t.Fatal(err)
	}
	if custom != "custom template" {
		t.Errorf("Template(custom.html) = %q", custom)
	}
}

func TestFaviconPathPrefersTheOneInTheDeckDirectory(t *testing.T) {
	cwd := t.TempDir()
	cfg := newConfig(t, cwd)

	path, err := cfg.FaviconPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Errorf("FaviconPath() = %q, want the embedded fallback", path)
	}

	favicon := writeFile(t, cwd, "favicon.ico", "not really an icon")
	jsutil.ResetStatCache()
	cfg = newConfig(t, cwd)
	path, err = cfg.FaviconPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != favicon {
		t.Errorf("FaviconPath() = %q, want %q", path, favicon)
	}
}

func TestPuppeteerLaunchConfigSplitsTheArguments(t *testing.T) {
	cfg := newConfig(t, t.TempDir(), ".",
		"--puppeteer-launch-args=--no-sandbox --disable-gpu",
		"--puppeteer-chromium-executable", "/opt/chrome")

	args, executable := cfg.PuppeteerLaunchConfig()
	if len(args) != 2 || args[0] != "--no-sandbox" || args[1] != "--disable-gpu" {
		t.Errorf("args = %v", args)
	}
	if executable != "/opt/chrome" {
		t.Errorf("executablePath = %q", executable)
	}

	empty := newConfig(t, t.TempDir())
	args, executable = empty.PuppeteerLaunchConfig()
	if len(args) != 0 {
		t.Errorf("args = %v, want empty", args)
	}
	if executable != "" {
		t.Errorf("executablePath = %q, want empty", executable)
	}
}

func TestLocalConfigIsReadFromJSON5(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "reveal-md.json5", "{\n  // a comment\n  theme: 'moon',\n}\n")

	cfg := newConfig(t, cwd)

	if got, _ := cfg.Options().GetString("theme"); got != "moon" {
		t.Errorf("theme = %q, want moon", got)
	}
}
