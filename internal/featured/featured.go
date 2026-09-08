package featured

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/webpro/reveal-md/internal/browser"
	"github.com/webpro/reveal-md/internal/config"
	"github.com/webpro/reveal-md/internal/jsutil"
)

const (
	viewportWidth  = 1200
	viewportHeight = 1200
	jpegQuality    = 70
	launchTimeout  = 5 * time.Minute
)

// Snapshot reproduces lib/featured-slide.js: when --featured-slide is set it
// screenshots that slide into <targetDir>/featured-slide.jpg for OpenGraph.
func Snapshot(cfg *config.Config, initialURL, targetDir string) error {
	featuredSlide := cfg.Options().Get("featuredSlide")
	if !jsutil.Truthy(featuredSlide) {
		return nil
	}

	args, executablePath := cfg.PuppeteerLaunchConfig()
	if !browser.Available(executablePath) {
		fmt.Fprintln(os.Stderr, "Puppeteer unavailable, unable to create featured slide image for OpenGraph metadata.")
		return nil
	}

	snapshotFilename := targetDir + "/featured-slide.jpg"
	url := fmt.Sprintf("http://%s:%s/%s%s", cfg.Host(), cfg.Port(), initialURL, slideAnchor(jsutil.JSString(featuredSlide)))

	jsutil.Debug(jsutil.ObjectOf("url", url, "snapshotFilename", snapshotFilename))

	if err := capture(url, snapshotFilename, executablePath, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error while generating featured slide snapshot for \"%s\"]\n", initialURL)
		jsutil.Debug(err)
	}
	return nil
}

func capture(url, snapshotFilename, executablePath string, args []string) error {
	session, err := browser.Launch(executablePath, args, launchTimeout)
	if err != nil {
		return err
	}
	defer session.Close()

	var buf []byte
	err = chromedp.Run(session.Context(),
		chromedp.EmulateViewport(viewportWidth, viewportHeight),
		chromedp.Navigate(url),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FullScreenshot(&buf, jpegQuality).Do(ctx)
		}),
	)
	if err != nil {
		return err
	}
	return os.WriteFile(snapshotFilename, buf, 0o644)
}

// slideAnchor turns "2" or "2-3" into "#/2" or "#/2/3", using _.parseInt's
// tolerance for trailing garbage and its NaN result for unparsable input.
func slideAnchor(featuredSlide string) string {
	parts := strings.Split(featuredSlide, "-")
	slide := jsutil.ParseIntJS(parts[0])
	if math.IsNaN(slide) {
		return ""
	}
	anchor := "#/" + jsutil.NumberToString(slide)
	if len(parts) > 1 {
		subslide := jsutil.ParseIntJS(parts[1])
		if !math.IsNaN(subslide) {
			anchor += "/" + jsutil.NumberToString(subslide)
		}
	}
	return anchor
}
