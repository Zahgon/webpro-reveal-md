package listing

import (
	"os"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
	"github.com/webpro/reveal-md/internal/util"
)

func fileMeta(cfg *config.Config, filePath string) *jsutil.Object {
	baseDir, err := cfg.InitialDir()
	if err != nil {
		baseDir = cfg.Cwd
	}
	markdownFilePath := jsutil.PathJoin(baseDir, filePath)
	if len(markdownFilePath) > 5 && markdownFilePath[len(markdownFilePath)-5:] == ".html" {
		markdownFilePath = markdownFilePath[:len(markdownFilePath)-5] + ".md"
	}

	yamlOptions := jsutil.NewObject()
	if markdown, readErr := os.ReadFile(markdownFilePath); readErr != nil {
		reportError(readErr, markdownFilePath)
	} else if parsed, _, parseErr := util.ParseYamlFrontMatter(string(markdown)); parseErr != nil {
		reportError(parseErr, markdownFilePath)
	} else {
		yamlOptions = parsed
	}

	meta := jsutil.ObjectOf(
		"filePath", filePath,
		"fileName", jsutil.PathBasename(filePath),
		"absPath", jsutil.PathResolve(cfg.Cwd, filePath),
	)
	return jsutil.Assign(meta, yamlOptions)
}

// RenderListFile ports renderListFile: the index page listing every markdown
// file, sorted the way JavaScript's localeCompare sorts it.
func RenderListFile(cfg *config.Config, filePaths []string) (string, error) {
	options := cfg.Options()
	template, err := cfg.ListingTemplate(jsutil.JSString(options.Get("listingTemplate")))
	if err != nil {
		return "", err
	}
	themeURL := cfg.ThemeURL(
		jsutil.JSString(options.Get("theme")),
		jsutil.JSString(options.Get("assetsDir")),
		".",
	)

	files := make([]any, 0, len(filePaths))
	metas := make([]*jsutil.Object, 0, len(filePaths))
	for _, filePath := range filePaths {
		metas = append(metas, fileMeta(cfg, filePath))
	}
	sortByFileName(metas)
	for _, meta := range metas {
		files = append(files, meta)
	}

	return jsutil.RenderMustache(template, jsutil.ObjectOf(
		"base", "",
		"themeUrl", themeURL,
		"pageTitle", options.Get("title"),
		"files", files,
		"date", jsutil.ISONow(),
	))
}

// Render ports the default export used as the express listing handler.
func Render(cfg *config.Config) (string, error) {
	initialDir, err := cfg.InitialDir()
	if err != nil {
		return "", err
	}
	list, err := util.GetFilePaths(initialDir, cfg.FilesGlob())
	if err != nil {
		return "", err
	}
	return RenderListFile(cfg, list)
}
