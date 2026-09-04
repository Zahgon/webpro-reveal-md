package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// startServer binds port 0 so the tests never collide with a real reveal-md
// instance or with each other.
func startServer(t *testing.T, cwd string, argv ...string) *Server {
	t.Helper()
	argv = append(argv, "--port", "0", "--disable-auto-open")
	srv, err := Start(loadConfig(t, cwd, argv...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

func get(t *testing.T, srv *Server, path string) (*http.Response, string) {
	t.Helper()
	base := "http://" + srv.listener.Addr().String()
	res, err := http.Get(base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, string(body)
}

func TestServerRendersMarkdownRequests(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "slides.md", "---\ntitle: Served Deck\n---\n# Hello")
	srv := startServer(t, cwd, ".")

	res, body := get(t, srv, "/slides.md")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if !strings.Contains(body, "<title>Served Deck</title>") {
		t.Error("expected the front-matter title in the rendered page")
	}
	if !strings.Contains(body, "# Hello") {
		t.Error("expected the markdown in the rendered page")
	}
}

func TestServerRendersFileNotFoundForMissingDecks(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "slides.md", "# Hello")
	srv := startServer(t, cwd, ".")

	res, body := get(t, srv, "/absent.md")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(body, "File not found.") {
		t.Error("expected the original's catch-all slide for a missing deck")
	}
}

func TestServerListsTheDirectoryAtTheRoot(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "alpha.md", "---\ntitle: Alpha\n---\n# A")
	writeFile(t, cwd, "sub/bravo.md", "# B")
	srv := startServer(t, cwd, ".")

	res, body := get(t, srv, "/")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	for _, want := range []string{"alpha.md", "sub&#x2F;bravo.md", "Alpha", "Last update:"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the listing to contain %q", want)
		}
	}
}

func TestServerServesEmbeddedRevealAssets(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "slides.md", "# Hello")
	srv := startServer(t, cwd, ".")

	cases := []struct {
		path        string
		contentType string
		contains    string
	}{
		{"/dist/reveal.js", "application/javascript; charset=UTF-8", "Reveal"},
		{"/dist/theme/black.css", "text/css; charset=UTF-8", "background"},
		{"/plugin/markdown/markdown.js", "application/javascript; charset=UTF-8", ""},
		{"/css/highlight/base16/zenburn.css", "text/css; charset=UTF-8", ""},
		{"/mermaid/dist/mermaid.min.js", "application/javascript; charset=UTF-8", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			res, body := get(t, srv, tc.path)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.StatusCode)
			}
			if got := res.Header.Get("Content-Type"); got != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tc.contentType)
			}
			if res.Header.Get("ETag") == "" {
				t.Error("expected an ETag on a static asset")
			}
			if tc.contains != "" && !strings.Contains(body, tc.contains) {
				t.Errorf("expected the body to contain %q", tc.contains)
			}
		})
	}
}

func TestServerServesTheFavicon(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "slides.md", "# Hello")
	srv := startServer(t, cwd, ".")

	res, body := get(t, srv, "/favicon.ico")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Cache-Control"); got != "public, max-age=86400" {
		t.Errorf("Cache-Control = %q", got)
	}
	if !strings.HasPrefix(body, "\x00\x00\x01\x00") {
		t.Error("expected ICO magic bytes")
	}
}

func TestServerServesFilesFromTheDeckDirectory(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "slides.md", "# Hello")
	writeFile(t, cwd, "cat.jpg", "not really a jpeg")
	srv := startServer(t, cwd, ".")

	res, body := get(t, srv, "/cat.jpg")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if body != "not really a jpeg" {
		t.Errorf("body = %q", body)
	}
}

func TestServerReportsMissingAssetsUnderTheAssetsMount(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "slides.md", "# Hello")
	srv := startServer(t, cwd, ".")

	res, body := get(t, srv, "/_assets/absent.css")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if !strings.Contains(body, "ENOENT: no such file or directory") {
		t.Errorf("expected the serve-static ENOENT page, got %q", body)
	}
}

func TestServerRefusesPathTraversal(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "slides.md", "# Hello")
	writeFile(t, cwd, "secret.txt", "top secret")
	deck := filepath.Join(cwd, "deck")
	if err := os.MkdirAll(deck, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, deck, "slides.md", "# Hello")
	srv := startServer(t, cwd, "deck")

	for _, path := range []string{"/../secret.txt", "/%2e%2e/secret.txt"} {
		t.Run(path, func(t *testing.T) {
			_, body := get(t, srv, path)
			if strings.Contains(body, "top secret") {
				t.Error("path traversal escaped the deck directory")
			}
		})
	}
}

func TestMountPathStripsThePrefix(t *testing.T) {
	cases := []struct {
		urlPath string
		mount   string
		want    string
		ok      bool
	}{
		{"/dist/reveal.js", "/dist", "/reveal.js", true},
		{"/dist", "/dist", "/", true},
		{"/dist/", "/dist", "/", true},
		{"/distinct/x.js", "/dist", "", false},
		{"/other", "/dist", "", false},
		{"/css/highlight/a.css", "/css/highlight", "/a.css", true},
	}
	for _, tc := range cases {
		got, ok := mountPath(tc.urlPath, tc.mount)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("mountPath(%q, %q) = (%q, %v), want (%q, %v)", tc.urlPath, tc.mount, got, ok, tc.want, tc.ok)
		}
	}
}

func TestEntityTagMatchesTheEtagPackage(t *testing.T) {
	if got, want := entityTag(nil), `W/"0-2jmj7l5rSw0yVb/vlWAYkK/YBwk"`; got != want {
		t.Errorf("entityTag(empty) = %q, want %q", got, want)
	}
	tag := entityTag([]byte("hello"))
	if !strings.HasPrefix(tag, `W/"5-`) {
		t.Errorf("entityTag = %q, want a weak tag with the length in hex", tag)
	}
}

func TestEtagMatchesHandlesListsAndWildcards(t *testing.T) {
	tag := entityTag([]byte("hello"))
	cases := []struct {
		header string
		want   bool
	}{
		{tag, true},
		{"*", true},
		{`W/"nope", ` + tag, true},
		{`W/"nope"`, false},
		{"", false},
	}
	for _, tc := range cases {
		if got := etagMatches(tc.header, tag); got != tc.want {
			t.Errorf("etagMatches(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestContentTypeAddsCharsetForTextualTypes(t *testing.T) {
	cases := map[string]string{
		".html": "text/html; charset=UTF-8",
		".css":  "text/css; charset=UTF-8",
		".js":   "application/javascript; charset=UTF-8",
		".json": "application/json; charset=UTF-8",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".ico":  "image/x-icon",
		".zzz":  "application/octet-stream",
	}
	for ext, want := range cases {
		if got := contentType(ext); got != want {
			t.Errorf("contentType(%q) = %q, want %q", ext, got, want)
		}
	}
}

// escapeHTMLEntities must stay narrower than mustache's table: finalhandler
// uses the escape-html package, which leaves "/", "=" and backticks alone.
func TestEscapeHTMLEntitiesMatchesEscapeHTML(t *testing.T) {
	got := escapeHTMLEntities(`<a href="/x?y=1&z">'` + "`" + `</a>`)
	want := `&lt;a href=&quot;/x?y=1&amp;z&quot;&gt;&#39;` + "`" + `&lt;/a&gt;`
	if got != want {
		t.Errorf("escapeHTMLEntities = %q, want %q", got, want)
	}
}

func TestCreateHTMLDocumentMatchesFinalhandler(t *testing.T) {
	doc := createHTMLDocument("Error: boom")
	for _, want := range []string{
		"<!DOCTYPE html>\n",
		`<html lang="en">`,
		"<title>Error</title>",
		"<pre>Error: boom</pre>",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("expected the error page to contain %q", want)
		}
	}
}

func TestMarkdownRouteMatchesTheExpressRegex(t *testing.T) {
	for _, path := range []string{"/slides.md", "/sub/deck.md", "/a_b.md"} {
		if !markdownRoute.MatchString(path) {
			t.Errorf("expected %q to match the markdown route", path)
		}
	}
	for _, path := range []string{"/slides.html", "/dist/reveal.js", "/"} {
		if markdownRoute.MatchString(path) {
			t.Errorf("expected %q not to match the markdown route", path)
		}
	}
}

func TestParseSingleRangeReadsByteRanges(t *testing.T) {
	cases := []struct {
		header     string
		size       int64
		start, end int64
		ok         bool
	}{
		{"bytes=0-4", 10, 0, 4, true},
		{"bytes=5-", 10, 5, 9, true},
		{"bytes=-3", 10, 7, 9, true},
		{"bytes=0-100", 10, 0, 9, true},
		{"bytes=20-30", 10, 0, 0, false},
		{"items=0-4", 10, 0, 0, false},
		{"", 10, 0, 0, false},
	}
	for _, tc := range cases {
		start, end, ok := parseSingleRange(tc.header, tc.size)
		if ok != tc.ok || (ok && (start != tc.start || end != tc.end)) {
			t.Errorf("parseSingleRange(%q, %d) = (%d, %d, %v), want (%d, %d, %v)",
				tc.header, tc.size, start, end, ok, tc.start, tc.end, tc.ok)
		}
	}
}

func TestEncodeURLEscapesUnsafeCharacters(t *testing.T) {
	cases := map[string]string{
		"/slides.md":     "/slides.md",
		"/with space.md": "/with%20space.md",
		"/%2e%2e/x":      "/%2e%2e/x",
		"/a\"b":          "/a%22b",
		"/a<b>c":         "/a%3Cb%3Ec",
		"/caf\u00e9.md":  "/caf%C3%A9.md",
	}
	for input, want := range cases {
		if got := encodeURL(input); got != want {
			t.Errorf("encodeURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNotFoundRouteMatchesExpress(t *testing.T) {
	recorder := httptest.NewRecorder()
	notFoundRoute(recorder, httptest.NewRequest(http.MethodGet, "/no/such/deck", nil))

	result := recorder.Result()
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", result.StatusCode)
	}
	if got := result.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if !strings.Contains(string(body), "Cannot GET /no/such/deck") {
		t.Errorf("body = %q, want it to report the missing route", body)
	}
}

func TestPathWithTrailingSlashKeepsTheQueryString(t *testing.T) {
	cases := map[string]string{
		"/sub":           "/sub/",
		"/sub?print-pdf": "/sub/?print-pdf",
		"/a/b?x=1&y=2":   "/a/b/?x=1&y=2",
		"/with%20space":  "/with%20space/",
	}
	for raw, want := range cases {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := pathWithTrailingSlash(parsed); got != want {
			t.Errorf("pathWithTrailingSlash(%q) = %q, want %q", raw, got, want)
		}
	}
}
