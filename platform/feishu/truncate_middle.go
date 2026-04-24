package feishu

import (
	"fmt"
	"unicode/utf8"
)

// truncateMiddle keeps the head (~55%) and tail (~45%) of s when its rune
// count exceeds maxRunes. The omitted middle is replaced with a marker that
// reports the exact number of omitted runes for data-loss transparency.
// Returns s unchanged when utf8.RuneCountInString(s) <= maxRunes.
func truncateMiddle(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	total := utf8.RuneCountInString(s)
	if total <= maxRunes {
		return s
	}
	// Build a probe marker to estimate its rune cost, so the budget
	// accounts for the marker's length. The final marker is rebuilt after
	// headRunes/tailRunes are finalized to report the true omitted count.
	probe := fmt.Sprintf("\n...omitted %d chars...\n", total-maxRunes)
	if isZhString(s) {
		probe = fmt.Sprintf("\n...省略 %d 字...\n", total-maxRunes)
	}
	budget := maxRunes - utf8.RuneCountInString(probe)
	if budget < 40 {
		budget = 40
	}
	headRunes := budget * 55 / 100
	tailRunes := budget - headRunes

	omitted := total - headRunes - tailRunes
	marker := fmt.Sprintf("\n...omitted %d chars...\n", omitted)
	if isZhString(s) {
		marker = fmt.Sprintf("\n...省略 %d 字...\n", omitted)
	}

	return runeHead(s, headRunes) + marker + runeTail(s, tailRunes)
}

func runeHead(s string, n int) string {
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

func runeTail(s string, n int) string {
	if n <= 0 {
		return ""
	}
	total := utf8.RuneCountInString(s)
	if n >= total {
		return s
	}
	skip := total - n
	i := 0
	for pos := range s {
		if i == skip {
			return s[pos:]
		}
		i++
	}
	return ""
}

func isZhString(s string) bool {
	// Cheap heuristic: at least one CJK unified ideograph in the first 200 bytes.
	head := s
	if len(head) > 200 {
		head = head[:200]
	}
	for _, r := range head {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}
