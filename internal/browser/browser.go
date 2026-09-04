package browser

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/chromedp/chromedp"
)

// candidatePaths lists the executables puppeteer's launcher would find on each
// platform once its bundled Chromium is absent. Puppeteer ships its own
// Chromium download; a Go binary cannot, so an installed browser is located
// instead. This is the registered "puppeteer unavailable" equivalent.
func candidatePaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	case "windows":
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	default:
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	}
}

func lookupNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"chrome.exe", "msedge.exe"}
	}
	return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"}
}

// Find returns the browser executable to drive, honouring the explicit
// --puppeteer-chromium-executable value first, then PUPPETEER_EXECUTABLE_PATH,
// then the well-known install locations.
func Find(executablePath string) (string, bool) {
	if executablePath != "" {
		if info, err := os.Stat(executablePath); err == nil && !info.IsDir() {
			return executablePath, true
		}
		return "", false
	}
	if env := os.Getenv("PUPPETEER_EXECUTABLE_PATH"); env != "" {
		if info, err := os.Stat(env); err == nil && !info.IsDir() {
			return env, true
		}
	}
	for _, name := range lookupNames() {
		if resolved, err := exec.LookPath(name); err == nil {
			return resolved, true
		}
	}
	for _, candidate := range candidatePaths() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// Available reports whether a browser can be launched at all, standing in for
// puppeteer's import check.
func Available(executablePath string) bool {
	_, ok := Find(executablePath)
	return ok
}

type Session struct {
	ctx         context.Context
	cancelCtx   context.CancelFunc
	cancelAlloc context.CancelFunc
}

// Launch starts a headless browser with puppeteer's default flag set plus the
// user's --puppeteer-launch-args.
func Launch(executablePath string, args []string, timeout time.Duration) (*Session, error) {
	resolved, ok := Find(executablePath)
	if !ok {
		return nil, ErrNoBrowser
	}

	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(resolved))
	for _, arg := range args {
		name, value := splitFlag(arg)
		opts = append(opts, chromedp.Flag(name, value))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	if timeout > 0 {
		timeoutCtx, cancelTimeout := context.WithTimeout(ctx, timeout)
		ctx = timeoutCtx
		parent := cancelCtx
		cancelCtx = func() {
			cancelTimeout()
			parent()
		}
	}
	if err := chromedp.Run(ctx); err != nil {
		cancelCtx()
		cancelAlloc()
		return nil, err
	}
	return &Session{ctx: ctx, cancelCtx: cancelCtx, cancelAlloc: cancelAlloc}, nil
}

func (s *Session) Context() context.Context { return s.ctx }

func (s *Session) Close() {
	s.cancelCtx()
	s.cancelAlloc()
}

func splitFlag(arg string) (string, any) {
	trimmed := arg
	for len(trimmed) > 0 && trimmed[0] == '-' {
		trimmed = trimmed[1:]
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] == '=' {
			return trimmed[:i], trimmed[i+1:]
		}
	}
	return trimmed, true
}
