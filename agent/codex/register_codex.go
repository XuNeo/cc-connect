//go:build !no_codex

package codex

import "github.com/chenhg5/cc-connect/core"

func init() {
	core.RegisterAgent("codex", New)
}
