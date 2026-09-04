package jsutil

import (
	"fmt"
	"strings"
)

// This file is a port of mustache.js 4.2.0, limited to the features the two
// reveal-md templates use but faithful to the details that change output
// bytes. A third-party Go mustache package was rejected because:
//
//   - the HTML escape table differs (mustache.js escapes ` = / as well as
//     & < > " ', and uses &#39; not &#x27;),
//   - standalone-line stripping rules differ, and the templates rely on them
//     (lib/template/reveal.html has section tags on their own lines),
//   - lookup semantics for missing/false/zero values differ.
//
// Every rule below was confirmed against mustache 4.2.0 in the oracle project.

// escapeHTML is mustache.js's escapeHtml. The entity spellings are part of the
// byte-for-byte contract with the JavaScript implementation.
var escapeHTML = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
	"`", "&#x60;",
	"=", "&#x3D;",
	"/", "&#x2F;",
)

// EscapeHTML exposes the mustache.js escaper for callers that need it.
func EscapeHTML(s string) string { return escapeHTML.Replace(s) }

type tokenType int

const (
	tokenText      tokenType = iota
	tokenVariable            // {{ name }}   escaped
	tokenUnescaped           // {{{ name }}} or {{& name }}
	tokenSection             // {{# name }}
	tokenInverted            // {{^ name }}
	tokenComment             // {{! comment }}
	tokenPartial             // {{> name }}
	tokenClose               // {{/ name }}
)

type token struct {
	kind     tokenType
	name     string
	children []token
}

// RenderMustache renders a mustache template against a context object.
func RenderMustache(template string, ctx *Object) (string, error) {
	return RenderMustachePartials(template, ctx, nil)
}

// RenderMustachePartials renders a template with a partial resolver.
func RenderMustachePartials(template string, ctx *Object, partials map[string]string) (string, error) {
	tokens, err := parseMustache(template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	stack := []any{ctx}
	if err := renderTokens(&b, tokens, stack, partials, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// parseMustache tokenises the template and nests section tokens.
func parseMustache(tmpl string) ([]token, error) {
	flat, err := scanMustache(tmpl)
	if err != nil {
		return nil, err
	}
	return nestTokens(flat)
}

type rawToken struct {
	kind    tokenType
	name    string
	deleted bool
}

// scanMustache produces a flat token list, reproducing mustache.js's
// parseTemplate: whitespace-only text tokens are recorded per line and, at
// every newline, dropped when the line carried at least one tag and no
// non-space content. A line may hold SEVERAL tags, so the rule cannot be
// expressed as lookahead around a single tag: reveal.html's
// "{{/cssPaths}} {{#watch}}" line depends on the space between the two tags
// being stripped as well.
func scanMustache(tmpl string) ([]rawToken, error) {
	var tokens []rawToken
	var spaces []int
	hasTag := false
	nonSpace := false

	stripSpace := func() {
		if hasTag && !nonSpace {
			for len(spaces) > 0 {
				tokens[spaces[len(spaces)-1]].deleted = true
				spaces = spaces[:len(spaces)-1]
			}
		} else {
			spaces = spaces[:0]
		}
		hasTag = false
		nonSpace = false
	}

	scanText := func(text string) {
		for _, chr := range text {
			if strings.ContainsRune(jsWhitespace, chr) {
				spaces = append(spaces, len(tokens))
			} else {
				nonSpace = true
			}
			tokens = append(tokens, rawToken{kind: tokenText, name: string(chr)})
			if chr == '\n' {
				stripSpace()
			}
		}
	}

	pos := 0
	for pos < len(tmpl) {
		open := strings.Index(tmpl[pos:], "{{")
		if open < 0 {
			scanText(tmpl[pos:])
			break
		}
		open += pos
		scanText(tmpl[pos:open])

		body := tmpl[open+2:]
		var kind tokenType
		var closeTag string
		switch {
		case strings.HasPrefix(body, "{"):
			kind, closeTag, body = tokenUnescaped, "}}}", body[1:]
		case strings.HasPrefix(body, "&"):
			kind, closeTag, body = tokenUnescaped, "}}", body[1:]
		case strings.HasPrefix(body, "#"):
			kind, closeTag, body = tokenSection, "}}", body[1:]
		case strings.HasPrefix(body, "^"):
			kind, closeTag, body = tokenInverted, "}}", body[1:]
		case strings.HasPrefix(body, "/"):
			kind, closeTag, body = tokenClose, "}}", body[1:]
		case strings.HasPrefix(body, "!"):
			kind, closeTag, body = tokenComment, "}}", body[1:]
		case strings.HasPrefix(body, ">"):
			kind, closeTag, body = tokenPartial, "}}", body[1:]
		default:
			kind, closeTag = tokenVariable, "}}"
		}
		end := strings.Index(body, closeTag)
		if end < 0 {
			return nil, fmt.Errorf("unclosed tag at position %d", open)
		}
		name := strings.TrimSpace(body[:end])
		pos = open + 2 + (len(tmpl[open+2:]) - len(body)) + end + len(closeTag)

		hasTag = true
		if kind == tokenVariable || kind == tokenUnescaped {
			nonSpace = true
		}
		tokens = append(tokens, rawToken{kind: kind, name: name})
	}
	stripSpace()

	out := make([]rawToken, 0, len(tokens))
	for _, t := range tokens {
		if t.deleted {
			continue
		}
		if t.kind == tokenText && len(out) > 0 && out[len(out)-1].kind == tokenText {
			out[len(out)-1].name += t.name
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

type openSection struct {
	kind            tokenType
	name            string
	tokensBeforeTag []token
}

// nestTokens turns the flat list into a tree, matching closing tags.
func nestTokens(flat []rawToken) ([]token, error) {
	var stack []openSection
	current := []token{}

	for _, rt := range flat {
		switch rt.kind {
		case tokenSection, tokenInverted:
			stack = append(stack, openSection{kind: rt.kind, name: rt.name, tokensBeforeTag: current})
			current = []token{}
		case tokenClose:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unopened section %q", rt.name)
			}
			open := stack[len(stack)-1]
			if open.name != rt.name {
				return nil, fmt.Errorf("unclosed section %q", open.name)
			}
			stack = stack[:len(stack)-1]
			children := current
			current = append(open.tokensBeforeTag, token{kind: open.kind, name: open.name, children: children})
		default:
			current = append(current, token{kind: rt.kind, name: rt.name})
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unclosed section %q", stack[len(stack)-1].name)
	}
	return current, nil
}

const maxMustacheDepth = 32

func renderTokens(b *strings.Builder, tokens []token, stack []any, partials map[string]string, depth int) error {
	if depth > maxMustacheDepth {
		return fmt.Errorf("mustache: partial recursion too deep")
	}
	for _, t := range tokens {
		switch t.kind {
		case tokenText:
			b.WriteString(t.name)
		case tokenComment:
			// nothing
		case tokenVariable:
			b.WriteString(EscapeHTML(mustacheString(lookup(stack, t.name))))
		case tokenUnescaped:
			b.WriteString(mustacheString(lookup(stack, t.name)))
		case tokenSection:
			if err := renderSection(b, t, stack, partials, depth); err != nil {
				return err
			}
		case tokenInverted:
			v := lookup(stack, t.name)
			if !sectionTruthy(v) {
				if err := renderTokens(b, t.children, stack, partials, depth); err != nil {
					return err
				}
			}
		case tokenPartial:
			src, ok := partials[t.name]
			if !ok {
				continue
			}
			sub, err := parseMustache(src)
			if err != nil {
				return err
			}
			if err := renderTokens(b, sub, stack, partials, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderSection(b *strings.Builder, t token, stack []any, partials map[string]string, depth int) error {
	v := lookup(stack, t.name)
	switch val := v.(type) {
	case nil, Undefined:
		return nil
	case bool:
		if !val {
			return nil
		}
		return renderTokens(b, t.children, stack, partials, depth)
	case []any:
		// An empty list renders nothing; otherwise each item becomes the
		// innermost context.
		for _, item := range val {
			if err := renderTokens(b, t.children, append(stack, item), partials, depth); err != nil {
				return err
			}
		}
		return nil
	case *Object:
		return renderTokens(b, t.children, append(stack, val), partials, depth)
	default:
		// Scalars are pushed onto the stack so {{.}} resolves to them, which
		// is what mustache.js does for non-empty strings and numbers.
		if !sectionTruthy(v) {
			return nil
		}
		return renderTokens(b, t.children, append(stack, v), partials, depth)
	}
}

// sectionTruthy implements mustache.js's section test. It differs from plain
// JavaScript truthiness in exactly one way: an EMPTY ARRAY is falsy, so
// {{^files}}...{{/files}} renders when there are no files.
func sectionTruthy(v any) bool {
	if arr, ok := v.([]any); ok {
		return len(arr) > 0
	}
	return Truthy(v)
}

// lookup resolves a (possibly dotted) name against the context stack,
// innermost first. Missing names yield Undefined, and a missing intermediate
// segment aborts the lookup rather than raising.
func lookup(stack []any, name string) any {
	if name == "." {
		if len(stack) == 0 {
			return Undef
		}
		return stack[len(stack)-1]
	}
	parts := strings.Split(name, ".")
	for i := len(stack) - 1; i >= 0; i-- {
		obj, ok := stack[i].(*Object)
		if !ok {
			continue
		}
		if !obj.Has(parts[0]) {
			continue
		}
		var cur any = obj.Get(parts[0])
		resolved := true
		for _, seg := range parts[1:] {
			child, ok := cur.(*Object)
			if !ok || !child.Has(seg) {
				resolved = false
				break
			}
			cur = child.Get(seg)
		}
		if resolved {
			return cur
		}
		return Undef
	}
	return Undef
}

// mustacheString converts a looked-up value to its output text. null and
// undefined render as the empty string, but false renders as "false" and 0
// renders as "0" — verified against mustache 4.2.0.
func mustacheString(v any) string {
	switch v.(type) {
	case nil, Undefined:
		return ""
	}
	return JSString(v)
}
