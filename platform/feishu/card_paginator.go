package feishu

import "encoding/json"

// paginateElements splits a flat list of card body elements into N pages so
// that every page stays within the given budgets. Each page's element count
// is <= maxElems, and each page's JSON byte length is <= maxBytes (best
// effort: a single element larger than maxBytes is placed alone rather than
// dropped, so data is never lost).
func paginateElements(elements []map[string]any, maxElems, maxBytes int) [][]map[string]any {
	if maxElems <= 0 {
		maxElems = 180
	}
	if maxBytes <= 0 {
		maxBytes = 28_000
	}
	var pages [][]map[string]any
	var cur []map[string]any
	curBytes := 0

	flush := func() {
		if len(cur) > 0 {
			pages = append(pages, cur)
			cur = nil
			curBytes = 0
		}
	}

	for _, el := range elements {
		raw, _ := json.Marshal(el)
		size := len(raw) + 1
		exceeds := len(cur)+1 > maxElems || (len(cur) > 0 && curBytes+size > maxBytes)
		if exceeds {
			flush()
		}
		cur = append(cur, el)
		curBytes += size
	}
	flush()
	return pages
}
