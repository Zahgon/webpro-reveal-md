package jsutil

import "testing"

// cfgAliases is the alias map lib/config.js passes to yargs-parser.
var cfgAliases = YargsOptions{Alias: map[string][]string{
	"h": {"help"},
	"v": {"version"},
	"w": {"watch"},
}}

// binAliases is the alias map bin/reveal-md.js passes to yargs-parser.
var binAliases = YargsOptions{Alias: map[string][]string{
	"h": {"help"},
	"s": {"separator"},
	"S": {"vertical-separator"},
	"t": {"theme"},
	"V": {"version"},
}}

// The expectations below are the recorded output of yargs-parser 21.1.1
// running in the oracle project, serialised with JSON.stringify so that key
// order is part of the assertion.
func TestParseArgvMatchesYargsParser(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		opts YargsOptions
		want string
	}{
		{"cfg empty", []string{}, cfgAliases, `{"_":[]}`},
		{"bin empty", []string{}, binAliases, `{"_":[]}`},
		{"positional", []string{"slides.md"}, cfgAliases, `{"_":["slides.md"]}`},
		{"cfg --version", []string{"--version"}, cfgAliases, `{"_":[],"version":true,"v":true}`},
		{"bin --version", []string{"--version"}, binAliases, `{"_":[],"version":true,"V":true}`},
		{"cfg -V", []string{"-V"}, cfgAliases, `{"_":[],"V":true}`},
		{"bin -V", []string{"-V"}, binAliases, `{"_":[],"V":true,"version":true}`},
		{"cfg -h", []string{"-h"}, cfgAliases, `{"_":[],"h":true,"help":true}`},
		{"bin -h", []string{"-h"}, binAliases, `{"_":[],"h":true,"help":true}`},
		{"cfg -w", []string{"-w"}, cfgAliases, `{"_":[],"w":true,"watch":true}`},
		{"bin -w", []string{"-w"}, binAliases, `{"_":[],"w":true}`},
		{
			"cfg kebab flag",
			[]string{"--vertical-separator", "x"}, cfgAliases,
			`{"_":[],"vertical-separator":"x","verticalSeparator":"x"}`,
		},
		{
			"bin kebab flag with alias",
			[]string{"--vertical-separator", "x"}, binAliases,
			`{"_":[],"vertical-separator":"x","S":"x","verticalSeparator":"x"}`,
		},
		{
			"bin short alias expands",
			[]string{"-S", "x"}, binAliases,
			`{"_":[],"S":"x","vertical-separator":"x","verticalSeparator":"x"}`,
		},
		{"number value", []string{"--port", "1948"}, cfgAliases, `{"_":[],"port":1948}`},
		{"number equals", []string{"--port=1948"}, cfgAliases, `{"_":[],"port":1948}`},
		{"string value", []string{"--theme", "solarized"}, cfgAliases, `{"_":[],"theme":"solarized"}`},
		{"bare flag", []string{"--print"}, cfgAliases, `{"_":[],"print":true}`},
		{"flag with value", []string{"--print", "out.pdf"}, cfgAliases, `{"_":[],"print":"out.pdf"}`},
		{"static bare", []string{"--static"}, cfgAliases, `{"_":[],"static":true}`},
		{"static value", []string{"--static", "site"}, cfgAliases, `{"_":[],"static":"site"}`},
		{
			"comma list stays a string",
			[]string{"--static-dirs", "a,b"}, cfgAliases,
			`{"_":[],"static-dirs":"a,b","staticDirs":"a,b"}`,
		},
		{
			"repeated flag becomes an array",
			[]string{"--css", "a.css", "--css", "b.css"}, cfgAliases,
			`{"_":[],"css":["a.css","b.css"]}`,
		},
		{"negation", []string{"--no-highlight"}, cfgAliases, `{"_":[],"highlight":false}`},
		{
			"disable-auto-open camel twin",
			[]string{"--disable-auto-open"}, cfgAliases,
			`{"_":[],"disable-auto-open":true,"disableAutoOpen":true}`,
		},
		{"grouped shorts", []string{"-abc"}, cfgAliases, `{"_":[],"a":true,"b":true,"c":true}`},
		{"double dash terminator", []string{"--", "--raw"}, cfgAliases, `{"_":["--raw"]}`},
		{"dot notation", []string{"--a.b", "1"}, cfgAliases, `{"_":[],"a":{"b":1}}`},
		{"hex number", []string{"--x", "0x10"}, cfgAliases, `{"_":[],"x":16}`},
		{"leading zeros stay a string", []string{"--x", "007"}, cfgAliases, `{"_":[],"x":"007"}`},
		{"exponent", []string{"--x", "1e3"}, cfgAliases, `{"_":[],"x":1000}`},
		{"empty string value", []string{"--x", ""}, cfgAliases, `{"_":[],"x":""}`},
		{
			"puppeteer args swallow the next flag",
			[]string{"--puppeteer-launch-args", "--no-sandbox"}, cfgAliases,
			`{"_":[],"puppeteer-launch-args":true,"puppeteerLaunchArgs":true,"sandbox":false}`,
		},
		{
			"mixed invocation",
			[]string{"demo", "-w", "--port", "8000"}, cfgAliases,
			`{"_":["demo"],"w":true,"watch":true,"port":8000}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StringifyOrEmpty(ParseArgv(tc.argv, tc.opts))
			if got != tc.want {
				t.Fatalf("ParseArgv(%q)\n got: %s\nwant: %s", tc.argv, got, tc.want)
			}
		})
	}
}
