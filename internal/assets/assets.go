// Package assets embeds the browser-side runtime that the JavaScript
// original resolved out of node_modules at run time: reveal.js dist and
// plugin trees, the highlight.js stylesheet collection, and the single
// mermaid bundle the presentation template loads.
package assets

import (
	"embed"
	"io/fs"
)

// Versions of the npm packages these trees were vendored from. They are the
// exact versions pinned in the source project's package.json.
const (
	RevealVersion    = "5.1.0"
	MermaidVersion   = "11.4.0"
	HighlightVersion = "11.10.0"
)

//go:embed all:data
var data embed.FS

func sub(dir string) fs.FS {
	sub, err := fs.Sub(data, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// Reveal is the reveal.js package root, holding dist/ and plugin/, mounted by
// the server as /dist and /plugin and copied wholesale by the static export.
func Reveal() fs.FS { return sub("data/reveal.js") }

// Origin describes where the runtime is served from, standing in for the
// node_modules path the JavaScript original printed at startup.
func Origin() string { return "embedded reveal.js " + RevealVersion }

func RevealDist() fs.FS { return sub("data/reveal.js/dist") }

func RevealPlugin() fs.FS { return sub("data/reveal.js/plugin") }

// HighlightStyles is highlight.js/styles, served at /css/highlight.
func HighlightStyles() fs.FS { return sub("data/highlight.js/styles") }

// Mermaid is the mermaid package root, served at /mermaid.
func Mermaid() fs.FS { return sub("data/mermaid") }

// ThemeNames lists the stylesheet basenames in reveal.js dist/theme, which is
// how config.js decides whether --theme names a bundled theme or a file the
// user supplied.
func ThemeNames() ([]string, error) {
	entries, err := fs.ReadDir(data, "data/reveal.js/dist/theme")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
