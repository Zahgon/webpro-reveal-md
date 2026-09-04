package main

import (
	"regexp"
	"sort"
	"strings"
)

var (
	nodeWarningRE   = regexp.MustCompile(`(?m)^\(node:\d+\).*$|^\(Use ` + "`node --trace-.*$")
	assetSourceRE   = regexp.MustCompile(`(?m)^❏ .* → `)
	revealOriginRE  = regexp.MustCompile(`(?m)^Serving reveal\.js from .*$`)
	isoDateRE       = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z`)
	portRE          = regexp.MustCompile(`(localhost|127\.0\.0\.1):\d+`)
	portOptionRE    = regexp.MustCompile(`"port":\d+`)
	updateNotifyRE  = regexp.MustCompile(`(?m)^\s*[╭╰│─╮╯].*$`)
	tmpDirRE        = regexp.MustCompile(`/(private/)?(tmp|var/folders)/[^\s'"]+`)
	lastUpdateRE    = regexp.MustCompile(`Last update: [^<\n]*`)
	blankLineRunsRE = regexp.MustCompile(`\n{3,}`)
	nodeCrashRE     = regexp.MustCompile(`(?m)^(node:internal/.*|\s+at .*|\s*\^\s*|Node\.js v[\d.]+|.*triggerUncaughtException.*|\s*\.\.\..*)$`)
)

// normalizeStream removes the documented, environment-dependent differences
// listed in truth.md: Node deprecation warnings, update-notifier boxes,
// temporary directory names, timestamps, ephemeral ports, and the asset origin
// labels that necessarily differ between node_modules and embedded assets.
func normalizeStream(s string, sortLines bool) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = nodeWarningRE.ReplaceAllString(s, "")
	s = nodeCrashRE.ReplaceAllString(s, "")
	s = updateNotifyRE.ReplaceAllString(s, "")
	s = revealOriginRE.ReplaceAllString(s, "Serving reveal.js from <REVEAL>")
	s = assetSourceRE.ReplaceAllString(s, "❏ <SOURCE> → ")
	s = tmpDirRE.ReplaceAllString(s, "<TMP>")
	s = isoDateRE.ReplaceAllString(s, "<DATE>")
	s = portRE.ReplaceAllString(s, "$1:<PORT>")
	s = portOptionRE.ReplaceAllString(s, `"port":<PORT>`)
	s = blankLineRunsRE.ReplaceAllString(s, "\n")

	lines := []string{}
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	if sortLines {
		sort.Strings(lines)
	}
	return strings.Join(lines, "\n")
}

// normalizeArtifact strips the only nondeterministic byte range inside a
// generated file: the listing page's "Last update" timestamp.
func normalizeArtifact(name string, data []byte) []byte {
	if !strings.HasSuffix(name, ".html") {
		return data
	}
	return lastUpdateRE.ReplaceAll(data, []byte("Last update: <DATE>"))
}
