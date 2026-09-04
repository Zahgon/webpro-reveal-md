// Package config ports lib/config.js.
//
// The JavaScript module is a singleton: it parses process.argv and reads the
// local config files at import time. Here that state is an explicit value so
// tests can construct one, but the defaults of Load(nil, cwd) reproduce a
// fresh import with no command line arguments.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/webpro/reveal-md/internal/assets"
	"github.com/webpro/reveal-md/internal/jsutil"
	"github.com/webpro/reveal-md/internal/templates"
)

// Aliases is the alias map lib/config.js passes to yargs-parser. It differs
// from the one bin/reveal-md.js uses, and both parses are observable.
var Aliases = map[string][]string{
	"h": {"help"},
	"v": {"version"},
	"w": {"watch"},
}

var printSizeRe = regexp.MustCompile(`^([\d.]+)x([\d.]+)([a-z]*)$`)

type Config struct {
	Cwd          string
	Defaults     *jsutil.Object
	CLI          *jsutil.Object
	Local        *jsutil.Object
	Reveal       *jsutil.Object
	Merged       *jsutil.Object
	revealThemes []string
}

// Load mirrors the import-time work of lib/config.js: parse the arguments,
// read reveal-md.json5/reveal-md.json and reveal.json5/reveal.json from the
// working directory, then merge with _.defaults.
func Load(argv []string, cwd string) (*Config, error) {
	defaultsValue, err := jsutil.ParseJSON(templates.DefaultsJSON())
	if err != nil {
		return nil, fmt.Errorf("parse defaults.json: %w", err)
	}
	defaultsObj, ok := defaultsValue.(*jsutil.Object)
	if !ok {
		return nil, errors.New("defaults.json is not an object")
	}

	local, err := TryReadJSON5Configs(
		jsutil.PathJoin(cwd, "reveal-md.json5"),
		jsutil.PathJoin(cwd, "reveal-md.json"),
	)
	if err != nil {
		return nil, err
	}
	reveal, err := TryReadJSON5Configs(
		jsutil.PathJoin(cwd, "reveal.json5"),
		jsutil.PathJoin(cwd, "reveal.json"),
	)
	if err != nil {
		return nil, err
	}

	cli := jsutil.ParseArgv(argv, jsutil.YargsOptions{Alias: Aliases})

	themeNames, err := assets.ThemeNames()
	if err != nil {
		return nil, err
	}
	themes := make([]string, 0, len(themeNames))
	for _, name := range themeNames {
		if strings.HasSuffix(name, ".css") {
			themes = append(themes, "dist/theme/"+name)
		}
	}
	sort.Strings(themes)

	c := &Config{
		Cwd:          cwd,
		Defaults:     defaultsObj,
		CLI:          cli,
		Local:        local,
		Reveal:       reveal,
		revealThemes: themes,
	}
	c.Merged = jsutil.Defaults(jsutil.NewObject(), cli, local, defaultsObj)
	return c, nil
}

// TryReadJSON5Configs ports util.js tryReadJson5Configs: return the first
// file that exists, parsed as JSON5, and propagate every error except a
// missing file.
func TryReadJSON5Configs(files ...string) (*jsutil.Object, error) {
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		value, err := jsutil.ParseJSON5(string(contents))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		obj, ok := value.(*jsutil.Object)
		if !ok {
			return nil, nil
		}
		return obj, nil
	}
	return nil, nil
}

func (c *Config) Path() string {
	positional, ok := c.CLI.Get("_").([]any)
	if !ok || len(positional) == 0 {
		return "."
	}
	first := jsutil.JSString(positional[0])
	if first == "" {
		return "."
	}
	return first
}

// InitialDir resolves the requested path and, when it points at a file,
// returns its directory. The stat error surfaced here is what produces the
// CLI's ENOENT failure.
func (c *Config) InitialDir() (string, error) {
	dir := jsutil.PathResolve(c.Cwd, c.Path())
	isDir, err := jsutil.IsDirectory(dir)
	if err != nil {
		return "", err
	}
	if isDir {
		return dir, nil
	}
	return jsutil.PathDirname(dir), nil
}

func (c *Config) InitialPath() (string, error) {
	dir, err := c.InitialDir()
	if err != nil {
		return "", err
	}
	return jsutil.PathRelative(dir, jsutil.PathResolve(c.Cwd, c.Path())), nil
}

func (c *Config) AssetsDir() string { return jsutil.JSString(c.Merged.Get("assetsDir")) }

// StaticDir ports `mergedConfig.static === true ? staticDir : static`.
func (c *Config) StaticDir() string {
	value := c.Merged.Get("static")
	if b, ok := value.(bool); ok && b {
		return jsutil.JSString(c.Merged.Get("staticDir"))
	}
	return jsutil.JSString(value)
}

func (c *Config) HasStatic() bool { return jsutil.Truthy(c.Merged.Get("static")) }

func (c *Config) Host() string { return jsutil.JSString(c.Merged.Get("host")) }

func (c *Config) Port() string { return jsutil.JSString(c.Merged.Get("port")) }

func (c *Config) Watch() bool { return jsutil.Truthy(c.Merged.Get("watch")) }

func (c *Config) FilesGlob() string { return jsutil.JSString(c.Merged.Get("glob")) }

func (c *Config) Options() *jsutil.Object { return c.Merged }

// SlideOptions ports getSlideOptions: _.defaultsDeep({}, cliConfig, options,
// localConfig, defaults).
func (c *Config) SlideOptions(options *jsutil.Object) *jsutil.Object {
	return jsutil.DefaultsDeep(jsutil.NewObject(), c.CLI, options, c.Local, c.Defaults)
}

// RevealOptions ports getRevealOptions: _.defaults({}, options, revealConfig,
// cliConfig). The whole parsed command line ends up in the reveal.js options
// object, which is why the rendered page contains {"_":[]}.
func (c *Config) RevealOptions(options *jsutil.Object) *jsutil.Object {
	return jsutil.Defaults(jsutil.NewObject(), options, c.Reveal, c.CLI)
}

func (c *Config) assetPath(asset, assetsDir, base string) string {
	if jsutil.IsAbsoluteURL(asset) {
		return asset
	}
	if assetsDir == "" {
		assetsDir = jsutil.JSString(c.Defaults.Get("assetsDir"))
	}
	return base + "/" + assetsDir + "/" + asset
}

// AssetPaths ports getAssetPaths, including the comma splitting a string
// value gets and the TypeError a non-list value would raise in JavaScript.
func (c *Config) AssetPaths(value any, assetsDir, base string) ([]string, error) {
	var list []any
	switch v := value.(type) {
	case string:
		for _, part := range strings.Split(v, ",") {
			list = append(list, part)
		}
	case []any:
		list = v
	case nil, jsutil.Undefined:
		return nil, fmt.Errorf("TypeError: Cannot read properties of %s (reading 'map')", jsutil.JSString(value))
	default:
		return nil, fmt.Errorf("TypeError: assets.map is not a function")
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, c.assetPath(jsutil.JSString(item), assetsDir, base))
	}
	return out, nil
}

// AssetList applies the `typeof x === 'string' ? x.split(',') : x` idiom
// lib/static.js uses for --scripts, --css and --static-dirs.
func (c *Config) AssetList(value any) ([]string, error) {
	switch v := value.(type) {
	case string:
		return strings.Split(v, ","), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, jsutil.JSString(item))
		}
		return out, nil
	case nil, jsutil.Undefined:
		return nil, fmt.Errorf("TypeError: Cannot read properties of %s (reading 'map')", jsutil.JSString(value))
	default:
		return nil, fmt.Errorf("TypeError: assets.map is not a function")
	}
}

// ThemeURL ports getThemeUrl: a theme whose URL has a host is used as is, a
// name matching a bundled reveal.js theme becomes a dist path, and anything
// else is treated as a file in the assets directory.
func (c *Config) ThemeURL(theme, assetsDir, base string) string {
	if jsutil.LegacyURLHost(theme) != "" {
		return theme
	}
	for _, themePath := range c.revealThemes {
		name := jsutil.PathBasename(themePath)
		ext := jsutil.PathExtname(themePath)
		if strings.Replace(name, ext, "", 1) == theme {
			return base + "/" + themePath
		}
	}
	return c.assetPath(theme, assetsDir, base)
}

func (c *Config) HighlightThemeURL(highlightTheme string) string {
	return "/css/highlight/" + highlightTheme + ".css"
}

// Template ports getTemplate: the default template comes from the package,
// anything else is read relative to the working directory.
func (c *Config) Template(template string) (string, error) {
	if jsutil.JSString(c.Defaults.Get("template")) == template {
		return templates.Read(template)
	}
	contents, err := os.ReadFile(jsutil.PathJoin(c.Cwd, template))
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

func (c *Config) ListingTemplate(template string) (string, error) {
	if jsutil.JSString(c.Defaults.Get("listingTemplate")) == template {
		return templates.Read(template)
	}
	contents, err := os.ReadFile(jsutil.PathJoin(c.Cwd, template))
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

// FaviconPath ports getFaviconPath. The JavaScript's second condition,
// isFile(faviconPath), is an unawaited promise and therefore always truthy,
// so existence alone decides; that behaviour is preserved. An empty path
// means the embedded fallback should be served.
func (c *Config) FaviconPath() (string, error) {
	initialDir, err := c.InitialDir()
	if err != nil {
		return "", err
	}
	faviconPath := jsutil.PathJoin(initialDir, "favicon.ico")
	if _, err := os.Stat(faviconPath); err == nil {
		return faviconPath, nil
	}
	return "", nil
}

// PuppeteerLaunchConfig ports getPuppeteerLaunchConfig.
func (c *Config) PuppeteerLaunchConfig() (args []string, executablePath string) {
	if raw := c.Merged.Get("puppeteerLaunchArgs"); !jsutil.IsUndefined(raw) && raw != nil {
		if s, ok := raw.(string); ok && s != "" {
			args = strings.Split(s, " ")
		}
	}
	if raw := c.Merged.Get("puppeteerChromiumExecutable"); !jsutil.IsUndefined(raw) && raw != nil {
		if s, ok := raw.(string); ok {
			executablePath = s
		}
	}
	return args, executablePath
}

// PageOptions ports getPageOptions, whose result is handed to Puppeteer's
// page.pdf(). Dimensions parsed from --print-size are strings carrying the
// unit; the fallback pair are numbers.
func (c *Config) PageOptions(printSize any) *jsutil.Object {
	if jsutil.Truthy(printSize) {
		size := jsutil.JSString(printSize)
		if m := printSizeRe.FindStringSubmatch(size); m != nil {
			return jsutil.ObjectOf(
				"width", m[1]+m[3],
				"height", m[2]+m[3],
			)
		}
		return jsutil.ObjectOf("format", size)
	}
	if c.Reveal != nil && jsutil.Truthy(c.Reveal.Get("width")) && jsutil.Truthy(c.Reveal.Get("height")) {
		return jsutil.ObjectOf(
			"width", c.Reveal.Get("width"),
			"height", c.Reveal.Get("height"),
		)
	}
	return jsutil.ObjectOf("width", float64(960), "height", float64(700))
}
