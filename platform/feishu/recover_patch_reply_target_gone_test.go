package feishu

import (
	"context"
	"errors"
	"testing"

	core "github.com/chenhg5/cc-connect/core"
)

// Regression: when someone withdraws the bot's own progress card, the next
// patchSingleCard returns 230011. recoverPatchError must treat this the
// same way as errKindExpired — send a fresh card via sendCardToChat and
// rewrite the handle's messageIDs slot so the writer can keep updating.
func TestRecoverPatchError_ReplyTargetGone_MatchesExpiredPath(t *testing.T) {
	if classifyFeishuError(&feishuAPIError{Code: 230011}) != errKindReplyTargetGone {
		t.Fatal("precondition: 230011 must classify as errKindReplyTargetGone")
	}

	// The expired branch requires (progressPayload != nil) && (h.chatID != "")
	// to attempt recovery; otherwise it returns the original error. We assert
	// errKindReplyTargetGone honours the same contract.
	h := &feishuPreviewHandle{messageIDs: []string{"om_old"}, chatID: "", rc: replyContext{}}
	p := &Platform{}
	err := p.recoverPatchError(
		context.Background(),
		&feishuAPIError{Code: 230011},
		h, 0, "om_old", "{}",
		&core.ProgressCardPayload{Items: []core.ProgressCardEntry{{Kind: core.ProgressEntryInfo, Text: "x"}}},
	)
	if err == nil {
		t.Fatalf("want original error returned when chatID empty, got nil")
	}
	var fe *feishuAPIError
	if !errors.As(err, &fe) || fe.Code != 230011 {
		t.Errorf("want wrapped 230011, got %v", err)
	}
	// No successful send happened, so messageIDs must be untouched.
	if len(h.messageIDs) != 1 || h.messageIDs[0] != "om_old" {
		t.Errorf("handle mutated on non-recoverable path: %v", h.messageIDs)
	}
}

// TestRecoverPatchError_ReplyTargetGone_UsesRecreatePath dispatches the same
// recovery flow the expired branch uses. Because sendCardToChat reaches the
// lark SDK we can't exercise the success path hermetically, so we instead
// assert the classifier + switch wiring: the 230011 branch must enter the
// recreate block (chatID != "" && payload != nil) rather than falling through
// to the default return.
func TestRecoverPatchError_ReplyTargetGone_UsesRecreatePath(t *testing.T) {
	// The branch we want to reach invokes p.sendCardToChat, which needs a
	// non-nil lark client. Providing a zero-value Platform causes sendCardToChat
	// to nil-deref. If the switch correctly entered the recreate branch we
	// expect panic-recovered error that mentions "send extra card" — the
	// function used to post a fresh card. Falling through to default would
	// instead return the original 230011 wrapped error unchanged.
	h := &feishuPreviewHandle{messageIDs: []string{"om_old"}, chatID: "oc_chat", rc: replyContext{messageID: "om_trigger"}}
	p := &Platform{}
	defer func() {
		if r := recover(); r != nil {
			// OK — we reached sendCardToChat which is the branch we wanted.
			// A real run would succeed.
			return
		}
	}()
	payload := &core.ProgressCardPayload{Items: []core.ProgressCardEntry{{Kind: core.ProgressEntryInfo, Text: "x"}}}
	_ = p.recoverPatchError(
		context.Background(),
		&feishuAPIError{Code: 230011},
		h, 0, "om_old", "{}",
		payload,
	)
	t.Fatalf("recoverPatchError did not enter sendCardToChat branch; 230011 must reach the recreate path")
}
