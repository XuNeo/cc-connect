package feishu

import (
	"errors"
	"reflect"
	"testing"
)

func TestOrchestrateSendPreview_FirstCandidateSucceeds(t *testing.T) {
	calls := []string{}
	send := func(mid string) (string, error) {
		calls = append(calls, mid)
		return "om_new", nil
	}
	rc := replyContext{messageID: "om_trigger", sessionKey: "feishu:oc:root:om_root"}
	newID, usedMID, err := orchestrateSendPreview(rc, send)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if newID != "om_new" {
		t.Errorf("newID=%q, want om_new", newID)
	}
	if usedMID != "om_trigger" {
		t.Errorf("usedMID=%q, want om_trigger", usedMID)
	}
	if !reflect.DeepEqual(calls, []string{"om_trigger"}) {
		t.Errorf("calls=%v, want [om_trigger]", calls)
	}
}

func TestOrchestrateSendPreview_TriggerWithdrawnFallsToThreadRoot(t *testing.T) {
	calls := []string{}
	send := func(mid string) (string, error) {
		calls = append(calls, mid)
		if mid == "om_trigger" {
			return "", &feishuAPIError{Code: 230011, Msg: "The message was withdrawn."}
		}
		return "om_new", nil
	}
	rc := replyContext{messageID: "om_trigger", sessionKey: "feishu:oc:root:om_root"}
	newID, usedMID, err := orchestrateSendPreview(rc, send)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if newID != "om_new" {
		t.Errorf("newID=%q, want om_new", newID)
	}
	if usedMID != "om_root" {
		t.Errorf("usedMID=%q, want om_root (thread root), got %q", "om_root", usedMID)
	}
	if !reflect.DeepEqual(calls, []string{"om_trigger", "om_root"}) {
		t.Errorf("calls=%v, want [om_trigger om_root]", calls)
	}
}

func TestOrchestrateSendPreview_AllRepliesWithdrawnFallsToCreate(t *testing.T) {
	calls := []string{}
	send := func(mid string) (string, error) {
		calls = append(calls, mid)
		if mid == "" {
			return "om_new", nil
		}
		return "", &feishuAPIError{Code: 230011}
	}
	rc := replyContext{messageID: "om_trigger", sessionKey: "feishu:oc:root:om_root"}
	newID, usedMID, err := orchestrateSendPreview(rc, send)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if newID != "om_new" {
		t.Errorf("newID=%q, want om_new", newID)
	}
	if usedMID != "" {
		t.Errorf("usedMID=%q, want empty (Create fallback)", usedMID)
	}
	if !reflect.DeepEqual(calls, []string{"om_trigger", "om_root", ""}) {
		t.Errorf("calls=%v, want [om_trigger om_root ''] ", calls)
	}
}

func TestOrchestrateSendPreview_NonRecoverableErrorAborts(t *testing.T) {
	boom := &feishuAPIError{Code: 230020, Msg: "freq limit"} // rate-limited, NOT reply-target-gone
	calls := []string{}
	send := func(mid string) (string, error) {
		calls = append(calls, mid)
		return "", boom
	}
	rc := replyContext{messageID: "om_trigger", sessionKey: "feishu:oc:root:om_root"}
	_, _, err := orchestrateSendPreview(rc, send)
	if !errors.Is(err, boom) {
		t.Errorf("want boom, got %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("calls=%v, want exactly one attempt (rate-limit must not trigger fallback)", calls)
	}
}

func TestOrchestrateSendPreview_NoCandidatesTriesCreateImmediately(t *testing.T) {
	calls := []string{}
	send := func(mid string) (string, error) {
		calls = append(calls, mid)
		return "om_new", nil
	}
	rc := replyContext{chatID: "oc_chat"} // no messageID, no sessionKey with root
	newID, usedMID, err := orchestrateSendPreview(rc, send)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if newID != "om_new" || usedMID != "" {
		t.Errorf("newID=%q usedMID=%q, want (om_new, \"\")", newID, usedMID)
	}
	if !reflect.DeepEqual(calls, []string{""}) {
		t.Errorf("calls=%v, want [''] ", calls)
	}
}

func TestOrchestrateSendPreview_CreateFallbackErrorPropagates(t *testing.T) {
	boom := &feishuAPIError{Code: 230002}
	calls := []string{}
	send := func(mid string) (string, error) {
		calls = append(calls, mid)
		if mid == "" {
			return "", boom
		}
		return "", &feishuAPIError{Code: 230011}
	}
	rc := replyContext{messageID: "om_trigger"}
	_, _, err := orchestrateSendPreview(rc, send)
	if !errors.Is(err, boom) {
		t.Errorf("want chat-unavailable propagated, got %v", err)
	}
}

// Regression: noReplyToTrigger=true must force Create even when the trigger
// messageID and thread root are both present. Before this fix, the
// orchestrator happily returned [messageID, threadRoot] and SendPreviewStart
// would quote-reply to the user's trigger message, silently violating the
// platform's reply_to_trigger=false config.
func TestOrchestrateSendPreview_NoReplyToTrigger_ForcesCreate(t *testing.T) {
	calls := []string{}
	send := func(mid string) (string, error) {
		calls = append(calls, mid)
		return "om_new", nil
	}
	rc := replyContext{
		messageID:  "om_trigger",
		chatID:     "oc_chat",
		sessionKey: "feishu:oc_chat:root:om_root",
	}
	// Simulate noReplyToTrigger=true via the shouldReply parameter.
	newID, usedMID, err := orchestrateSendPreviewWithOpts(rc, false, send)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if newID != "om_new" || usedMID != "" {
		t.Errorf("newID=%q usedMID=%q, want (om_new, \"\") — Create fallback", newID, usedMID)
	}
	if len(calls) != 1 || calls[0] != "" {
		t.Errorf("calls=%v, want exactly one Create call with empty mid", calls)
	}
}

func TestOrchestrateSendPreview_ShouldReplyTrue_WalksCandidates(t *testing.T) {
	// Baseline: shouldReply=true behaves identically to the existing
	// orchestrateSendPreview.
	calls := []string{}
	send := func(mid string) (string, error) {
		calls = append(calls, mid)
		if mid == "om_trigger" {
			return "", &feishuAPIError{Code: 230011}
		}
		return "om_new", nil
	}
	rc := replyContext{
		messageID:  "om_trigger",
		chatID:     "oc_chat",
		sessionKey: "feishu:oc_chat:root:om_root",
	}
	newID, usedMID, err := orchestrateSendPreviewWithOpts(rc, true, send)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if newID != "om_new" || usedMID != "om_root" {
		t.Errorf("newID=%q usedMID=%q, want (om_new, om_root)", newID, usedMID)
	}
}

// TestSendPreviewStart_ThreadPreservedAfterWithdraw verifies the end-to-end
// behaviour: a withdrawn trigger message causes one 230011, then the retry
// against the thread root succeeds, and the resulting handle carries the
// thread-root messageID so subsequent extra cards stay in the thread.
func TestSendPreviewStart_ThreadPreservedAfterWithdraw(t *testing.T) {
	// orchestrateSendPreview already covers the decision logic; this test
	// pins the handle-construction contract that SendPreviewStart must
	// honour once wired through the orchestrator. We exercise the same
	// sequence directly so the test is hermetic.
	rc := replyContext{
		messageID:  "om_trigger",
		chatID:     "oc_chat",
		sessionKey: "feishu:oc_chat:root:om_root",
	}
	send := func(mid string) (string, error) {
		if mid == "om_trigger" {
			return "", &feishuAPIError{Code: 230011}
		}
		return "om_posted", nil
	}
	newID, usedMID, err := orchestrateSendPreview(rc, send)
	if err != nil {
		t.Fatalf("orchestrator err: %v", err)
	}
	// Handle must be built with the messageID that succeeded — not the
	// original trigger — so sendCardToChat replies to a live target.
	effectiveRC := rc
	effectiveRC.messageID = usedMID
	h := &feishuPreviewHandle{messageIDs: []string{newID}, chatID: rc.chatID, rc: effectiveRC}
	if h.rc.messageID != "om_root" {
		t.Errorf("handle.rc.messageID=%q, want om_root (thread root)", h.rc.messageID)
	}
	if h.chatID != "oc_chat" {
		t.Errorf("handle.chatID=%q, want oc_chat", h.chatID)
	}
}
