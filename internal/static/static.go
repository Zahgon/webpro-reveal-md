package static

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/webpro/reveal-md/internal/assets"
	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/featured"
	"github.com/webpro/reveal-md/internal/jsutil"
	"github.com/webpro/reveal-md/internal/listing"
	"github.com/webpro/reveal-md/internal/render"
	"github.com/webpro/reveal-md/internal/templates"
	"github.com/webpro/reveal-md/internal/util"
)

var (
	markdownImageRE           = regexp.MustCompile(`!\[.*?\]\((.+?)\)`)
	htmlImageRE               = regexp.MustCompile(`<img.*?src=["'](.+?)["'].*?>`)
	markdownImageBackgroundRE = regexp.MustCompile(`<!--.*?data-background-image=["'](.+?)["'].*?-->`)
	markdownExtRE             = regexp.MustCompile(`\.md$`)
)

type exporter struct {
	cfg       *config.Config
	staticDir string
	written   map[string]bool
}

// Export reproduces the default export of lib/static.js.
func Export(cfg *config.Config) error {
	e := &exporter{cfg: cfg, staticDir: cfg.StaticDir(), written: map[string]bool{}}

	for _, dir := range []string{"dist", "plugin"} {
		source := embeddedSource{fsys: assets.Reveal(), name: dir, label: "embedded:reveal.js/" + dir}
		if err := e.cp(source, jsutil.PathJoin(e.staticDir, dir)); err != nil {
			return err
		}
	}

	staticDirs, err := cfg.AssetList(cfg.Options().Get("staticDirs"))
	if err != nil {
		return err
	}
	for _, dir := range staticDirs {
		source := diskSource{path: jsutil.PathJoin(cfg.Cwd, dir), cwd: cfg.Cwd}
		target := jsutil.PathJoin(e.staticDir, relativeDir(cfg.Path(), dir))
		if err := e.cp(source, target); err != nil {
			return err
		}
	}

	if err := e.writeMarkupFiles(cfg.Path(), e.staticDir); err != nil {
		return err
	}

	faviconPath, err := cfg.FaviconPath()
	if err != nil {
		return err
	}
	favicon := source(faviconPath, cfg.Cwd)
	if err := e.cp(favicon, jsutil.PathJoin(e.staticDir, "favicon.ico")); err != nil {
		return err
	}

	fmt.Printf("Wrote static site to %s\n", e.staticDir)
	return nil
}

func (e *exporter) writeMarkupFiles(sourceDir, targetDir string) error {
	isDir, err := jsutil.IsDirectory(sourceDir)
	if err == nil && isDir {
		list, err := util.GetFilePaths(sourceDir, e.cfg.FilesGlob())
		if err != nil {
			return err
		}
		htmlList := make([]string, len(list))
		for i, file := range list {
			htmlList[i] = markdownExtRE.ReplaceAllString(file, ".html")
		}
		listMarkup, err := listing.RenderListFile(e.cfg, htmlList)
		if err != nil {
			return err
		}
		if err := e.write(jsutil.PathJoin(targetDir, "index.html"), listMarkup); err != nil {
			return err
		}
		for _, file := range list {
			if err := e.copyAssetsAndWriteFile(sourceDir, file, targetDir); err != nil {
				return err
			}
		}
		return nil
	}

	fileName := jsutil.PathBasename(sourceDir)
	markupName := markdownExtRE.ReplaceAllString(fileName, ".html")
	if err := e.copyAssetsAndWriteFile(jsutil.PathDirname(sourceDir), fileName, targetDir); err != nil {
		return err
	}
	if markupName != "index.html" {
		from := jsutil.PathJoin(targetDir, markupName)
		return e.cp(diskSource{path: from, cwd: e.cfg.Cwd}, jsutil.PathJoin(targetDir, "index.html"))
	}
	return nil
}

func (e *exporter) copyAssetsAndWriteFile(sourceDir, file, targetDir string) error {
	sourcePath := jsutil.PathJoin(sourceDir, file)
	targetPath := markdownExtRE.ReplaceAllString(jsutil.PathJoin(targetDir, file), ".html")

	markdown, err := os.ReadFile(sourcePath)
	if err != nil {
		return jsutil.NewSystemError(err, "open", sourcePath)
	}

	if err := e.copyAssetsFromOptions(string(markdown)); err != nil {
		return err
	}

	base := relativeDir(file, ".")
	markup, err := render.RenderFile(e.cfg, sourcePath, jsutil.ObjectOf("base", base))
	if err != nil {
		return err
	}

	for _, imgPath := range imagePaths(string(markdown)) {
		if jsutil.IsAbsoluteURL(imgPath) {
			continue
		}
		relPath := jsutil.PathJoin(jsutil.PathDirname(file), imgPath)
		from := diskSource{path: jsutil.PathJoin(sourceDir, relPath), cwd: e.cfg.Cwd}
		if err := e.cp(from, jsutil.PathJoin(targetDir, relPath)); err != nil {
			fmt.Fprintln(os.Stderr, jsutil.Inspect(err))
		}
	}

	if err := e.write(targetPath, markup); err != nil {
		return err
	}
	return featured.Snapshot(e.cfg, file, jsutil.PathJoin(targetDir, jsutil.PathDirname(file)))
}

func (e *exporter) copyAssetsFromOptions(markdown string) error {
	yamlOptions, _, err := util.ParseYamlFrontMatter(markdown)
	if err != nil {
		return err
	}
	options := e.cfg.SlideOptions(yamlOptions)
	highlightTheme := jsutil.JSString(options.Get("highlightTheme"))

	highlight := embeddedSource{
		fsys:  assets.HighlightStyles(),
		name:  highlightTheme + ".css",
		label: "embedded:highlight.js/styles/" + highlightTheme + ".css",
	}
	pending := e.queue(highlight, jsutil.PathJoin(e.staticDir, "css", "highlight", highlightTheme+".css"), highlightTheme+".css")

	assetList, err := e.cfg.AssetList(options.Get("scripts"))
	if err != nil {
		return err
	}
	cssList, err := e.cfg.AssetList(options.Get("css"))
	if err != nil {
		return err
	}
	candidates := append(assetList, cssList...)

	theme := jsutil.JSString(options.Get("theme"))
	if exists(jsutil.PathResolve(e.cfg.Cwd, theme)) {
		candidates = append(candidates, theme)
	}

	for _, asset := range candidates {
		if asset == "" || strings.HasPrefix(asset, "http") {
			continue
		}
		from := diskSource{path: jsutil.PathResolve(e.cfg.Cwd, asset), cwd: e.cfg.Cwd}
		pending = append(pending, e.queue(from, jsutil.PathJoin(e.staticDir, e.cfg.AssetsDir(), asset), asset)...)
	}
	return e.runCopies(pending)
}

func imagePaths(markdown string) []string {
	var paths []string
	for _, re := range []*regexp.Regexp{markdownImageRE, htmlImageRE, markdownImageBackgroundRE} {
		for _, match := range re.FindAllStringSubmatch(markdown, -1) {
			paths = append(paths, match[1])
		}
	}
	return paths
}

func (e *exporter) cp(from copySource, target string) error {
	return e.runCopies(e.queue(from, target, from.Label()))
}

type pendingCopy struct {
	from    copySource
	target  string
	errPath string
}

// queue prints the line and reserves the target without touching the
// filesystem. lib/static.js's cp() is an async function whose console.log runs
// when the promise is CREATED, and every asset promise is created before any of
// them is awaited, so a missing asset must not suppress the lines that follow.
func (e *exporter) queue(from copySource, target, errPath string) []*pendingCopy {
	if e.written[target] {
		return nil
	}
	e.written[target] = true
	fmt.Printf("\u274f %s \u2192 %s\n", from.Label(), target)
	return []*pendingCopy{{from: from, target: target, errPath: errPath}}
}

func (e *exporter) runCopies(ops []*pendingCopy) error {
	for _, op := range ops {
		if err := op.from.CopyTo(op.target); err != nil {
			return jsutil.NewSystemError(err, "lstat", op.errPath)
		}
	}
	return nil
}

func (e *exporter) write(target, content string) error {
	fmt.Printf("\u2605 %s\n", target)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

// relativeDir turns a path into the "base" used by templates: only the FIRST
// ".." becomes ".", so "../../x" becomes "./../x", exactly like the original.
func relativeDir(from, to string) string {
	rel := jsutil.PathRelative(from, to)
	if strings.HasPrefix(rel, "..") {
		return "." + rel[2:]
	}
	return rel
}

type copySource interface {
	Label() string
	CopyTo(target string) error
}

func source(path, cwd string) copySource {
	if path == "" {
		return bytesSource{data: templates.Favicon(), label: "embedded:favicon.ico"}
	}
	return diskSource{path: path, cwd: cwd}
}

type bytesSource struct {
	data  []byte
	label string
}

func (b bytesSource) Label() string { return b.label }

func (b bytesSource) CopyTo(target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, b.data, 0o644)
}

type diskSource struct {
	path string
	cwd  string
}

func (d diskSource) Label() string {
	return strings.TrimPrefix(d.path, d.cwd+"/")
}

func (d diskSource) CopyTo(target string) error {
	info, err := os.Stat(d.path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(d.path, target)
	}
	return copyFile(d.path, target, info.Mode())
}

type embeddedSource struct {
	fsys  fs.FS
	name  string
	label string
}

func (s embeddedSource) Label() string { return s.label }

func (s embeddedSource) CopyTo(target string) error {
	info, err := fs.Stat(s.fsys, s.name)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFSFile(s.fsys, s.name, target)
	}
	return fs.WalkDir(s.fsys, s.name, func(entry string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(entry, s.name), "/")
		dest := target
		if rel != "" {
			dest = filepath.Join(target, filepath.FromSlash(rel))
		}
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		return copyFSFile(s.fsys, entry, dest)
	})
}

func copyFSFile(fsys fs.FS, name, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyFile(sourcePath, target string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyDir(sourceDir, target string) error {
	return filepath.WalkDir(sourceDir, func(entry string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, entry)
		if err != nil {
			return err
		}
		dest := target
		if rel != "." {
			dest = filepath.Join(target, rel)
		}
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(entry, dest, info.Mode())
	})
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
