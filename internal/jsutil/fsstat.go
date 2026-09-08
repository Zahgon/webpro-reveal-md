package jsutil

import (
	"os"
	"sync"
)

type statResult struct {
	value bool
	err   error
}

var (
	statMu      sync.Mutex
	isDirCache  = map[string]statResult{}
	isFileCache = map[string]statResult{}
)

// IsDirectory ports util.js isDirectory, including its _.memoize cache: the
// result, error included, is computed once per argument. The error it returns
// is a Node style system error because the CLI prints it verbatim.
func IsDirectory(dir string) (bool, error) {
	return cachedStat(isDirCache, dir, func(info os.FileInfo) bool { return info.IsDir() })
}

// IsFile ports util.js isFile.
func IsFile(p string) (bool, error) {
	return cachedStat(isFileCache, p, func(info os.FileInfo) bool { return info.Mode().IsRegular() })
}

func cachedStat(cache map[string]statResult, key string, pick func(os.FileInfo) bool) (bool, error) {
	statMu.Lock()
	defer statMu.Unlock()
	if hit, ok := cache[key]; ok {
		return hit.value, hit.err
	}
	info, err := os.Stat(key)
	result := statResult{}
	if err != nil {
		result.err = NewSystemError(err, "stat", key)
	} else {
		result.value = pick(info)
	}
	cache[key] = result
	return result.value, result.err
}

// ResetStatCache clears the memoisation so tests can observe a fresh process.
func ResetStatCache() {
	statMu.Lock()
	defer statMu.Unlock()
	isDirCache = map[string]statResult{}
	isFileCache = map[string]statResult{}
}
