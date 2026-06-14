//go:build !no_traecli

package codex

import "github.com/chenhg5/cc-connect/core"

func init() {
	core.RegisterAgent("traecli", NewTraeCLI)
}
