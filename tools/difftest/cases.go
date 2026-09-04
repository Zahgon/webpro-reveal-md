package main

type cliCase struct {
	name      string
	fixture   string
	args      []string
	tree      bool
	sortLines bool
}

// globalCases exercise the entry points that do not depend on slide content.
func globalCases() []cliCase {
	return []cliCase{
		{name: "version", fixture: "basic", args: []string{"--version"}},
		{name: "version-short", fixture: "basic", args: []string{"-V"}},
		{name: "help", fixture: "basic", args: nil},
		{name: "help-flag", fixture: "basic", args: []string{"--help"}},
		{name: "missing-file", fixture: "basic", args: []string{"no_such_file.md"}},
		{name: "missing-dir", fixture: "basic", args: []string{"no/such/dir"}},
	}
}

// staticCases cover the static export, which is the only entry point that
// renders every markdown file and writes a complete site to disk.
func staticCases() []cliCase {
	cases := []cliCase{}
	for _, f := range corpus() {
		cases = append(cases,
			cliCase{
				name:      "static/" + f.name,
				fixture:   f.name,
				args:      []string{".", "--static", "_out"},
				tree:      true,
				sortLines: true,
			},
			cliCase{
				name:      "static-theme/" + f.name,
				fixture:   f.name,
				args:      []string{".", "--static", "_out", "--theme", "moon", "--highlight-theme", "github"},
				tree:      true,
				sortLines: true,
			},
			cliCase{
				name:      "static-absolute-url/" + f.name,
				fixture:   f.name,
				args:      []string{".", "--static", "_out", "--absolute-url", "https://example.com/deck", "--title", "Custom & <Title>"},
				tree:      true,
				sortLines: true,
			},
		)
	}
	cases = append(cases,
		cliCase{
			name:      "static-single-file",
			fixture:   "basic",
			args:      []string{"slides.md", "--static", "_out"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-single-file-nested",
			fixture:   "nested",
			args:      []string{"a/b/c.md", "--static", "_out"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-glob",
			fixture:   "nested",
			args:      []string{".", "--static", "_out", "--glob", "**/c*.md"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-separators",
			fixture:   "basic",
			args:      []string{".", "--static", "_out", "-s", "<!--s-->", "-S", "<!--v-->"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-assets",
			fixture:   "assets",
			args:      []string{".", "--static", "_out", "--assets-dir", "media", "--css", "one.css,two.css", "--scripts", "one.js,https://example.org/two.js"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-missing-asset",
			fixture:   "images",
			args:      []string{".", "--static", "_out", "--css", "absent.css"},
			tree:      false,
			sortLines: true,
		},
		cliCase{
			name:      "static-dirs",
			fixture:   "images",
			args:      []string{".", "--static", "_out", "--static-dirs", "assets"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-print-size",
			fixture:   "basic",
			args:      []string{".", "--static", "_out", "--print-size", "210x297mm"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-no-mermaid",
			fixture:   "mermaid",
			args:      []string{".", "--static", "_out", "--no-mermaid"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-remote-theme",
			fixture:   "basic",
			args:      []string{".", "--static", "_out", "--theme", "https://example.org/theme.css"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-preprocessor",
			fixture:   "preprocessor",
			args:      []string{".", "--static", "_out", "--preprocessor", "./pre.mjs"},
			tree:      true,
			sortLines: true,
		},
		cliCase{
			name:      "static-unknown-flags",
			fixture:   "basic",
			args:      []string{".", "--static", "_out", "--controls", "--no-progress", "--width", "1280"},
			tree:      true,
			sortLines: true,
		},
	)
	return cases
}

// serverCases drive the HTTP server; each entry lists the request paths that
// are replayed against both implementations.
func serverCases() []serverCase {
	return []serverCase{
		{
			name:    "server/basic",
			fixture: "basic",
			args:    []string{"."},
			paths: []string{
				"/",
				"/slides.md",
				"/a.md",
				"/nonexistent.md",
				"/slides.md?print-pdf",
				"/favicon.ico",
				"/dist/reveal.js",
				"/dist/theme/black.css",
				"/plugin/markdown/markdown.js",
				"/css/highlight/base16/zenburn.css",
				"/mermaid/dist/mermaid.min.js",
				"/_assets/nope.css",
				"/../../etc/passwd",
				"/%2e%2e/%2e%2e/etc/passwd",
				"/..%2f..%2fetc/passwd",
				"/dist/../../package.json",
				"/no/such/path",
			},
		},
		{
			name:    "server/single-file",
			fixture: "basic",
			args:    []string{"slides.md"},
			paths:   []string{"/", "/slides.md", "/a.md"},
		},
		{
			name:    "server/nested",
			fixture: "nested",
			args:    []string{"."},
			paths:   []string{"/", "/index.md", "/a/b/c.md", "/a/b/c/d.md", "/a/readme.txt", "/a/"},
		},
		{
			name:    "server/collation",
			fixture: "collation",
			args:    []string{"."},
			paths:   []string{"/", "/a.md", "/A.md", "/%C3%A1.md"},
		},
		{
			name:    "server/names",
			fixture: "names",
			args:    []string{"."},
			paths:   []string{"/", "/with%20space.md", "/UPPER.md"},
		},
		{
			name:    "server/frontmatter",
			fixture: "frontmatter",
			args:    []string{"."},
			paths:   []string{"/", "/full.md", "/crlf.md", "/bom.md", "/numbers.md", "/blankfirst.md", "/onlyfront.md", "/notrailing.md"},
		},
		{
			name:    "server/json5config",
			fixture: "json5config",
			args:    []string{"."},
			paths:   []string{"/", "/slides.md"},
		},
		{
			name:    "server/revealconfig",
			fixture: "revealconfig",
			args:    []string{"."},
			paths:   []string{"/", "/slides.md"},
		},
		{
			name:    "server/images",
			fixture: "images",
			args:    []string{"."},
			paths:   []string{"/", "/img.md", "/assets/cat.jpg", "/sub/deep/why.md"},
		},
		{
			name:    "server/emptydir",
			fixture: "emptydir",
			args:    []string{"."},
			paths:   []string{"/", "/readme.txt"},
		},
		{
			name:    "server/assets-dir",
			fixture: "images",
			args:    []string{".", "--assets-dir", "assets"},
			paths:   []string{"/", "/assets/cat.jpg", "/assets/missing.css"},
		},
		{
			name:    "server/escaping",
			fixture: "escaping",
			args:    []string{"."},
			paths:   []string{"/", "/escape.md"},
		},
		{
			name:    "server/unicode",
			fixture: "unicode",
			args:    []string{"."},
			paths:   []string{"/", "/cjk.md", "/emoji.md", "/rtl.md", "/combining.md"},
		},
	}
}
