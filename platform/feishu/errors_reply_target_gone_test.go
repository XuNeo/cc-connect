package feishu

import "testing"

func TestClassifyFeishuError_ReplyTargetGone(t *testing.T) {
	err := &feishuAPIError{Code: 230011, Msg: "The message was withdrawn."}
	if got := classifyFeishuError(err); got != errKindReplyTargetGone {
		t.Errorf("classifyFeishuError(230011) = %v, want errKindReplyTargetGone", got)
	}
}
