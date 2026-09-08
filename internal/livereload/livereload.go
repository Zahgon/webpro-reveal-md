package livereload

import (
	"context"
	"embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/webpro/reveal-md/internal/jsutil"
)

const (
	Port            = 35729
	ProtocolVersion = "7"
	pollInterval    = 250 * time.Millisecond
)

//go:embed data/livereload.js
var clientScript embed.FS

var watchedExtensions = []string{
	"html", "css", "js", "png", "gif", "jpg",
	"php", "php5", "py", "rb", "erb", "coffee", "md",
}

var excludedDirs = []string{".git", ".svn", ".hg"}

type Server struct {
	http     *http.Server
	listener net.Listener
	clients  map[*conn]struct{}
	mu       sync.Mutex
	stop     chan struct{}
	done     sync.WaitGroup
}

func Start(dir string) (*Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", Port))
	if err != nil {
		return nil, err
	}
	s := &Server{
		listener: listener,
		clients:  map[*conn]struct{}{},
		stop:     make(chan struct{}),
	}
	s.http = &http.Server{Handler: http.HandlerFunc(s.serve)}
	go func() {
		_ = s.http.Serve(listener)
	}()
	s.done.Add(1)
	go s.watch(dir)
	return s, nil
}

func (s *Server) Close() error {
	close(s.stop)
	s.done.Wait()
	s.mu.Lock()
	for c := range s.clients {
		_ = c.close()
	}
	s.clients = map[*conn]struct{}{}
	s.mu.Unlock()
	return s.http.Shutdown(context.Background())
}

func (s *Server) serve(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/livereload.js" {
		body, err := clientScript.ReadFile("data/livereload.js")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	if isWebSocketUpgrade(req) {
		s.upgrade(w, req)
		return
	}
	w.WriteHeader(http.StatusUpgradeRequired)
}

func (s *Server) upgrade(w http.ResponseWriter, req *http.Request) {
	c, err := acceptWebSocket(w, req)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		_ = c.close()
	}()

	for {
		message, err := c.readText()
		if err != nil {
			return
		}
		command, _ := jsutil.ParseJSON(message)
		obj, ok := command.(*jsutil.Object)
		if !ok {
			continue
		}
		if name, _ := obj.GetString("command"); name == "hello" {
			if err := c.writeText(helloMessage()); err != nil {
				return
			}
		}
	}
}

func helloMessage() string {
	protocols := []any{
		"http://livereload.com/protocols/official-7",
		"http://livereload.com/protocols/official-8",
		"http://livereload.com/protocols/official-9",
		"http://livereload.com/protocols/2.x-origin-version-negotiation",
		"http://livereload.com/protocols/2.x-remote-control",
	}
	return jsutil.StringifyOrEmpty(jsutil.ObjectOf(
		"command", "hello",
		"protocols", protocols,
		"serverName", "node-livereload",
	))
}

func reloadMessage(path string) string {
	return jsutil.StringifyOrEmpty(jsutil.ObjectOf(
		"command", "reload",
		"path", path,
		"liveCSS", true,
		"originalPath", "",
		"overrideURL", "",
	))
}

func (s *Server) refresh(path string) {
	message := reloadMessage(path)
	s.mu.Lock()
	targets := make([]*conn, 0, len(s.clients))
	for c := range s.clients {
		targets = append(targets, c)
	}
	s.mu.Unlock()
	for _, c := range targets {
		_ = c.writeText(message)
	}
}

func (s *Server) watch(dir string) {
	defer s.done.Done()
	previous := snapshot(dir)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			current := snapshot(dir)
			for path, stamp := range current {
				if previous[path] != stamp && shouldReload(path) {
					s.refresh(path)
				}
			}
			for path := range previous {
				if _, ok := current[path]; !ok && shouldReload(path) {
					s.refresh(path)
				}
			}
			previous = current
		}
	}
}

func shouldReload(path string) bool {
	ext := strings.TrimPrefix(jsutil.PathExtname(filepath.ToSlash(path)), ".")
	for _, candidate := range watchedExtensions {
		if candidate == ext {
			return true
		}
	}
	return false
}

func snapshot(dir string) map[string]string {
	files := map[string]string{}
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			for _, excluded := range excludedDirs {
				if entry.Name() == excluded {
					return filepath.SkipDir
				}
			}
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		files[path] = fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return files
}
