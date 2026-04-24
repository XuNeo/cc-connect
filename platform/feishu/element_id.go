package feishu

import (
	"fmt"
	"strings"
)

// makeElementID returns a Feishu-legal element_id derived from a semantic
// prefix (e.g., "tool", "think", "bash") and a monotonic sequence number.
// Rules: <=20 chars, starts with [A-Za-z], remaining chars in [A-Za-z0-9_].
func makeElementID(prefix string, seq int) string {
	p := sanitizeIDPrefix(prefix)
	if p == "" {
		p = "e"
	}
	seqStr := fmt.Sprintf("%d", seq)
	// 20 char budget: 1 letter-prefix + "_" + seqStr ≤ 20, so seqStr ≤ 18.
	// Keep the least-significant digits (tail) for best uniqueness.
	if len(seqStr) > 18 {
		seqStr = seqStr[len(seqStr)-18:]
	}
	// Reserve at least 1 char for the prefix to keep the letter-start rule.
	maxPrefix := 20 - 1 - len(seqStr)
	if maxPrefix < 1 {
		maxPrefix = 1
	}
	if len(p) > maxPrefix {
		p = p[:maxPrefix]
	}
	return p + "_" + seqStr
}

func sanitizeIDPrefix(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			b.WriteRune(r)
		case r >= '0' && r <= '9' && i > 0:
			b.WriteRune(r)
		case r == '_' && i > 0:
			b.WriteRune('_')
		}
	}
	return b.String()
}
