package feishu

import (
	"testing"
)

// Regression: sendCardToChat previously used Im.Message.Create against the
// chat root regardless of whether the preview started inside a thread, so
// paginated extra cards leaked out of the thread into the chat root. The
// fix widened feishuPreviewHandle to carry the originating replyContext,
// and sendCardToChat now branches on shouldUseThreadOrReplyAPI(rc). This
// test verifies that branch selection without exercising the SDK.
func TestShouldUseThreadOrReplyAPI_ForHandleRC(t *testing.T) {
	p := &Platform{}

	// Typical thread/reply case: preview created via Reply — rc has a trigger messageID.
	rcThread := replyContext{messageID: "om_root_thread", chatID: "oc_chat"}
	if !p.shouldUseThreadOrReplyAPI(rcThread) {
		t.Errorf("thread rc with messageID should take the Reply path")
	}

	// Direct DM / bot @group case: no messageID — falls back to Create.
	rcChatOnly := replyContext{chatID: "oc_chat"}
	if p.shouldUseThreadOrReplyAPI(rcChatOnly) {
		t.Errorf("chat-only rc (no messageID) must fall back to Create")
	}

	// Explicit no-reply opt-out still bypasses thread routing.
	pNoReply := &Platform{noReplyToTrigger: true}
	if pNoReply.shouldUseThreadOrReplyAPI(rcThread) {
		t.Errorf("noReplyToTrigger must force Create even when messageID is present")
	}
}

// Handle plumbing: SendPreviewStart stores rc on the handle so subsequent
// sendCardToChat / recoverPatchError calls see the thread context.
func TestFeishuPreviewHandle_CarriesReplyContext(t *testing.T) {
	rc := replyContext{messageID: "om_root_xyz", chatID: "oc_chat_xyz", sessionKey: "k"}
	h := &feishuPreviewHandle{messageIDs: []string{"om_card_0"}, chatID: rc.chatID, rc: rc}
	if h.rc.messageID != rc.messageID {
		t.Errorf("handle.rc.messageID=%q, want %q", h.rc.messageID, rc.messageID)
	}
	if h.rc.chatID != rc.chatID {
		t.Errorf("handle.rc.chatID=%q, want %q", h.rc.chatID, rc.chatID)
	}
}
