package config

import (
	"testing"

	"github.com/webpro/reveal-md/internal/jsutil"
)

func pageOptions(t *testing.T, printSize string) *jsutil.Object {
	t.Helper()
	cfg, err := Load(nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cfg.PageOptions(printSize)
}

func assertKey(t *testing.T, actual *jsutil.Object, key, want string) {
	t.Helper()
	got, ok := actual.GetString(key)
	if !ok {
		t.Fatalf("%s is %v, want the string %q", key, actual.Get(key), want)
	}
	if got != want {
		t.Errorf("%s = %q, want %q", key, got, want)
	}
}

func TestShouldHandleDimensionsWithoutUnits(t *testing.T) {
	actual := pageOptions(t, "1024x768")
	assertKey(t, actual, "width", "1024")
	assertKey(t, actual, "height", "768")
}

func TestShouldHandleDimensionsWithUnits(t *testing.T) {
	actual := pageOptions(t, "210x297mm")
	assertKey(t, actual, "width", "210mm")
	assertKey(t, actual, "height", "297mm")
}

func TestShouldHandleFractionalDimensions(t *testing.T) {
	actual := pageOptions(t, "8.5x11in")
	assertKey(t, actual, "width", "8.5in")
	assertKey(t, actual, "height", "11in")
}

func TestShouldHandleFormatName(t *testing.T) {
	actual := pageOptions(t, "Letter")
	assertKey(t, actual, "format", "Letter")
}

func TestShouldFallBackToRevealDimensions(t *testing.T) {
	cfg, err := Load(nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actual := cfg.PageOptions(jsutil.Undef)
	if got := jsutil.StringifyOrEmpty(actual); got != `{"width":960,"height":700}` {
		t.Errorf("PageOptions(undefined) = %s, want {\"width\":960,\"height\":700}", got)
	}
}
