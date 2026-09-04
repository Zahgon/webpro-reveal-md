# reveal-md (Go)

A Go port of [webpro/reveal-md](https://github.com/webpro/reveal-md) v6.1.4 —
turn Markdown files into [reveal.js](https://revealjs.com) presentations from
the command line.

It is a behavioural port, not a rewrite: the CLI surface, the rendered HTML, the
static export tree, the log lines, the HTTP responses and the exit codes are
reproduced byte for byte. See [`truth.md`](truth.md) for the evidence and for
the Divergence Register — every place where behaviour intentionally differs.

## Why a port

The Node original needs `node_modules` at runtime: reveal.js, mermaid and the
highlight.js themes are served straight out of the dependency tree. This port
embeds them in the binary, so `reveal-md` is a single ~23 MB executable with no
runtime dependencies — nothing to install, nothing to resolve, no network.

## Install

```sh
go install github.com/webpro/reveal-md@latest
```

Or build from a checkout:

```sh
make build     # -> bin/reveal-md
```

Or run it in Docker, exactly like the original image:

```sh
docker build -t reveal-md-go .
docker run --rm -p 1948:1948 -v "$PWD:/slides" reveal-md-go /slides --host 0.0.0.0
```

## Usage

```sh
reveal-md slides.md              # serve one deck and open a browser
reveal-md docs/                  # serve a directory with a file listing
reveal-md slides.md --watch      # reload the browser on every save
reveal-md slides.md --static     # write a self-contained site to _static
reveal-md slides.md --print      # render slides.pdf (needs Chrome)
```

Separate slides with `---` on its own line, and vertical slides with `----`:

```markdown
# Title

---

## Second slide

Note: speaker notes go here.

----

### A vertical slide
```

Per-deck settings go in YAML front matter:

```markdown
---
title: My deck
theme: solarized
separator: <!--s-->
verticalSeparator: <!--v-->
revealOptions:
  transition: fade
---
```

Project-wide settings go in `reveal-md.json`, `reveal-md.json5`, `reveal.json`
or `reveal.json5` in the working directory. Every option can also be given on
the command line:

| Option | Description |
|---|---|
| `-V, --version` | print the version |
| `--title` | page title (default `reveal-md`) |
| `-s, --separator` | slide separator (default `\r?\n---\r?\n`) |
| `-S, --vertical-separator` | vertical slide separator (default `\r?\n----\r?\n`) |
| `-t, --theme` | reveal.js theme name, local file or URL (default `black`) |
| `--highlight-theme` | highlight.js theme (default `base16/zenburn`) |
| `--css` | extra stylesheets, comma-separated |
| `--scripts` | extra scripts, comma-separated |
| `--assets-dir` | directory served for `--css`/`--scripts` (default `_assets`) |
| `--preprocessor` | script that transforms Markdown before rendering |
| `--template`, `--listing-template` | replacement Mustache templates |
| `--glob` | which files to list in a directory (default `**/*.md`) |
| `--print [file]` | print to PDF instead of serving |
| `--print-size` | `1024x768`, `210x297mm`, `8.5x11in` or `Letter` |
| `--static [dir]` | write a static site (default `_static`) |
| `--static-dirs` | extra directories to copy into the static site |
| `-w, --watch` | live-reload on file changes |
| `--disable-auto-open` | do not open a browser |
| `--host`, `--port` | listen address (default `localhost:1948`) |
| `--featured-slide` | slide to screenshot for OpenGraph metadata |
| `--absolute-url` | site URL, enables OpenGraph metadata |
| `--puppeteer-launch-args` | extra Chrome flags for printing |
| `--puppeteer-chromium-executable` | path to a Chrome binary |
| `-h, --help` | show help |

Anything not listed above is forwarded to reveal.js, so `--controls`,
`--no-progress` and `--width 1280` work as they do in the original.

### Printing

`--print` and `--featured-slide` drive a real Chrome over the DevTools
protocol. The original used Puppeteer's bundled Chromium; this port looks for
an installed Chrome instead — `--puppeteer-chromium-executable`, then
`PUPPETEER_EXECUTABLE_PATH`, then `google-chrome`/`chromium` on `PATH`, then the
usual install locations. With no browser present it prints the original's
`Puppeteer unavailable, unable to generate PDF file.` and carries on, exactly
like the original does when Puppeteer is not installed.

### Preprocessors

A `.js`, `.mjs` or `.cjs` preprocessor runs under Node, so existing
preprocessors work unchanged:

```js
export default (markdown, options) => markdown.replace(/^#/gm, '##');
```

Any other executable file is also accepted: it receives the Markdown on stdin,
the options as JSON in `REVEAL_MD_OPTIONS`, and writes the result to stdout.

## Development

```sh
make test       # unit tests, including all 23 tests ported from the original
make race       # the same under the race detector
make lint       # gofmt + go vet
make difftest   # run both implementations side by side and compare
```

The JavaScript original ships in this repository as `source_javascript/`.
`make difftest` runs it directly, so it only needs its dependencies installed
once:

```sh
cd source_javascript && PUPPETEER_SKIP_DOWNLOAD=true npm install
```

It then runs 69 scenarios — 6 CLI, 50 static exports, 13 servers — across 13
fixtures and compares stdout, stderr, exit codes, the produced file trees and
every HTTP response.

### Layout

| Path | Contents |
|---|---|
| `main.go` | CLI entry point (port of `bin/reveal-md.js`) |
| `internal/config` | port of `lib/config.js` |
| `internal/render` | port of `lib/render.js` |
| `internal/listing` | port of `lib/listing.js` |
| `internal/server` | port of `lib/server.js` plus an Express/serve-static emulation |
| `internal/static` | port of `lib/static.js` |
| `internal/print`, `internal/featured` | ports of `lib/print.js` and `lib/featured-slide.js` |
| `internal/livereload` | port of the `livereload` package's server and protocol |
| `internal/jsutil` | JavaScript semantics: lodash, mustache, yargs-parser, json5, yaml-front-matter, `node:path`, `node:url`, `util.inspect`, glob, ICU collation |
| `internal/assets` | embedded reveal.js 5.1.0, mermaid 11.4.0, highlight.js 11.10.0 |
| `tools/difftest` | the differential harness |
| `source_javascript` | the original JavaScript implementation, unmodified |

## Licence

MIT, as the original. See [`LICENSE`](LICENSE).
