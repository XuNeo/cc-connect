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
