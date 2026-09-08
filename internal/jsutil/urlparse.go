package jsutil

import (
	"errors"
	"strings"
)

// LegacyURLHost reproduces the `host` property of Node's deprecated
// url.parse(input) — with slashesDenoteHost left at its default of false.
//
// lib/config.js uses this to decide whether a --theme / --css / --scripts
// value points at a remote stylesheet or at a local file:
//
//	const { host } = url.parse(theme);
//	return host ? theme : ...
//
// The empirically confirmed behaviour (Node 26, oracle project) is:
//
//	'black'            => null   (falsy)
//	'white.css'        => null
//	'./x.css'          => null
//	'/abs/x.css'       => null
//	'//cdn/x.css'      => null   <- NOT a host, because slashesDenoteHost is off
//	'https://cdn/x.css'=> 'cdn'
//	'C:\x.css'         => ''     (falsy)
//
// So a host is produced only when the input starts with a scheme followed by
// "://". Note this differs from net/url.Parse, which happily reports a host
// for protocol-relative URLs — using the stdlib here would silently change
// which themes are treated as remote.
//
// The returned string is empty whenever the JavaScript value would be falsy.
func LegacyURLHost(input string) string {
	rest, ok := stripLegacyScheme(input)
	if !ok {
		return ""
	}
	// Everything up to the first /, ?, or # is the authority; strip any
	// userinfo, then drop the fragment-ish remainder.
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' || rest[i] == '?' || rest[i] == '#' {
			end = i
			break
		}
	}
	authority := rest[:end]
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		authority = authority[at+1:]
	}
	return strings.ToLower(authority)
}

// stripLegacyScheme returns the remainder after a `scheme://` prefix.
// A scheme is an ASCII letter followed by letters, digits, '+', '-' or '.'.
func stripLegacyScheme(input string) (string, bool) {
	i := 0
	if i >= len(input) || !isASCIILetter(input[i]) {
		return "", false
	}
	i++
	for i < len(input) && isSchemeChar(input[i]) {
		i++
	}
	if !strings.HasPrefix(input[i:], "://") {
		return "", false
	}
	return input[i+3:], true
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSchemeChar(c byte) bool {
	return isASCIILetter(c) || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
}

// IsAbsoluteURL reproduces lib/util.js:
//
//	const isAbsoluteURL = url => url.indexOf('://') > 0 || url.indexOf('//') === 0;
//
// Note the strict `> 0`: a string that begins with "://" is not absolute,
// while a protocol-relative "//cdn/x.css" is. This is a different test from
// LegacyURLHost above, and reveal-md genuinely uses both.
func IsAbsoluteURL(u string) bool {
	return strings.Index(u, "://") > 0 || strings.Index(u, "//") == 0
}

// EncodeURIComponent implements JavaScript's encodeURIComponent: every byte
// outside the unreserved set A-Z a-z 0-9 - _ . ! ~ * ' ( ) is percent-encoded
// from its UTF-8 representation, with uppercase hex digits.
func EncodeURIComponent(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isURIUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

func isURIUnreserved(c byte) bool {
	if isASCIILetter(c) || (c >= '0' && c <= '9') {
		return true
	}
	switch c {
	case '-', '_', '.', '!', '~', '*', '\'', '(', ')':
		return true
	}
	return false
}

// DecodeURI implements JavaScript's decodeURI closely enough for request
// paths: %XX escapes are decoded, and an invalid escape is left as-is rather
// than raising, which keeps malformed URLs from crashing the server.
func DecodeURI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+2 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		hi, ok1 := fromHexDigit(s[i+1])
		lo, ok2 := fromHexDigit(s[i+2])
		if !ok1 || !ok2 {
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String()
}

// DecodeURIComponentStrict decodes like decodeURIComponent but reports the
// malformed sequences that make the JavaScript built-in throw a URIError,
// which is how express.static distinguishes a bad request from a miss.
func DecodeURIComponentStrict(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", errURIMalformed
		}
		hi, ok1 := fromHexDigit(s[i+1])
		lo, ok2 := fromHexDigit(s[i+2])
		if !ok1 || !ok2 {
			return "", errURIMalformed
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String(), nil
}

var errURIMalformed = errors.New("URI malformed")

func fromHexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
