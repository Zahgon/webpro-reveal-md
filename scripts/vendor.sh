#!/usr/bin/env bash
#
# Repopulates the third-party assets that get compiled into the binary with
# go:embed. reveal-md served these straight out of node_modules; a single Go
# binary has no node_modules, so the same files are vendored instead.
#
# The versions below are the ones resolved by reveal-md v6.1.4's package.json.
# Changing them changes rendered output, so they are pinned deliberately.
set -euo pipefail

REVEAL_VERSION=5.1.0
MERMAID_VERSION=11.4.0
HIGHLIGHT_VERSION=11.10.0
LIVERELOAD_JS_VERSION=3.4.1

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
assets="$repo_root/internal/assets/data"
livereload="$repo_root/internal/livereload/data"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fetch() {
  local spec=$1 dest=$2
  local tarball
  tarball=$(cd "$work" && npm pack "$spec" --silent)
  mkdir -p "$dest"
  tar -xzf "$work/$tarball" -C "$dest" --strip-components=1
  rm -f "$work/$tarball"
}

echo "vendoring reveal.js@$REVEAL_VERSION"
rm -rf "$assets/reveal.js" "$work/reveal.js"
fetch "reveal.js@$REVEAL_VERSION" "$work/reveal.js"
mkdir -p "$assets/reveal.js"
cp -R "$work/reveal.js/dist" "$work/reveal.js/plugin" "$assets/reveal.js/"

echo "vendoring highlight.js@$HIGHLIGHT_VERSION (styles only)"
rm -rf "$assets/highlight.js" "$work/highlight.js"
fetch "highlight.js@$HIGHLIGHT_VERSION" "$work/highlight.js"
mkdir -p "$assets/highlight.js"
cp -R "$work/highlight.js/styles" "$assets/highlight.js/"

# Only mermaid.min.js is vendored: lib/template/reveal.html references exactly
# that one path, and lib/static.js never copies mermaid at all.
echo "vendoring mermaid@$MERMAID_VERSION (mermaid.min.js only)"
rm -rf "$assets/mermaid" "$work/mermaid"
fetch "mermaid@$MERMAID_VERSION" "$work/mermaid"
mkdir -p "$assets/mermaid/dist"
cp "$work/mermaid/dist/mermaid.min.js" "$assets/mermaid/dist/"

echo "vendoring livereload-js@$LIVERELOAD_JS_VERSION"
rm -rf "$work/livereload-js"
fetch "livereload-js@$LIVERELOAD_JS_VERSION" "$work/livereload-js"
mkdir -p "$livereload"
cp "$work/livereload-js/dist/livereload.js" "$livereload/livereload.js"

echo "done; run 'go build ./...' to re-embed"
