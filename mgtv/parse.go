// Package mgtv: HTML parsing for mgtv.com pages.
package mgtv

import (
	"regexp"
)

// clipIDRE matches /b/<clipId>/ or /b/<clipId>.html patterns in HTML.
var clipIDRE = regexp.MustCompile(`/b/(\d+)`)

// parseClipIDs extracts all unique clipIds from an HTML page.
func parseClipIDs(body []byte) []string {
	seen := map[string]bool{}
	var ids []string
	for _, m := range clipIDRE.FindAllSubmatch(body, -1) {
		id := string(m[1])
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
