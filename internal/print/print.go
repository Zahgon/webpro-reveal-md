package print

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/webpro/reveal-md/internal/browser"
	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
)

const navigationTimeout = 5 * time.Minute

// Print reproduces lib/print.js: it drives a headless browser to the print
// view of the deck and writes a PDF, reporting failures without failing the
// process.
func Print(cfg *config.Config, initialURL string, printValue any, printSize any) error {
	args, executablePath := cfg.PuppeteerLaunchConfig()
	if !browser.Available(executablePath) {
		fmt.Fprintln(os.Stderr, "Puppeteer unavailable, unable to generate PDF file.")
		return nil
	}

	filename := jsutil.PathBasename(initialURL)
	pdfFilename, ok := printValue.(string)
	if !ok {
		pdfFilename = filename
		if strings.HasSuffix(filename, ".md") {
			pdfFilename = strings.TrimSuffix(filename, ".md") + ".pdf"
		}
	}

	fmt.Printf("Attempting to print \"%s\" to \"%s\".\n", filename, pdfFilename)

	if err := renderPDF(cfg, initialURL, pdfFilename, printSize, args, executablePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error while generating PDF for \"%s\"\n", filename)
		jsutil.Debug(err)
	}
	return nil
}

func renderPDF(cfg *config.Config, initialURL, pdfFilename string, printSize any, args []string, executablePath string) error {
	session, err := browser.Launch(executablePath, args, navigationTimeout)
	if err != nil {
		return err
	}
	defer session.Close()

	pageOptions := cfg.PageOptions(printSize)
	params := printToPDFParams(pageOptions)

	var data []byte
	err = chromedp.Run(session.Context(),
		navigateAndWaitForNetworkIdle(initialURL+"?view=print"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, actionErr := params.Do(ctx)
			if actionErr != nil {
				return actionErr
			}
			data = buf
			return nil
		}),
	)
	if err != nil {
		return err
	}
	return os.WriteFile(pdfFilename, data, 0o644)
}

// printToPDFParams maps puppeteer's page.pdf options onto the CDP call,
// including its unit conversion and its zero default margins (raw CDP would
// otherwise apply 0.4 inch margins).
func printToPDFParams(pageOptions *jsutil.Object) *page.PrintToPDFParams {
	params := page.PrintToPDF().
		WithPrintBackground(true).
		WithMarginTop(0).WithMarginRight(0).WithMarginBottom(0).WithMarginLeft(0).
		WithPaperWidth(8.5).WithPaperHeight(11)

	if format, ok := pageOptions.GetString("format"); ok {
		if width, height, found := paperFormat(format); found {
			params = params.WithPaperWidth(width).WithPaperHeight(height)
		}
		return params
	}

	width, hasWidth := toInches(pageOptions.Get("width"))
	height, hasHeight := toInches(pageOptions.Get("height"))
	if hasWidth {
		params = params.WithPaperWidth(width)
	}
	if hasHeight {
		params = params.WithPaperHeight(height)
	}
	return params
}

func paperFormat(name string) (float64, float64, bool) {
	formats := map[string][2]float64{
		"letter":  {8.5, 11},
		"legal":   {8.5, 14},
		"tabloid": {11, 17},
		"ledger":  {17, 11},
		"a0":      {33.1, 46.8},
		"a1":      {23.4, 33.1},
		"a2":      {16.54, 23.4},
		"a3":      {11.7, 16.54},
		"a4":      {8.27, 11.7},
		"a5":      {5.83, 8.27},
		"a6":      {4.13, 5.83},
	}
	size, ok := formats[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return 0, 0, false
	}
	return size[0], size[1], true
}

// toInches is puppeteer's convertPrintParameterToInches: bare numbers are CSS
// pixels at 96 dpi, strings may carry px, in, cm or mm.
func toInches(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v / 96, true
	case int:
		return float64(v) / 96, true
	case string:
		text := strings.ToLower(strings.TrimSpace(v))
		unit := "px"
		for _, candidate := range []string{"px", "in", "cm", "mm"} {
			if strings.HasSuffix(text, candidate) {
				unit = candidate
				text = strings.TrimSuffix(text, candidate)
				break
			}
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return 0, false
		}
		switch unit {
		case "px":
			return number / 96, true
		case "in":
			return number, true
		case "cm":
			return number / 2.54, true
		case "mm":
			return number / 25.4, true
		}
	}
	return 0, false
}

// navigateAndWaitForNetworkIdle reproduces puppeteer's waitUntil:
// 'networkidle0' using Chrome's own networkIdle lifecycle event.
func navigateAndWaitForNetworkIdle(url string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		idle := make(chan struct{})
		var once bool
		chromedp.ListenTarget(ctx, func(event any) {
			if lifecycle, ok := event.(*page.EventLifecycleEvent); ok && lifecycle.Name == "networkIdle" && !once {
				once = true
				close(idle)
			}
		})
		if err := chromedp.Navigate(url).Do(ctx); err != nil {
			return err
		}
		select {
		case <-idle:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}
