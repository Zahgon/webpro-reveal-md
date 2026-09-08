package featured

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
)

func TestSlideAnchorMatchesGetSlideAnchor(t *testing.T) {
	cases := map[string]string{
		"1":     "#/1",
		"2-3":   "#/2/3",
		"0":     "#/0",
		"10-0":  "#/10/0",
		"2-x":   "#/2",
		"x":     "",
		"":      "",
		"3x":    "#/3",
		"4-5-6": "#/4/5",
		"08":    "#/8",
	}
	for input, want := range cases {
		if got := slideAnchor(input); got != want {
			t.Errorf("slideAnchor(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSnapshotDoesNothingWithoutFeaturedSlide(t *testing.T) {
	jsutil.ResetStatCache()
	cwd := t.TempDir()
	cfg, err := config.Load(nil, cwd)
	if err != nil {
		t.Fatal(err)
	}
	targetDir := t.TempDir()
	if err := Snapshot(cfg, "slides.md", targetDir); err != nil {
		t.Fatalf("Snapshot() error = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "featured-slide.jpg")); !os.IsNotExist(err) {
		t.Error("expected no snapshot to be written when --featured-slide is unset")
	}
}

func TestSnapshotWarnsWhenNoBrowserIsAvailable(t *testing.T) {
	jsutil.ResetStatCache()
	cwd := t.TempDir()
	cfg, err := config.Load([]string{"--featured-slide", "1", "--puppeteer-chromium-executable", filepath.Join(cwd, "no-such-chrome")}, cwd)
	if err != nil {
		t.Fatal(err)
	}
	targetDir := t.TempDir()
	if err := Snapshot(cfg, "slides.md", targetDir); err != nil {
		t.Fatalf("Snapshot() error = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "featured-slide.jpg")); !os.IsNotExist(err) {
		t.Error("expected no snapshot to be written without a browser")
	}
}

func TestCaptureFailsWithoutABrowser(t *testing.T) {
	err := capture("http://localhost:1948/slides.md", filepath.Join(t.TempDir(), "featured-slide.jpg"), filepath.Join(t.TempDir(), "absent-chrome"), nil)
	if err == nil {
		t.Fatal("expected an error without a browser")
	}
}
