package jsutil

import "strings"

// This file reproduces node:path (POSIX flavour). Go's path/filepath is close
// but differs in enough edge cases to matter here: filepath.Join("a", "")
// keeps a trailing state Node does not, filepath.Rel returns an error where
// Node returns a string, and Node's Dirname("c.md") is "." while several Go
// helpers return "". reveal-md derives user-visible URLs and log lines from
// these functions, so the Node behaviour is reproduced exactly.
//
// All functions operate on forward slashes, matching glob's posix:true output.

// PathJoin implements path.posix.join.
//
//	join('a', 'b')    => 'a/b'
//	join('a/', '/b')  => 'a/b'
//	join('', 'b')     => 'b'
//	join('a', '')     => 'a'
//	join('a', '..', 'b') => 'b'
//	join('.', 'x.md') => 'x.md'
func PathJoin(parts ...string) string {
	var joined string
	for _, p := range parts {
		if p == "" {
			continue
		}
		if joined == "" {
			joined = p
			continue
		}
		joined += "/" + p
	}
	if joined == "" {
		return "."
	}
	return PathNormalize(joined)
}

// PathResolve implements path.posix.resolve with an explicit cwd, so callers
// stay testable. Segments are processed right to left until an absolute path
// is found.
func PathResolve(cwd string, parts ...string) string {
	resolved := ""
	absolute := false
	for i := len(parts) - 1; i >= 0 && !absolute; i-- {
		p := parts[i]
		if p == "" {
			continue
		}
		if resolved == "" {
			resolved = p
		} else {
			resolved = p + "/" + resolved
		}
		absolute = strings.HasPrefix(p, "/")
	}
	if !absolute {
		if resolved == "" {
			resolved = cwd
		} else {
			resolved = cwd + "/" + resolved
		}
		absolute = strings.HasPrefix(cwd, "/")
	}
	out := normalizePath(resolved, absolute)
	if absolute {
		if !strings.HasPrefix(out, "/") {
			return "/" + out
		}
		return out
	}
	if out == "" {
		return "."
	}
	return out
}

// PathNormalize implements path.posix.normalize.
func PathNormalize(p string) string {
	if p == "" {
		return "."
	}
	isAbs := strings.HasPrefix(p, "/")
	trailing := strings.HasSuffix(p, "/")
	out := normalizePath(p, isAbs)
	if out == "" && !isAbs {
		if trailing {
			return "./"
		}
		return "."
	}
	if trailing && !strings.HasSuffix(out, "/") {
		out += "/"
	}
	if isAbs {
		return "/" + strings.TrimPrefix(out, "/")
	}
	return out
}

// normalizePath collapses "." and ".." segments. When the path is relative,
// leading ".." segments are preserved (Node keeps them); when absolute they
// are discarded, because "/.." is "/".
func normalizePath(p string, allowAboveRoot bool) string {
	segments := strings.Split(p, "/")
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 && out[len(out)-1] != ".." {
				out = out[:len(out)-1]
				continue
			}
			if !allowAboveRoot {
				out = append(out, "..")
			}
		default:
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}

// PathRelative implements path.posix.relative for the relative-path inputs
// reveal-md uses.
//
//	relative('a/b', 'a/b/c') => 'c'
//	relative('a/b/c', 'a')   => '../..'
//	relative('.', 'sub/c.md')=> 'sub/c.md'
//	relative('sub', '.')     => '..'
//	relative('.', '.')       => ''
func PathRelative(from, to string) string {
	if from == to {
		return ""
	}
	fromAbs := PathResolve("/", from)
	toAbs := PathResolve("/", to)
	if fromAbs == toAbs {
		return ""
	}
	fromParts := splitNonEmpty(fromAbs)
	toParts := splitNonEmpty(toAbs)

	common := 0
	for common < len(fromParts) && common < len(toParts) && fromParts[common] == toParts[common] {
		common++
	}
	out := make([]string, 0, len(fromParts)-common+len(toParts)-common)
	for i := common; i < len(fromParts); i++ {
		out = append(out, "..")
	}
	out = append(out, toParts[common:]...)
	return strings.Join(out, "/")
}

func splitNonEmpty(p string) []string {
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// PathBasename implements path.posix.basename(p) — trailing slashes are
// ignored, and basename('/') is "".
func PathBasename(p string) string {
	if p == "" {
		return ""
	}
	trimmed := strings.TrimRight(p, "/")
	if trimmed == "" {
		return ""
	}
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

// PathBasenameExt implements path.posix.basename(p, ext): the extension is
// stripped only when it is a proper suffix and not the entire name.
func PathBasenameExt(p, ext string) string {
	base := PathBasename(p)
	if ext != "" && base != ext && strings.HasSuffix(base, ext) {
		return base[:len(base)-len(ext)]
	}
	return base
}

// PathDirname implements path.posix.dirname: "c.md" => ".", "/a" => "/".
func PathDirname(p string) string {
	if p == "" {
		return "."
	}
	isAbs := strings.HasPrefix(p, "/")
	trimmed := strings.TrimRight(p, "/")
	if trimmed == "" {
		return "/"
	}
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		if isAbs {
			return "/"
		}
		return "."
	}
	if idx == 0 {
		return "/"
	}
	return trimmed[:idx]
}

// PathExtname implements path.posix.extname. A leading dot does not start an
// extension: extname('.md') is "".
func PathExtname(p string) string {
	base := PathBasename(p)
	idx := strings.LastIndex(base, ".")
	if idx <= 0 {
		return ""
	}
	return base[idx:]
}

// PathIsAbsolute implements path.posix.isAbsolute.
func PathIsAbsolute(p string) bool { return strings.HasPrefix(p, "/") }
