package jsutil

import (
	"math"
	"strings"
	"time"
)

// JSDate is a JavaScript Date value.
//
// js-yaml resolves YAML timestamps to real Date objects, and those objects
// then flow into JSON.stringify (which calls toJSON, producing an ISO string)
// and into Mustache interpolation (which calls String, producing the
// human-readable form). Both paths are reachable from YAML front matter, so
// the distinction has to survive in the value tree.
type JSDate struct {
	Millis float64
}

// NewJSDate builds a Date from a UTC instant expressed in milliseconds.
func NewJSDate(ms float64) *JSDate { return &JSDate{Millis: ms} }

// ISOString implements Date.prototype.toISOString, which is also what
// Date.prototype.toJSON returns, so it is what JSON.stringify emits.
func (d *JSDate) ISOString() string {
	if math.IsNaN(d.Millis) || math.IsInf(d.Millis, 0) {
		return ""
	}
	sec := math.Floor(d.Millis / 1000)
	ms := d.Millis - sec*1000
	t := time.Unix(int64(sec), 0).UTC()
	year := t.Year()
	var b strings.Builder
	switch {
	case year < 0:
		b.WriteString("-")
		b.WriteString(pad(-year, 6))
	case year > 9999:
		b.WriteString("+")
		b.WriteString(pad(year, 6))
	default:
		b.WriteString(pad(year, 4))
	}
	b.WriteString("-")
	b.WriteString(pad(int(t.Month()), 2))
	b.WriteString("-")
	b.WriteString(pad(t.Day(), 2))
	b.WriteString("T")
	b.WriteString(pad(t.Hour(), 2))
	b.WriteString(":")
	b.WriteString(pad(t.Minute(), 2))
	b.WriteString(":")
	b.WriteString(pad(t.Second(), 2))
	b.WriteString(".")
	b.WriteString(pad(int(ms), 3))
	b.WriteString("Z")
	return b.String()
}

// String implements Date.prototype.toString, which renders in the host's
// local time zone.
func (d *JSDate) String() string {
	if math.IsNaN(d.Millis) {
		return "Invalid Date"
	}
	sec := int64(math.Floor(d.Millis / 1000))
	t := time.Unix(sec, 0).Local()
	return t.Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")
}

func pad(v, width int) string {
	s := ""
	for v > 0 || s == "" {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// ISONow returns new Date().toISOString() for the current instant, which is
// what listing.js stamps onto the generated index page.
func ISONow() string {
	now := time.Now().UTC()
	return NewJSDate(float64(now.UnixNano()) / 1e6).ISOString()
}
