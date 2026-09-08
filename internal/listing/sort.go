package listing

import (
	"fmt"
	"os"
	"sort"

	"github.com/webpro/reveal-md/internal/jsutil"
)

func sortByFileName(metas []*jsutil.Object) {
	sort.SliceStable(metas, func(i, j int) bool {
		left := jsutil.JSString(metas[i].Get("fileName"))
		right := jsutil.JSString(metas[j].Get("fileName"))
		return jsutil.LocaleCompare(left, right) < 0
	})
}

func reportError(err error, path string) {
	fmt.Fprintln(os.Stderr, jsutil.NewSystemError(err, "open", path).Inspect())
}
