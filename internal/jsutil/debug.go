package jsutil

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

const debugNamespace = "reveal-md"

var (
	debugOnce    sync.Once
	debugEnabled bool
)

// DebugEnabled mirrors the debug package's namespace matching for the single
// namespace this project uses, so DEBUG=reveal-md and DEBUG=* both enable it.
func DebugEnabled() bool {
	debugOnce.Do(func() {
		debugEnabled = debugSpecEnables(os.Getenv("DEBUG"))
	})
	return debugEnabled
}

func debugSpecEnables(spec string) bool {
	for _, pattern := range strings.Split(spec, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" || strings.HasPrefix(pattern, "-") {
			continue
		}
		if pattern == "*" || pattern == debugNamespace ||
			(strings.HasSuffix(pattern, "*") && strings.HasPrefix(debugNamespace, strings.TrimSuffix(pattern, "*"))) {
			return true
		}
	}
	return false
}

func Debug(args ...any) {
	if !DebugEnabled() {
		return
	}
	parts := make([]string, len(args))
	for i, arg := range args {
		switch v := arg.(type) {
		case string:
			parts[i] = v
		case error:
			parts[i] = v.Error()
		default:
			parts[i] = StringifyOrEmpty(v)
		}
	}
	fmt.Fprintf(os.Stderr, "  %s %s\n", debugNamespace, strings.Join(parts, " "))
}
