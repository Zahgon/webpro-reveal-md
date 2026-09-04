package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/webpro/reveal-md/internal/assets"
	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
	"github.com/webpro/reveal-md/internal/listing"
	"github.com/webpro/reveal-md/internal/livereload"
	"github.com/webpro/reveal-md/internal/render"
	"github.com/webpro/reveal-md/internal/templates"
)

// markdownRoute is the unanchored regular expression express matches the
// request path against, so any path containing a word followed by ".md" is
// rendered as a presentation.
var markdownRoute = regexp.MustCompile(`(\w+\.md)`)

type Server struct {
	http       *http.Server
	listener   net.Listener
	livereload *livereload.Server
	URL        string
}

// Start ports lib/server.js, including the order the handlers are mounted in,
// which decides what a request resolves to.
func Start(cfg *config.Config) (*Server, error) {
	initialDir, err := cfg.InitialDir()
	if err != nil {
		return nil, err
	}
	initialPath, err := cfg.InitialPath()
	if err != nil {
		return nil, err
	}
	faviconPath, err := cfg.FaviconPath()
	if err != nil {
		return nil, err
	}

	mux := &handler{
		cfg:         cfg,
		initialDir:  initialDir,
		assetsDir:   cfg.AssetsDir(),
		faviconPath: faviconPath,
		dist:        assets.RevealDist(),
		plugin:      assets.RevealPlugin(),
		mermaid:     assets.Mermaid(),
		highlight:   assets.HighlightStyles(),
		cwdFS:       os.DirFS(cfg.Cwd),
		initialFS:   os.DirFS(initialDir),
	}

	fmt.Printf("Serving reveal.js from %s\n", assets.Origin())

	var reloader *livereload.Server
	if cfg.Watch() {
		reloader, err = livereload.Start(initialDir)
		if err != nil {
			return nil, err
		}
	}

	address := fmt.Sprintf(":%s", cfg.Port())
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	httpServer := &http.Server{Handler: mux}
	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, serveErr)
		}
	}()

	fmt.Printf("Reveal-server started at http://%s:%s\n", cfg.Host(), cfg.Port())

	return &Server{
		http:       httpServer,
		listener:   listener,
		livereload: reloader,
		URL:        fmt.Sprintf("http://%s:%s/%s", cfg.Host(), cfg.Port(), initialPath),
	}, nil
}

func (s *Server) Close() error {
	if s.livereload != nil {
		_ = s.livereload.Close()
	}
	return s.http.Shutdown(context.Background())
}

type handler struct {
	cfg         *config.Config
	initialDir  string
	assetsDir   string
	faviconPath string
	dist        fs.FS
	plugin      fs.FS
	mermaid     fs.FS
	highlight   fs.FS
	cwdFS       fs.FS
	initialFS   fs.FS
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	urlPath := req.URL.EscapedPath()

	if urlPath == "/favicon.ico" {
		h.serveFavicon(w, req)
		return
	}
	if rest, ok := mountPath(urlPath, "/plugin"); ok {
		if serveStatic(w, req, rest, staticOptions{fsys: h.plugin, fallThrough: true, index: true}) {
			return
		}
	}
	if rest, ok := mountPath(urlPath, "/dist"); ok {
		if serveStatic(w, req, rest, staticOptions{fsys: h.dist, fallThrough: true, index: true}) {
			return
		}
	}
	if rest, ok := mountPath(urlPath, "/mermaid"); ok {
		if serveStatic(w, req, rest, staticOptions{fsys: h.mermaid, fallThrough: true, index: true}) {
			return
		}
	}
	if rest, ok := mountPath(urlPath, "/css/highlight"); ok {
		if serveStatic(w, req, rest, staticOptions{fsys: h.highlight, fallThrough: true, index: true}) {
			return
		}
	}
	if req.Method == http.MethodGet && markdownRoute.MatchString(urlPath) {
		h.renderMarkdown(w, req)
		return
	}

	if rest, ok := mountPath(urlPath, "/"+h.assetsDir); ok {
		if serveStatic(w, req, rest, staticOptions{fsys: h.cwdFS, fallThrough: false, index: true, rootPath: h.cfg.Cwd}) {
			return
		}
	}
	if serveStatic(w, req, urlPath, staticOptions{fsys: h.initialFS, fallThrough: true, index: true, rootPath: h.initialDir}) {
		return
	}

	if req.Method == http.MethodGet {
		markup, err := listing.Render(h.cfg)
		if err != nil {
			finalHandler(w, http.StatusInternalServerError, err.Error())
			return
		}
		send(w, req, markup)
		return
	}

	notFoundRoute(w, req)
}

func (h *handler) serveFavicon(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		finalHandler(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	body := templates.Favicon()
	if h.faviconPath != "" {
		if custom, err := os.ReadFile(h.faviconPath); err == nil {
			body = custom
		}
	}
	head := w.Header()
	head.Set("Content-Type", "image/x-icon")
	head.Set("Cache-Control", "public, max-age=86400")
	head.Set("ETag", entityTag(body))
	if match := req.Header.Get("If-None-Match"); match != "" && etagMatches(match, head.Get("ETag")) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	head.Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// renderMarkdown ports render.js's default export, including the query strip
// that happens after the path has been decoded and sanitized.
func (h *handler) renderMarkdown(w http.ResponseWriter, req *http.Request) {
	requested := jsutil.DecodeURI(req.URL.RequestURI())
	requested = render.Sanitize(requested)
	if idx := strings.Index(requested, "?"); idx >= 0 {
		requested = requested[:idx]
	}
	filePath := jsutil.PathJoin(h.initialDir, requested)
	markup, err := render.RenderFile(h.cfg, filePath, nil)
	if err != nil {
		finalHandler(w, http.StatusInternalServerError, err.Error())
		return
	}
	send(w, req, markup)
}

func mountPath(urlPath, mount string) (string, bool) {
	if mount == "/" {
		return urlPath, true
	}
	if urlPath == mount {
		return "/", true
	}
	if strings.HasPrefix(urlPath, mount+"/") {
		return urlPath[len(mount):], true
	}
	return "", false
}
