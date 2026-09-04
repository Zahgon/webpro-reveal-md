package main

import (
	"os"
	"path/filepath"
	"sort"
)

type fixture struct {
	name  string
	files map[string]string
}

// corpus is the deterministic fixture set both implementations run against.
// Every entry exercises a documented behaviour of reveal-md: separators, front
// matter, asset discovery, collation, encoding and configuration loading.
func corpus() []fixture {
	return []fixture{
		{
			name: "basic",
			files: map[string]string{
				"slides.md": "# Slide A\n\nSome content\n\n---\n\n# Slide B\n\nNote: a speaker note\n",
				"a.md":      "Foo\n\nNote: test note\n\n---\n\nBar\n\n----\n\nSub Bar\n\n---\n\nThe End.\n",
			},
		},
		{
			name: "unicode",
			files: map[string]string{
				"cjk.md":       "# 日本語のスライド\n\n漢字とひらがな\n",
				"rtl.md":       "# مرحبا بالعالم\n\nنص عربي\n",
				"emoji.md":     "# Astral \U0001F680\U0001F1F3\U0001F1F1\n\nFamily: \U0001F468\u200D\U0001F469\u200D\U0001F467\n",
				"combining.md": "# Cafe\u0301 vs Caf\u00e9\n\nNFD then NFC\n",
			},
		},
		{
			name: "escaping",
			files: map[string]string{
				"escape.md": "---\ntitle: \"<script>alert('x') & \\\"quotes\\\" `tick` = / done\"\n---\n\n# Escaping\n",
			},
		},
		{
			name: "frontmatter",
			files: map[string]string{
				"full.md":       "---\ntitle: Full\ntheme: solarized\nseparator: <!--s-->\nverticalSeparator: <!--v-->\nrevealOptions:\n  transition: fade\n  width: 1280\n  height: 720\n  controls: false\n  keyboard: null\n  nested:\n    deep: [1, 2, 3]\n---\n\nSlide A<!--s-->Slide B<!--v-->Sub\n",
				"onlyfront.md":  "---\ntitle: Only Front Matter\n---\n",
				"noyaml.md":     "# No front matter here\n",
				"blankfirst.md": "\n---\ntitle: Not Front Matter\n---\nBody\n",
				"crlf.md":       "---\r\ntitle: CRLF\r\n---\r\nSlide A\r\n\r\n---\r\n\r\nSlide B\r\n",
				"bom.md":        "\ufeff---\ntitle: BOM\n---\nSlide\n",
				"notrailing.md": "# No trailing newline",
				"numbers.md":    "---\ntitle: Numbers\nrevealOptions:\n  a: 017\n  b: 0x1F\n  c: 1:30\n  d: yes\n  e: True\n  f: ~\n  g: .5\n  h: 1_000\n---\n\nSlide\n",
			},
		},
		{
			name: "collation",
			files: map[string]string{
				"b.md":  "# b\n",
				"A.md":  "# A\n",
				"a.md":  "# a\n",
				"Z.md":  "# Z\n",
				"á.md":  "# a-acute\n",
				"10.md": "# ten\n",
				"2.md":  "# two\n",
				"_x.md": "# underscore\n",
			},
		},
		{
			name: "names",
			files: map[string]string{
				"with space.md": "# Space\n",
				"dash-name.md":  "# Dash\n",
				"UPPER.md":      "# Upper\n",
			},
		},
		{
			name: "images",
			files: map[string]string{
				"img.md":          "# Images\n\n![alt](assets/cat.jpg)\n\n<img src=\"assets/cat.jpg\" width=\"100\">\n\n<!-- .slide: data-background-image=\"assets/cat.jpg\" -->\n\n![remote](https://example.org/x.png)\n\n![missing](assets/nope.png)\n",
				"assets/cat.jpg":  "not really a jpeg, but bytes are bytes\n",
				"sub/deep/why.md": "# Deep\n\n![up](../../assets/cat.jpg)\n",
			},
		},
		{
			name: "nested",
			files: map[string]string{
				"index.md":     "# Index\n",
				"a/b/c.md":     "# Deep C\n",
				"a/b/c/d.md":   "# Deeper D\n",
				"a/readme.txt": "not markdown\n",
			},
		},
		{
			name: "json5config",
			files: map[string]string{
				"reveal-md.json5": "{\n  // a comment\n  title: 'JSON5 Title',\n  theme: 'moon',\n  separator: '<!--s-->',\n}\n",
				"slides.md":       "Slide A<!--s-->Slide B\n",
			},
		},
		{
			name: "revealconfig",
			files: map[string]string{
				"reveal.json": "{\n  \"width\": 1280,\n  \"height\": 800,\n  \"transition\": \"zoom\"\n}\n",
				"slides.md":   "# Reveal config\n",
			},
		},
		{
			name: "mermaid",
			files: map[string]string{
				"chart.md": "# Mermaid\n\n```mermaid\ngraph TD;\n  A-->B;\n```\n",
			},
		},
		{
			name:  "emptydir",
			files: map[string]string{"readme.txt": "no markdown here\n"},
		},
		{
			name: "preprocessor",
			files: map[string]string{
				"slides.md": "# Slide A\n\ncontent\n\n# Slide B\n\ncontent\n\n#^ Sub slide\n\nmore content\n",
				"pre.mjs": `export default async function preprocess(markdown) {
  return markdown
    .split('\n')
    .map((line, index) => {
      if (index > 0 && /^#/.test(line)) {
        return /#\^/.test(line) ? '\n----\n\n' + line.replace('#^', '#') : '\n---\n\n' + line;
      }
      return line;
    })
    .join('\n');
}
`,
			},
		},
		{
			name: "assets",
			files: map[string]string{
				"slides.md": "# With assets\n",
				"one.css":   ".one { color: red }\n",
				"two.css":   ".two { color: blue }\n",
				"one.js":    "console.log('one');\n",
			},
		},
	}
}

// materialize writes names in sorted order so that on case-insensitive
// filesystems the fixture collapses to the same survivor on both sides.
func (f fixture) materialize(dir string) error {
	names := make([]string, 0, len(f.files))
	for name := range f.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := f.files[name]
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
