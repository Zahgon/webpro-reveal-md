package jsutil

import (
	"sort"
	"sync"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

var (
	collatorOnce sync.Once
	collator     *collate.Collator
	collatorMu   sync.Mutex
)

func defaultCollator() *collate.Collator {
	collatorOnce.Do(func() {
		collator = collate.New(language.English)
	})
	return collator
}

// LocaleCompare implements String.prototype.localeCompare(other) under the
// default locale: ICU root collation, not byte order. It is what listing.js
// sorts filenames with, so "10.md" sorts before "2.md" and "a.md" before
// "A.md".
func LocaleCompare(a, b string) int {
	collatorMu.Lock()
	defer collatorMu.Unlock()
	return defaultCollator().CompareString(a, b)
}

// SortLocale sorts in place the way Array.prototype.sort with a
// localeCompare comparator does: stable, by collation order.
func SortLocale(values []string) {
	sort.SliceStable(values, func(i, j int) bool {
		return LocaleCompare(values[i], values[j]) < 0
	})
}
