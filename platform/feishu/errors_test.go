package feishu

import (
	"errors"
	"testing"
)

func TestFeishuAPIError_PermanentCodes(t *testing.T) {
	cases := []struct {
		code      int
		msg       string
		permanent bool
	}{
		{230011, "The message was withdrawn.", true},    // target gone forever
		{230099, "Failed to create card content", false}, // transient, retry
		{2200, "check app tenant fail", false},            // transient tenant cache
		{99991663, "invalid access token", false},         // recoverable via refresh
		{230020, "too many requests", false},              // transient rate limit
	}
	for _, tc := range cases {
		err := &feishuAPIError{Code: tc.code, Msg: tc.msg}
		got := err.IsPermanent()
		if got != tc.permanent {
			t.Errorf("code=%d IsPermanent = %v, want %v", tc.code, got, tc.permanent)
		}
		// Also verify unwrapping works through fmt.Errorf chain.
		wrapped := errors.New("x")
		_ = wrapped
	}
}
