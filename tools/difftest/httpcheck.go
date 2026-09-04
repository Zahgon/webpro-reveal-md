package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type serverCase struct {
	name    string
	fixture string
	args    []string
	paths   []string
}

type httpResult struct {
	status      int
	contentType string
	body        string
}

type serverRun struct {
	responses []httpResult
	stdout    string
	stderr    string
	exitCode  int
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForServer(port int, proc *exec.Cmd) error {
	deadline := time.Now().Add(30 * time.Second)
	url := fmt.Sprintf("http://localhost:%d/", port)
	for time.Now().Before(deadline) {
		if proc.ProcessState != nil {
			return fmt.Errorf("server exited before becoming ready")
		}
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server did not start within 30s")
}

func runServerCase(impl implementation, dir string, sc serverCase) (*serverRun, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}

	args := append([]string{}, sc.args...)
	args = append(args, "--port", fmt.Sprint(port), "--disable-auto-open")

	cmd := impl.command(args)
	cmd.Dir = dir
	cmd.Env = childEnv()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	useProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if err := waitForServer(port, cmd); err != nil {
		killGroup(cmd)
		<-done
		return nil, fmt.Errorf("%s: %w (stdout=%q stderr=%q)", impl.name, err, stdout.String(), stderr.String())
	}

	client := &http.Client{Timeout: 30 * time.Second}
	responses := make([]httpResult, 0, len(sc.paths))
	for _, p := range sc.paths {
		responses = append(responses, requestPath(client, port, p))
	}

	if err := interruptGroup(cmd); err != nil {
		return nil, err
	}

	exitCode := 0
	select {
	case werr := <-done:
		exitCode = exitCodeOf(werr)
	case <-time.After(30 * time.Second):
		killGroup(cmd)
		<-done
		return nil, fmt.Errorf("%s: server did not exit after SIGINT", impl.name)
	}

	return &serverRun{
		responses: responses,
		stdout:    stdout.String(),
		stderr:    stderr.String(),
		exitCode:  exitCode,
	}, nil
}

// requestPath deliberately builds the request URL by hand so that traversal
// probes such as /..%2f..%2fetc/passwd reach the server exactly as written
// instead of being cleaned by net/http's URL handling.
func requestPath(client *http.Client, port int, p string) httpResult {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/", port), nil)
	if err != nil {
		return httpResult{status: -1, body: err.Error()}
	}
	req.URL.Opaque = p
	if idx := strings.IndexByte(p, '?'); idx >= 0 {
		req.URL.Opaque = p[:idx]
		req.URL.RawQuery = p[idx+1:]
	}

	resp, err := client.Do(req)
	if err != nil {
		return httpResult{status: -1, body: err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return httpResult{status: resp.StatusCode, body: err.Error()}
	}

	return httpResult{
		status:      resp.StatusCode,
		contentType: resp.Header.Get("Content-Type"),
		body:        summarizeBody(resp.Header.Get("Content-Type"), body),
	}
}

// summarizeBody keeps HTML and text bodies verbatim for byte comparison but
// reduces binary assets to a hash so the report stays readable.
func summarizeBody(contentType string, body []byte) string {
	if strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "javascript") || strings.Contains(contentType, "json") {
		return string(normalizeArtifact(".html", body))
	}
	return fmt.Sprintf("<binary %d bytes sha=%s>", len(body), hashBytes(body))
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func childEnv() []string {
	env := os.Environ()
	return append(env, "NO_UPDATE_NOTIFIER=1", "NODE_NO_WARNINGS=0")
}
