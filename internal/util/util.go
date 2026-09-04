// Package util ports lib/util.js. The memoised stat helpers and the URL and
// JSON5 readers live in jsutil because they are pure JavaScript emulation;
// what remains here is the file and front matter handling.
package util

import (
	"strings"

	"github.com/webpro/reveal-md/internal/jsutil"
)

const bom = "\ufeff"

// ParseYamlFrontMatter ports parseYamlFrontMatter: strip a leading byte order
// mark, split the document, and fall back to the whole input when the body is
// empty.
func ParseYamlFrontMatter(content string) (*jsutil.Object, string, error) {
	document, err := jsutil.LoadFront(strings.TrimPrefix(content, bom))
	if err != nil {
		return nil, "", err
	}
	yamlOptions := jsutil.Omit(document, jsutil.ContentKey)
	markdown := jsutil.JSString(document.Get(jsutil.ContentKey))
	if markdown == "" {
		markdown = content
	}
	return yamlOptions, markdown, nil
}

// GetFilePaths ports getFilePaths, including the node_modules exclusion and
// POSIX separated results.
func GetFilePaths(workingDir, globPattern string) ([]string, error) {
	return jsutil.GlobSync(globPattern, jsutil.GlobOptions{
		Cwd:    workingDir,
		Ignore: []string{"**/node_modules/**"},
	})
}
