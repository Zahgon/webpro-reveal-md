package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/webpro/reveal-md/internal/jsutil"
)

var (
	buildOnce  sync.Once
	binaryPath string
	buildErr   error
)

func revealMD(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "reveal-md-bin")
		if err != nil {
			buildErr = err
			return
		}
		binaryPath = filepath.Join(dir, "reveal-md")
		cmd := exec.Command("go", "build", "-o", binaryPath, ".")
		cmd.Stderr = os.Stderr
		buildErr = cmd.Run()
	})
	if buildErr != nil {
		t.Fatalf("building the CLI failed: %v", buildErr)
	}
	return binaryPath
}

func runCLI(t *testing.T, args ...string) (stdout string, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(revealMD(t), args...)
	cmd.Dir = t.TempDir()
	var out, errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running the CLI failed: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return out.String(), errOut.String(), exitCode
}

func TestShouldPrintVersion(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "--version")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if got := jsutil.JSTrim(stdout); got != version {
		t.Errorf("version = %q, want %q", got, version)
	}
}

func TestShouldProvideHelp(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "--help")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if got, want := jsutil.JSTrim(stdout), jsutil.JSTrim(helpText); got != want {
		t.Errorf("help output does not match help.txt\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestShouldExitOnError(t *testing.T) {
	stdout, stderr, exitCode := runCLI(t, "no_such_file.md")
	if exitCode == 0 {
		t.Fatal("expected a non-zero exit code")
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	pattern := regexp.MustCompile(`\[Error: ENOENT: no such file or directory, stat '.*no_such_file.md'\]`)
	if !pattern.MatchString(stderr) {
		t.Errorf("stderr = %q, want it to match %s", stderr, pattern)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})
}

func deckDir(t *testing.T) string {
	t.Helper()
	jsutil.ResetStatCache()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "slides.md"), []byte("# Slide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	return dir
}

func parse(argv []string) *jsutil.Object {
	return jsutil.ParseArgv(argv, jsutil.YargsOptions{Alias: alias})
}

func TestRunExportsAStaticSite(t *testing.T) {
	dir := deckDir(t)
	argv := []string{".", "--static", "_out"}
	if err := run(argv, parse(argv)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "_out", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("expected the exported listing to have content")
	}
}

func TestRunExportsAStaticSiteWithAFeaturedSlide(t *testing.T) {
	dir := deckDir(t)
	argv := []string{".", "--static", "_out", "--port", "0", "--featured-slide", "1", "--puppeteer-chromium-executable", filepath.Join(dir, "absent-chrome")}
	if err := run(argv, parse(argv)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_out", "slides.html")); err != nil {
		t.Fatal(err)
	}
}

func TestRunPrintsWithoutABrowser(t *testing.T) {
	dir := deckDir(t)
	argv := []string{"slides.md", "--print", "--port", "0", "--puppeteer-chromium-executable", filepath.Join(dir, "absent-chrome")}
	if err := run(argv, parse(argv)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "slides.pdf")); !os.IsNotExist(err) {
		t.Fatal("expected no PDF to be written without a browser")
	}
}

func TestServeStopsOnInterrupt(t *testing.T) {
	deckDir(t)
	interrupted := make(chan struct{})
	previousWait, previousExit := waitForInterrupt, exitProcess
	exitCode := 0
	waitForInterrupt = func() { <-interrupted }
	exitProcess = func(code int) { exitCode = code }
	t.Cleanup(func() { waitForInterrupt, exitProcess = previousWait, previousExit })

	argv := []string{".", "--port", "0", "--disable-auto-open"}
	args := parse(argv)
	done := make(chan error, 1)
	go func() { done <- run(argv, args) }()
	close(interrupted)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if exitCode != 128 {
		t.Errorf("exit code = %d, want 128", exitCode)
	}
}

func TestOpenBrowserSurvivesAMissingLauncher(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	openBrowser("http://localhost:1948/")
}

func TestInspectFormatsSystemErrorsLikeNode(t *testing.T) {
	jsutil.ResetStatCache()
	_, err := jsutil.IsFile(filepath.Join(t.TempDir(), "absent.md"))
	if err == nil {
		t.Fatal("expected a stat error")
	}
	if got := inspect(err); !strings.Contains(got, "ENOENT: no such file or directory") || !strings.Contains(got, "syscall: 'stat'") {
		t.Errorf("inspect() = %q", got)
	}
}
