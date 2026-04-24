package feishu

import (
	"errors"
	"testing"
)

func TestClassifyFeishuError_RateLimit(t *testing.T) {
	err := &feishuAPIError{Code: 230020, Msg: "freq limit"}
	if c := classifyFeishuError(err); c != errKindRateLimited {
		t.Errorf("got %v, want rate-limited", c)
	}
}

func TestClassifyFeishuError_Expired(t *testing.T) {
	err := &feishuAPIError{Code: 230031, Msg: "card expired"}
	if c := classifyFeishuError(err); c != errKindExpired {
		t.Errorf("got %v, want expired", c)
	}
}

func TestClassifyFeishuError_TooComplex(t *testing.T) {
	for _, code := range []int{230099, 200800} {
		err := &feishuAPIError{Code: code}
		if c := classifyFeishuError(err); c != errKindTooComplex {
			t.Errorf("code=%d got %v, want too-complex", code, c)
		}
	}
}

func TestClassifyFeishuError_ChatUnavailable(t *testing.T) {
	err := &feishuAPIError{Code: 230002}
	if c := classifyFeishuError(err); c != errKindChatUnavailable {
		t.Errorf("got %v, want chat-unavailable", c)
	}
}

func TestClassifyFeishuError_Generic(t *testing.T) {
	if c := classifyFeishuError(errors.New("random")); c != errKindOther {
		t.Errorf("got %v, want other", c)
	}
}
