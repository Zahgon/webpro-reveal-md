// Package render ports lib/render.js.
package render

import (
	"os"
	"strings"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
	"github.com/webpro/reveal-md/internal/util"
)

var slidifyAttributeNames = [][2]string{
	{"notesSeparator", "data-separator-notes"},
	{"separator", "data-separator"},
	{"verticalSeparator", "data-separator-vertical"},
}

// Sanitize ports render.js sanitize: repeatedly drop the first ".." until
// none remain. Rewriting this with filepath.Clean would change which paths
// escape the presentation directory.
func Sanitize(entry string) string {
	for strings.Contains(entry, "..") {
		entry = strings.Replace(entry, "..", "", 1)
	}
	return entry
}

// Render turns markdown, including its optional YAML front matter, into the
// reveal.js page. extra carries the options the tests and the static export
// pass in, and it overrides everything else.
func Render(cfg *config.Config, fullMarkdown string, extra *jsutil.Object) (string, error) {
	yamlOptions, contentOnlyMarkdown, err := util.ParseYamlFrontMatter(fullMarkdown)
	if err != nil {
		return "", err
	}
	options := jsutil.Assign(cfg.SlideOptions(yamlOptions), extra)

	title := options.Get("title")
	base := jsutil.JSString(options.Get("base"))
	if jsutil.IsUndefined(options.Get("base")) {
		base = ""
	}
	assetsDir := jsutil.JSString(options.Get("assetsDir"))

	themeURL := cfg.ThemeURL(jsutil.JSString(options.Get("theme")), assetsDir, base)
	highlightThemeURL := cfg.HighlightThemeURL(jsutil.JSString(options.Get("highlightTheme")))
	scriptPaths, err := cfg.AssetPaths(options.Get("scripts"), assetsDir, base)
	if err != nil {
		return "", err
	}
	cssPaths, err := cfg.AssetPaths(options.Get("css"), assetsDir, base)
	if err != nil {
		return "", err
	}

	revealOptions := jsutil.Assign(jsutil.NewObject(),
		cfg.RevealOptions(objectOrNil(options.Get("revealOptions"))),
		objectOrNil(yamlOptions.Get("revealOptions")))

	slidifyAttributes := []string{}
	for _, pair := range slidifyAttributeNames {
		if !options.Has(pair[0]) {
			continue
		}
		value := jsutil.JSString(options.Get(pair[0]))
		escaped := strings.NewReplacer("\n", `\n`, "\r", `\r`).Replace(value)
		slidifyAttributes = append(slidifyAttributes, pair[1]+`="`+escaped+`"`)
	}

	processedMarkdown, err := runPreprocessor(cfg, options, contentOnlyMarkdown)
	if err != nil {
		return "", err
	}

	revealOptionsStr := jsutil.StringifyOrEmpty(revealOptions)
	var mermaidOptionsStr any = jsutil.Undef
	if mermaid := options.Get("mermaid"); mermaid != false {
		mermaidOptionsStr = jsutil.StringifyOrEmpty(mermaid)
	}

	template, err := cfg.Template(jsutil.JSString(options.Get("template")))
	if err != nil {
		return "", err
	}

	context := options
	context.Set("title", title)
	context.Set("slidifyAttributes", strings.Join(slidifyAttributes, " "))
	context.Set("markdown", processedMarkdown)
	context.Set("themeUrl", themeURL)
	context.Set("highlightThemeUrl", highlightThemeURL)
	context.Set("scriptPaths", toAnySlice(scriptPaths))
	context.Set("cssPaths", toAnySlice(cssPaths))
	context.Set("revealOptionsStr", revealOptionsStr)
	context.Set("mermaidOptionsStr", mermaidOptionsStr)
	context.Set("watch", cfg.Watch())

	return jsutil.RenderMustache(template, context)
}

// RenderFile ports renderFile, whose catch-all turns any read failure into a
// rendered page whose body is the text "File not found.".
func RenderFile(cfg *config.Config, filePath string, extra *jsutil.Object) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return Render(cfg, "File not found.", extra)
	}
	markup, renderErr := Render(cfg, string(content), extra)
	if renderErr != nil {
		return Render(cfg, "File not found.", extra)
	}
	return markup, nil
}

func objectOrNil(value any) *jsutil.Object {
	if obj, ok := value.(*jsutil.Object); ok {
		return obj
	}
	return nil
}

func toAnySlice(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
