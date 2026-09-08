package jsutil

import (
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// GlobOptions mirrors the option subset util.js passes to glob@10's globSync:
// a cwd to resolve against, and ignore patterns. Results are always POSIX
// separated and relative to Cwd, which is what posix:true guarantees.
type GlobOptions struct {
	Cwd    string
	Ignore []string
}

// GlobSync implements globSync(pattern, {cwd, ignore, posix}).
//
// Ordering note: glob@10 returns entries in the order its internal walker
// happens to finish them, which is neither directory order nor sorted. This
// implementation returns them sorted by path instead; the SET of paths is
// identical, and every consumer in reveal-md either sorts (listing.js uses
// localeCompare) or is order-insensitive.
func GlobSync(pattern string, opts GlobOptions) ([]string, error) {
	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	patterns := expandBraces(pattern)
	matches := []string{}
	seen := map[string]bool{}
	err := walkGlobTree(cwd, "", opts, func(rel string, isDir bool) {
		for _, p := range patterns {
			if matchGlobSegments(p, rel) && !seen[rel] {
				seen[rel] = true
				matches = append(matches, rel)
				return
			}
		}
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func walkGlobTree(root, rel string, opts GlobOptions, visit func(string, bool)) error {
	dir := root
	if rel != "" {
		dir = path.Join(root, rel)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if rel == "" {
			return err
		}
		return nil
	}
	for _, entry := range entries {
		childRel := entry.Name()
		if rel != "" {
			childRel = rel + "/" + entry.Name()
		}
		if isIgnored(childRel, opts.Ignore) {
			continue
		}
		isDir := entry.IsDir()
		if entry.Type()&fs.ModeSymlink != 0 {
			info, statErr := os.Stat(path.Join(root, childRel))
			if statErr == nil && info.IsDir() {
				visit(childRel, true)
				continue
			}
		}
		visit(childRel, isDir)
		if isDir {
			if err := walkGlobTree(root, childRel, opts, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func isIgnored(rel string, ignore []string) bool {
	for _, pattern := range ignore {
		for _, p := range expandBraces(pattern) {
			if matchGlobSegments(p, rel) {
				return true
			}
			if strings.HasSuffix(p, "/**") && matchGlobSegments(strings.TrimSuffix(p, "/**"), rel) {
				return true
			}
		}
	}
	return false
}

// MatchGlob reports whether a POSIX path matches a glob pattern using
// minimatch's rules: ** spans path separators, * and ? do not, and neither
// matches a leading dot unless the pattern spells the dot out.
func MatchGlob(pattern, p string) bool {
	for _, expanded := range expandBraces(pattern) {
		if matchGlobSegments(expanded, p) {
			return true
		}
	}
	return false
}

func matchGlobSegments(pattern, p string) bool {
	return matchSegs(strings.Split(pattern, "/"), strings.Split(p, "/"))
}

func matchSegs(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		if len(pat) == 1 {
			for _, s := range segs {
				if strings.HasPrefix(s, ".") {
					return false
				}
			}
			return true
		}
		for i := 0; i <= len(segs); i++ {
			if matchSegs(pat[1:], segs[i:]) {
				return true
			}
			if i < len(segs) && strings.HasPrefix(segs[i], ".") {
				break
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if !matchOneSegment(pat[0], segs[0]) {
		return false
	}
	return matchSegs(pat[1:], segs[1:])
}

func matchOneSegment(pattern, name string) bool {
	if strings.HasPrefix(name, ".") && !strings.HasPrefix(pattern, ".") {
		return false
	}
	return matchHere(pattern, name)
}

func matchHere(pattern, name string) bool {
	pi, ni := 0, 0
	starPat, starName := -1, -1
	for ni < len(name) {
		switch {
		case pi < len(pattern) && pattern[pi] == '*':
			starPat, starName = pi, ni
			pi++
		case pi < len(pattern) && pattern[pi] == '?':
			pi++
			ni++
		case pi < len(pattern) && pattern[pi] == '[':
			end, ok := matchClass(pattern[pi:], name[ni])
			if !ok {
				if starPat < 0 {
					return false
				}
				starName++
				pi, ni = starPat+1, starName
				continue
			}
			pi += end
			ni++
		case pi < len(pattern) && pattern[pi] == name[ni]:
			pi++
			ni++
		default:
			if starPat < 0 {
				return false
			}
			starName++
			pi, ni = starPat+1, starName
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

func matchClass(pattern string, c byte) (int, bool) {
	i := 1
	negate := false
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		negate = true
		i++
	}
	matched := false
	first := true
	for i < len(pattern) && (pattern[i] != ']' || first) {
		first = false
		if i+2 < len(pattern) && pattern[i+1] == '-' && pattern[i+2] != ']' {
			if c >= pattern[i] && c <= pattern[i+2] {
				matched = true
			}
			i += 3
			continue
		}
		if pattern[i] == c {
			matched = true
		}
		i++
	}
	if i >= len(pattern) {
		return 0, false
	}
	return i + 1, matched != negate
}

func expandBraces(pattern string) []string {
	start := strings.IndexByte(pattern, '{')
	if start < 0 {
		return []string{pattern}
	}
	depth := 0
	for i := start; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				prefix, suffix := pattern[:start], pattern[i+1:]
				out := []string{}
				for _, alt := range splitAlternatives(pattern[start+1 : i]) {
					out = append(out, expandBraces(prefix+alt+suffix)...)
				}
				return out
			}
		}
	}
	return []string{pattern}
}

func splitAlternatives(body string) []string {
	parts := []string{}
	depth := 0
	current := strings.Builder{}
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
		}
		current.WriteByte(body[i])
	}
	parts = append(parts, current.String())
	return parts
}
