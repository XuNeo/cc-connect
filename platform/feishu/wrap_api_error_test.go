package feishu

import (
	"errors"
	"strings"
	"testing"
)

func TestWrapAPIError_PreservesClassification(t *testing.T) {
	err := wrapAPIError("tag", "send preview (reply)", 230011, "The message was withdrawn.")
	if err == nil {
		t.Fatal("wrapAPIError returned nil")
	}
	var fe *feishuAPIError
	if !errors.As(err, &fe) {
		t.Fatalf("wrapAPIError result does not unwrap to *feishuAPIError: %v", err)
	}
	if fe.Code != 230011 || fe.Msg != "The message was withdrawn." {
		t.Errorf("fields lost: code=%d msg=%q", fe.Code, fe.Msg)
	}
	if classifyFeishuError(err) != errKindReplyTargetGone {
		t.Errorf("classifier lost visibility through wrapAPIError")
	}
}

func TestWrapAPIError_IncludesOperationAndTag(t *testing.T) {
	err := wrapAPIError("feishu(vela)", "reply", 230020, "freq limit")
	msg := err.Error()
	for _, want := range []string{"feishu(vela)", "reply", "230020", "freq limit"} {
		if !strings.Contains(msg, want) {
			t.Errorf("wrapAPIError message missing %q: %s", want, msg)
		}
	}
}
