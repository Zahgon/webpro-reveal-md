// Package templates embeds the files that lived next to lib/config.js and
// were read relative to its __dirname: the presentation and listing Mustache
// templates, the default option set, and the fallback favicon.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:data
var data embed.FS

//go:embed data/defaults.json
var defaultsJSON string

//go:embed data/favicon.ico
var favicon []byte

// DefaultsJSON is the verbatim content of lib/defaults.json.
func DefaultsJSON() string { return defaultsJSON }

// Favicon is the fallback lib/favicon.ico served when the presentation
// directory does not contain one.
func Favicon() []byte { return favicon }

// Read returns a file stored alongside the original lib/config.js, using the
// same relative path the JavaScript passed to path.join(__dirname, ...),
// for example "template/reveal.html".
func Read(name string) (string, error) {
	b, err := fs.ReadFile(data, "data/"+name)
	if err != nil {
		return "", fmt.Errorf("read builtin template %s: %w", name, err)
	}
	return string(b), nil
}
