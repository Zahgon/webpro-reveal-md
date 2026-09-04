package livereload

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func startServer(t *testing.T, dir string) *Server {
	t.Helper()
	srv, err := Start(dir)
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			t.Skipf("port %d is already taken on this machine", Port)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	target := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestHelloMessageAnnouncesTheLiveReloadProtocols(t *testing.T) {
	var hello struct {
		Command    string   `json:"command"`
		Protocols  []string `json:"protocols"`
		ServerName string   `json:"serverName"`
	}
	if err := json.Unmarshal([]byte(helloMessage()), &hello); err != nil {
		t.Fatal(err)
	}
	if hello.Command != "hello" {
		t.Errorf("command = %q, want hello", hello.Command)
	}
	if hello.ServerName != "node-livereload" {
		t.Errorf("serverName = %q, want node-livereload", hello.ServerName)
	}
	want := "http://livereload.com/protocols/official-" + ProtocolVersion
	found := false
	for _, protocol := range hello.Protocols {
		if protocol == want {
			found = true
		}
	}
	if !found {
		t.Errorf("protocols %v do not advertise %s", hello.Protocols, want)
	}
}

func TestReloadMessageMatchesTheLiveReloadPayload(t *testing.T) {
	actual := reloadMessage("/decks/slides.md")
	expected := `{"command":"reload","path":"/decks/slides.md","liveCSS":true,"originalPath":"","overrideURL":""}`
	if actual != expected {
		t.Errorf("reloadMessage:\n got %s\nwant %s", actual, expected)
	}
}

func TestShouldReloadOnlyForWatchedExtensions(t *testing.T) {
	cases := map[string]bool{
		"slides.md":       true,
		"theme.css":       true,
		"plugin.js":       true,
		"cat.png":         true,
		"photo.jpg":       true,
		"notes.txt":       false,
		"archive.tar.gz":  false,
		"Makefile":        false,
		"sub/deck.md":     true,
		"sub/.hidden.css": true,
	}
	for path, want := range cases {
		if actual := shouldReload(path); actual != want {
			t.Errorf("shouldReload(%q) = %v, want %v", path, actual, want)
		}
	}
}

func TestSnapshotSkipsVersionControlDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "slides.md", "# Slides")
	writeFile(t, dir, "sub/nested.md", "# Nested")
	writeFile(t, dir, ".git/config.md", "# Not a deck")

	files := snapshot(dir)
	if _, ok := files[filepath.Join(dir, "slides.md")]; !ok {
		t.Error("snapshot is missing slides.md")
	}
	if _, ok := files[filepath.Join(dir, "sub", "nested.md")]; !ok {
		t.Error("snapshot is missing sub/nested.md")
	}
	if _, ok := files[filepath.Join(dir, ".git", "config.md")]; ok {
		t.Error("snapshot descended into .git")
	}
}

func TestServeReturnsTheLiveReloadClientScript(t *testing.T) {
	startServer(t, t.TempDir())

	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/livereload.js", Port))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if contentType := res.Header.Get("Content-Type"); contentType != "text/javascript" {
		t.Errorf("Content-Type = %q, want text/javascript", contentType)
	}
	if len(body) == 0 {
		t.Error("livereload.js is empty")
	}
}

func TestServeRejectsUnknownPaths(t *testing.T) {
	startServer(t, t.TempDir())

	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/nope", Port))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUpgradeRequired {
		t.Errorf("status = %d, want 426", res.StatusCode)
	}
}

func TestWebSocketClientsGetHelloAndReloadBroadcasts(t *testing.T) {
	dir := t.TempDir()
	srv := startServer(t, dir)

	client := dialLiveReload(t)
	defer client.Close()

	client.writeText(t, `{"command":"hello","protocols":["http://livereload.com/protocols/official-7"]}`)
	if hello := client.readText(t); !strings.Contains(hello, `"command":"hello"`) {
		t.Fatalf("handshake reply = %s", hello)
	}

	srv.refresh(filepath.Join(dir, "slides.md"))
	reload := client.readText(t)
	if !strings.Contains(reload, `"command":"reload"`) || !strings.Contains(reload, "slides.md") {
		t.Errorf("broadcast = %s", reload)
	}
}

type wsClient struct {
	conn   net.Conn
	reader *bufio.Reader
}

func (c *wsClient) Close() { _ = c.conn.Close() }

// dialLiveReload speaks RFC 6455 by hand rather than pulling in a WebSocket
// client library, so the test exercises the same framing the browser sends.
func dialLiveReload(t *testing.T) *wsClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", Port), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	request := "GET /livereload HTTP/1.1\r\n" +
		fmt.Sprintf("Host: 127.0.0.1:%d\r\n", Port) +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + base64.StdEncoding.EncodeToString(key) + "\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	res, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d, want 101", res.StatusCode)
	}
	return &wsClient{conn: conn, reader: reader}
}

func (c *wsClient) writeText(t *testing.T, payload string) {
	t.Helper()
	frame := []byte{0x81}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		t.Fatal(err)
	}
	length := len(payload)
	switch {
	case length < 126:
		frame = append(frame, byte(0x80|length))
	default:
		frame = append(frame, 0x80|126, byte(length>>8), byte(length))
	}
	frame = append(frame, mask...)
	for i := 0; i < length; i++ {
		frame = append(frame, payload[i]^mask[i%4])
	}
	if _, err := c.conn.Write(frame); err != nil {
		t.Fatal(err)
	}
}

func (c *wsClient) readText(t *testing.T) string {
	t.Helper()
	header := make([]byte, 2)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		t.Fatal(err)
	}
	length := int(header[1] & 0x7f)
	switch length {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(c.reader, extended); err != nil {
			t.Fatal(err)
		}
		length = int(binary.BigEndian.Uint16(extended))
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(c.reader, extended); err != nil {
			t.Fatal(err)
		}
		length = int(binary.BigEndian.Uint64(extended))
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
