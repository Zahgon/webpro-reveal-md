package browser

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFindPrefersAnExplicitExecutablePath(t *testing.T) {
	executable := writeFakeChrome(t)
	found, ok := Find(executable)
	if !ok {
		t.Fatal("expected the explicit path to be accepted")
	}
	if found != executable {
		t.Errorf("Find() = %q, want %q", found, executable)
	}
}

func TestFindRejectsAnExplicitPathThatDoesNotExist(t *testing.T) {
	t.Setenv("PUPPETEER_EXECUTABLE_PATH", "")
	missing := filepath.Join(t.TempDir(), "no-such-chrome")
	if _, ok := Find(missing); ok {
		t.Error("expected a missing explicit path to be rejected")
	}
}

func TestFindFallsBackToThePuppeteerEnvironmentVariable(t *testing.T) {
	executable := writeFakeChrome(t)
	t.Setenv("PUPPETEER_EXECUTABLE_PATH", executable)
	found, ok := Find("")
	if !ok {
		t.Fatal("expected PUPPETEER_EXECUTABLE_PATH to be honoured")
	}
	if found != executable {
		t.Errorf("Find() = %q, want %q", found, executable)
	}
}

func TestAvailableMirrorsFind(t *testing.T) {
	executable := writeFakeChrome(t)
	if !Available(executable) {
		t.Error("Available() = false for an existing executable")
	}
	t.Setenv("PUPPETEER_EXECUTABLE_PATH", "")
	if Available(filepath.Join(t.TempDir(), "absent")) {
		t.Error("Available() = true for a missing executable")
	}
}

func TestLaunchFailsWithoutABrowser(t *testing.T) {
	t.Setenv("PUPPETEER_EXECUTABLE_PATH", "")
	missing := filepath.Join(t.TempDir(), "absent")
	session, err := Launch(missing, nil, time.Second)
	if err == nil {
		session.Close()
		t.Fatal("expected Launch to fail without a browser")
	}
	if err != ErrNoBrowser {
		t.Errorf("Launch() error = %v, want %v", err, ErrNoBrowser)
	}
}

func TestSplitFlagParsesPuppeteerLaunchArguments(t *testing.T) {
	cases := []struct {
		arg   string
		name  string
		value any
	}{
		{"--no-sandbox", "no-sandbox", true},
		{"--window-size=800,600", "window-size", "800,600"},
		{"--remote-debugging-port=0", "remote-debugging-port", "0"},
		{"--disable-gpu", "disable-gpu", true},
		{"no-dashes", "no-dashes", true},
		{"--empty=", "empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.arg, func(t *testing.T) {
			name, value := splitFlag(tc.arg)
			if name != tc.name {
				t.Errorf("name = %q, want %q", name, tc.name)
			}
			if value != tc.value {
				t.Errorf("value = %#v, want %#v", value, tc.value)
			}
		})
	}
}

func TestLookupNamesAreExecutableNames(t *testing.T) {
	names := lookupNames()
	if len(names) == 0 {
		t.Fatal("lookupNames() is empty")
	}
	for _, name := range names {
		if strings.ContainsRune(name, filepath.Separator) {
			t.Errorf("lookupNames() entry %q is a path, not a bare name", name)
		}
	}
}

func TestCandidatePathsAreAbsolute(t *testing.T) {
	for _, candidate := range candidatePaths() {
		if !filepath.IsAbs(candidate) {
			t.Errorf("candidatePaths() entry %q is not absolute", candidate)
		}
	}
}

// writeFakeChrome creates a file that Find can accept: on Unix it has to be
// executable, which is why this is not simply an os.WriteFile call.
func writeFakeChrome(t *testing.T) string {
	t.Helper()
	name := "chrome"
	if runtime.GOOS == "windows" {
		name = "chrome.exe"
	}
	executable := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return executable
}

func TestSessionExposesItsContextAndClosesOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	closed := 0
	session := &Session{ctx: ctx, cancelCtx: func() { closed++; cancel() }, cancelAlloc: func() { closed++ }}
	if session.Context() != ctx {
		t.Error("Context() should return the session context")
	}
	session.Close()
	if closed != 2 {
		t.Errorf("cancel calls = %d, want 2", closed)
	}
}
