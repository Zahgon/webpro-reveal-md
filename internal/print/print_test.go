package print

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
)

func loadConfig(t *testing.T, argv ...string) *config.Config {
	t.Helper()
	jsutil.ResetStatCache()
	cfg, err := config.Load(argv, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestToInchesMatchesConvertPrintParameterToInches(t *testing.T) {
	cases := []struct {
		value any
		want  float64
		ok    bool
	}{
		{float64(96), 1, true},
		{float64(960), 10, true},
		{"960", 10, true},
		{"10in", 10, true},
		{"2.54cm", 1, true},
		{"25.4mm", 1, true},
		{"96px", 1, true},
		{"8.5in", 8.5, true},
		{jsutil.Undef, 0, false},
		{nil, 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := toInches(c.value)
		if ok != c.ok {
			t.Errorf("toInches(%v) ok = %v, want %v", c.value, ok, c.ok)
			continue
		}
		if ok && math.Abs(got-c.want) > 1e-9 {
			t.Errorf("toInches(%v) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestPaperFormatMatchesPuppeteer(t *testing.T) {
	cases := []struct {
		name          string
		width, height float64
	}{
		{"Letter", 8.5, 11},
		{"letter", 8.5, 11},
		{"Legal", 8.5, 14},
		{"Tabloid", 11, 17},
		{"Ledger", 17, 11},
		{"A0", 33.1, 46.8},
		{"A4", 8.27, 11.7},
		{"A6", 4.13, 5.83},
	}
	for _, c := range cases {
		width, height, ok := paperFormat(c.name)
		if !ok {
			t.Errorf("paperFormat(%q) not found", c.name)
			continue
		}
		if math.Abs(width-c.width) > 1e-9 || math.Abs(height-c.height) > 1e-9 {
			t.Errorf("paperFormat(%q) = %vx%v, want %vx%v", c.name, width, height, c.width, c.height)
		}
	}
	if _, _, ok := paperFormat("NotAFormat"); ok {
		t.Error("paperFormat should not resolve an unknown name")
	}
}

func TestPrintToPDFParamsUsesPixelDimensions(t *testing.T) {
	cfg := loadConfig(t)
	params := printToPDFParams(cfg.PageOptions("1024x768"))
	if math.Abs(params.PaperWidth-1024.0/96.0) > 1e-9 {
		t.Errorf("PaperWidth = %v, want %v", params.PaperWidth, 1024.0/96.0)
	}
	if math.Abs(params.PaperHeight-768.0/96.0) > 1e-9 {
		t.Errorf("PaperHeight = %v, want %v", params.PaperHeight, 768.0/96.0)
	}
	if !params.PrintBackground {
		t.Error("PrintBackground should be true, matching page.pdf({printBackground: true})")
	}
}

func TestPrintToPDFParamsUsesPhysicalUnits(t *testing.T) {
	cfg := loadConfig(t)
	params := printToPDFParams(cfg.PageOptions("210x297mm"))
	if math.Abs(params.PaperWidth-210.0/25.4) > 1e-9 {
		t.Errorf("PaperWidth = %v, want %v", params.PaperWidth, 210.0/25.4)
	}
	if math.Abs(params.PaperHeight-297.0/25.4) > 1e-9 {
		t.Errorf("PaperHeight = %v, want %v", params.PaperHeight, 297.0/25.4)
	}
}

func TestPrintToPDFParamsUsesNamedFormats(t *testing.T) {
	cfg := loadConfig(t)
	params := printToPDFParams(cfg.PageOptions("Letter"))
	if math.Abs(params.PaperWidth-8.5) > 1e-9 || math.Abs(params.PaperHeight-11) > 1e-9 {
		t.Errorf("Letter = %vx%v, want 8.5x11", params.PaperWidth, params.PaperHeight)
	}
}

func TestPrintToPDFParamsFallsBackToRevealDimensions(t *testing.T) {
	cfg := loadConfig(t)
	params := printToPDFParams(cfg.PageOptions(jsutil.Undef))
	if math.Abs(params.PaperWidth-960.0/96.0) > 1e-9 {
		t.Errorf("PaperWidth = %v, want %v", params.PaperWidth, 960.0/96.0)
	}
	if math.Abs(params.PaperHeight-700.0/96.0) > 1e-9 {
		t.Errorf("PaperHeight = %v, want %v", params.PaperHeight, 700.0/96.0)
	}
}

// Puppeteer defaults every margin to zero, while raw CDP would apply 0.4in.
func TestPrintToPDFParamsZeroesTheMargins(t *testing.T) {
	cfg := loadConfig(t)
	params := printToPDFParams(cfg.PageOptions(jsutil.Undef))
	margins := map[string]float64{
		"top":    params.MarginTop,
		"bottom": params.MarginBottom,
		"left":   params.MarginLeft,
		"right":  params.MarginRight,
	}
	for side, value := range margins {
		if value != 0 {
			t.Errorf("margin %s = %v, want 0", side, value)
		}
	}
}

func TestPrintWarnsWhenNoBrowserIsAvailable(t *testing.T) {
	cwd := t.TempDir()
	jsutil.ResetStatCache()
	missing := filepath.Join(cwd, "no-such-chrome")
	cfg, err := config.Load([]string{"--puppeteer-chromium-executable", missing}, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if err := Print(cfg, "http://localhost:1948/slides.md", true, jsutil.Undef); err != nil {
		t.Fatalf("Print should degrade gracefully without a browser, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "slides.pdf")); err == nil {
		t.Error("no PDF should be produced without a browser")
	}
}

func TestRenderPDFFailsWithoutABrowser(t *testing.T) {
	cfg := loadConfig(t, "slides.md")
	err := renderPDF(cfg, "http://localhost:1948/slides.md", "slides.pdf", jsutil.Undef, nil, filepath.Join(t.TempDir(), "absent-chrome"))
	if err == nil {
		t.Fatal("expected an error without a browser")
	}
}

func TestNavigateAndWaitForNetworkIdleBuildsAnAction(t *testing.T) {
	if navigateAndWaitForNetworkIdle("http://localhost:1948/") == nil {
		t.Fatal("expected a chromedp action")
	}
}
