package templates

import (
	"strings"
	"testing"
)

func TestReadReturnsTheBundledTemplates(t *testing.T) {
	for _, name := range []string{"template/reveal.html", "template/listing.html"} {
		content, err := Read(name)
		if err != nil {
			t.Fatalf("Read(%q): %v", name, err)
		}
		if content == "" {
			t.Errorf("Read(%q) returned an empty template", name)
		}
	}
}

func TestRevealTemplateKeepsItsMustachePlaceholders(t *testing.T) {
	content, err := Read("template/reveal.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, placeholder := range []string{
		"{{{title}}}",
		"{{{themeUrl}}}",
		"{{{highlightThemeUrl}}}",
		"{{{markdown}}}",
		"{{{revealOptionsStr}}}",
		"{{#watch}}",
		"{{#absoluteUrl}}",
	} {
		if !strings.Contains(content, placeholder) {
			t.Errorf("reveal.html is missing %s", placeholder)
		}
	}
}

func TestListingTemplateKeepsItsMustachePlaceholders(t *testing.T) {
	content, err := Read("template/listing.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, placeholder := range []string{"{{pageTitle}}", "{{{themeUrl}}}", "{{#files}}", "{{filePath}}", "{{date}}"} {
		if !strings.Contains(content, placeholder) {
			t.Errorf("listing.html is missing %s", placeholder)
		}
	}
}

func TestReadRejectsUnknownNames(t *testing.T) {
	if _, err := Read("template/does-not-exist.html"); err == nil {
		t.Error("expected an error for an unknown template name")
	}
}

func TestDefaultsJSONCarriesEveryDocumentedDefault(t *testing.T) {
	defaults := DefaultsJSON()
	for _, key := range []string{
		`"assetsDir"`, `"css"`, `"highlightTheme"`, `"host"`, `"listingTemplate"`,
		`"port"`, `"preprocessor"`, `"scripts"`, `"separator"`, `"staticDir"`,
		`"staticDirs"`, `"template"`, `"theme"`, `"title"`, `"verticalSeparator"`,
		`"glob"`, `"mermaid"`,
	} {
		if !strings.Contains(defaults, key) {
			t.Errorf("defaults.json is missing %s", key)
		}
	}
}

func TestFaviconIsAnIcoFile(t *testing.T) {
	icon := Favicon()
	if len(icon) < 4 {
		t.Fatalf("favicon is too small: %d bytes", len(icon))
	}
	header := []byte{0x00, 0x00, 0x01, 0x00}
	for i, b := range header {
		if icon[i] != b {
			t.Fatalf("favicon header = % x, want % x", icon[:4], header)
		}
	}
}
