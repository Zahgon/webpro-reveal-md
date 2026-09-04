package server

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/webpro/reveal-md/internal/jsutil"
)

// mimeTypes mirrors mime@1.6.0, the database express 4 resolves against.
// Go's mime.TypeByExtension disagrees on several of these (notably .js and
// .md), and the Content-Type header is part of the observable surface.
var mimeTypes = map[string]string{
	".html": "text/html",
	".htm":  "text/html",
	".css":  "text/css",
	".js":   "application/javascript",
	".mjs":  "application/javascript",
	".json": "application/json",
	".map":  "application/json",
	".md":   "text/markdown",
	".txt":  "text/plain",
	".xml":  "text/xml",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".ico":  "image/x-icon",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".woff": "font/woff",
	".ttf":  "font/ttf",
	".otf":  "font/otf",
	".eot":  "application/vnd.ms-fontobject",
	".pdf":  "application/pdf",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mp3":  "audio/mpeg",
	".wasm": "application/wasm",
	".zip":  "application/zip",
}

// charsetTypes lists the types mime.charsets.lookup annotates with UTF-8.
func contentType(ext string) string {
	base, ok := mimeTypes[strings.ToLower(ext)]
	if !ok {
		return "application/octet-stream"
	}
	if strings.HasPrefix(base, "text/") || base == "application/javascript" || base == "application/json" {
		return base + "; charset=UTF-8"
	}
	return base
}

// entityTag reproduces the etag package for a response body: a weak tag built
// from the byte length in hex and the first 27 characters of the base64 SHA-1.
func entityTag(body []byte) string {
	if len(body) == 0 {
		return `W/"0-2jmj7l5rSw0yVb/vlWAYkK/YBwk"`
	}
	sum := sha1.Sum(body)
	hash := base64.StdEncoding.EncodeToString(sum[:])
	if len(hash) > 27 {
		hash = hash[:27]
	}
	return fmt.Sprintf(`W/"%s-%s"`, strconv.FormatInt(int64(len(body)), 16), hash)
}

// statTag reproduces the etag package for a file: size and modification time
// in hex, weak because that is the default for stat input.
func statTag(size int64, modTime time.Time) string {
	return fmt.Sprintf(`W/"%s-%s"`,
		strconv.FormatInt(size, 16),
		strconv.FormatInt(modTime.UnixMilli(), 16))
}

// send ports express's res.send(string).
func send(w http.ResponseWriter, req *http.Request, body string) {
	raw := []byte(body)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("ETag", entityTag(raw))
	if match := req.Header.Get("If-None-Match"); match != "" && etagMatches(match, h.Get("ETag")) {
		h.Del("Content-Type")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Length", strconv.Itoa(len(raw)))
	if req.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func etagMatches(header, tag string) bool {
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == tag || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(tag, "W/") {
			return true
		}
	}
	return false
}

// escapeHTMLEntities ports the escape-html package used by finalhandler and
// express's redirect body. Its table is deliberately smaller than mustache's:
// `/`, `=` and backtick are left alone, which is visible in the error pages.
func escapeHTMLEntities(s string) string {
	return htmlEntityReplacer.Replace(s)
}

var htmlEntityReplacer = strings.NewReplacer(
	`&`, "&amp;",
	`"`, "&quot;",
	`'`, "&#39;",
	`<`, "&lt;",
	`>`, "&gt;",
)

// createHTMLDocument ports finalhandler's error page, byte for byte.
func createHTMLDocument(message string) string {
	body := escapeHTMLEntities(message)
	body = strings.ReplaceAll(body, "\n", "<br>")
	body = strings.ReplaceAll(body, "  ", " &nbsp;")
	return "<!DOCTYPE html>\n" +
		"<html lang=\"en\">\n" +
		"<head>\n" +
		"<meta charset=\"utf-8\">\n" +
		"<title>Error</title>\n" +
		"</head>\n" +
		"<body>\n" +
		"<pre>" + body + "</pre>\n" +
		"</body>\n" +
		"</html>\n"
}

func finalHandler(w http.ResponseWriter, status int, message string) {
	body := []byte(createHTMLDocument(message))
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// notFoundRoute ports express's default 404 for an unmatched request.
func notFoundRoute(w http.ResponseWriter, req *http.Request) {
	finalHandler(w, http.StatusNotFound,
		"Cannot "+req.Method+" "+encodeURL(req.URL.RequestURI()))
}

// encodeURL ports the encodeurl package: percent-encode only what must be,
// and leave existing escape sequences intact.
func encodeURL(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 0x21 && c <= 0x7e && c != '"' && c != '\'' && c != '<' && c != '>' && c != '\\' && c != '^' && c != '`' && c != '{' && c != '|' && c != '}':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

type staticOptions struct {
	fsys        fs.FS
	fallThrough bool
	index       bool
	// rootPath is the absolute directory fsys is rooted at. serve-static
	// reports the failing absolute path in its error page, and fsys alone
	// cannot reconstruct it.
	rootPath string
}

// embeddedModTime stands in for the modification time of the vendored
// reveal.js, mermaid and highlight.js files, which an embedded filesystem
// does not carry. It has to be stable for the lifetime of the process so
// that ETag and Last-Modified stay consistent across requests.
var embeddedModTime = time.Now().Truncate(time.Second)

// serveStatic ports express.static: index resolution, the trailing-slash
// redirect, conditional requests, ranges, and the fallthrough behaviour that
// decides whether a miss continues to the next handler or ends the request.
func serveStatic(w http.ResponseWriter, req *http.Request, urlPath string, opts staticOptions) bool {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		if opts.fallThrough {
			return false
		}
		w.Header().Set("Allow", "GET, HEAD")
		finalHandler(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return true
	}

	decoded, err := decodePath(urlPath)
	if err != nil {
		finalHandler(w, http.StatusBadRequest, "Bad Request")
		return true
	}
	if strings.Contains(decoded, "\x00") {
		finalHandler(w, http.StatusBadRequest, "Bad Request")
		return true
	}

	clean := jsutil.PathNormalize("/" + strings.TrimPrefix(decoded, "/"))
	if strings.HasPrefix(clean, "..") {
		finalHandler(w, http.StatusForbidden, "Forbidden")
		return true
	}
	for _, segment := range strings.Split(clean, "/") {
		if strings.HasPrefix(segment, ".") && segment != "." && segment != ".." {
			if opts.fallThrough {
				return false
			}
			finalHandler(w, http.StatusNotFound, "Not Found")
			return true
		}
	}

	target := strings.TrimPrefix(clean, "/")
	if target == "" {
		target = "."
	}
	info, err := fs.Stat(opts.fsys, target)
	statErr := err
	if err == nil && info.IsDir() {
		if !opts.index {
			if opts.fallThrough {
				return false
			}
			finalHandler(w, http.StatusNotFound, "Not Found")
			return true
		}
		if !strings.HasSuffix(urlPath, "/") {
			location := encodeURL(pathWithTrailingSlash(req.URL))
			w.Header().Set("Location", location)
			finalHandler(w, http.StatusMovedPermanently,
				"Redirecting to "+escapeHTMLEntities(location))
			return true
		}
		target = path.Join(target, "index.html")
		info, err = fs.Stat(opts.fsys, target)
		// send's sendIndex reports a bare 404 when no index file matches,
		// unlike the initial stat, whose error reaches the client verbatim.
		statErr = nil
	}
	if err != nil || info.IsDir() {
		if opts.fallThrough {
			return false
		}
		if statErr != nil {
			status, message := statErrorPage(opts.rootPath, clean, statErr)
			finalHandler(w, status, message)
			return true
		}
		finalHandler(w, http.StatusNotFound, "Not Found")
		return true
	}

	sendFile(w, req, opts.fsys, target, info)
	return true
}

// statErrorPage ports send's onStatError: the failing stat is turned into a
// 404 for the three "missing" error codes and a 500 otherwise, and the error
// itself becomes the response body because finalhandler prints err.stack,
// whose first line is "Error: " followed by the message.
func statErrorPage(root, urlPath string, err error) (int, string) {
	sysErr := jsutil.NewSystemError(err, "stat", jsutil.PathJoin(root, urlPath))
	status := http.StatusNotFound
	switch sysErr.Code {
	case "ENOENT", "ENOTDIR", "ENAMETOOLONG":
	default:
		status = http.StatusInternalServerError
	}
	return status, "Error: " + sysErr.Error()
}

func pathWithTrailingSlash(u *url.URL) string {
	clone := *u
	clone.Path += "/"
	return clone.RequestURI()
}

func sendFile(w http.ResponseWriter, req *http.Request, fsys fs.FS, target string, info fs.FileInfo) {
	modTime := info.ModTime()
	if modTime.IsZero() {
		modTime = embeddedModTime
	}
	h := w.Header()
	h.Set("Content-Type", contentType(path.Ext(target)))
	h.Set("Accept-Ranges", "bytes")
	h.Set("Cache-Control", "public, max-age=0")
	h.Set("Last-Modified", modTime.UTC().Format(http.TimeFormat))
	h.Set("ETag", statTag(info.Size(), modTime))

	if match := req.Header.Get("If-None-Match"); match != "" && etagMatches(match, h.Get("ETag")) {
		h.Del("Content-Type")
		h.Del("Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if since := req.Header.Get("If-Modified-Since"); since != "" && req.Header.Get("If-None-Match") == "" {
		if t, parseErr := http.ParseTime(since); parseErr == nil && !modTime.Truncate(time.Second).After(t) {
			h.Del("Content-Type")
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	file, err := fsys.Open(target)
	if err != nil {
		finalHandler(w, http.StatusNotFound, "Not Found")
		return
	}
	defer func() { _ = file.Close() }()

	if rangeHeader := req.Header.Get("Range"); rangeHeader != "" {
		if start, end, ok := parseSingleRange(rangeHeader, info.Size()); ok {
			h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, info.Size()))
			h.Set("Content-Length", strconv.FormatInt(end-start+1, 10))
			w.WriteHeader(http.StatusPartialContent)
			if req.Method != http.MethodHead {
				if seeker, ok := file.(io.Seeker); ok {
					_, _ = seeker.Seek(start, io.SeekStart)
				} else {
					_, _ = io.CopyN(io.Discard, file, start)
				}
				_, _ = io.CopyN(w, file, end-start+1)
			}
			return
		}
	}

	h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		_, _ = io.Copy(w, file)
	}
}

func parseSingleRange(header string, size int64) (int64, int64, bool) {
	if !strings.HasPrefix(header, "bytes=") || strings.Contains(header, ",") {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(header, "bytes=")
	dash := strings.Index(spec, "-")
	if dash < 0 {
		return 0, 0, false
	}
	startText, endText := spec[:dash], spec[dash+1:]
	switch {
	case startText == "":
		length, err := strconv.ParseInt(endText, 10, 64)
		if err != nil || length <= 0 {
			return 0, 0, false
		}
		if length > size {
			length = size
		}
		return size - length, size - 1, true
	default:
		start, err := strconv.ParseInt(startText, 10, 64)
		if err != nil || start >= size {
			return 0, 0, false
		}
		end := size - 1
		if endText != "" {
			parsed, parseErr := strconv.ParseInt(endText, 10, 64)
			if parseErr != nil {
				return 0, 0, false
			}
			if parsed < end {
				end = parsed
			}
		}
		if end < start {
			return 0, 0, false
		}
		return start, end, true
	}
}

func decodePath(p string) (string, error) {
	return jsutil.DecodeURIComponentStrict(p)
}
