package feishu

import (
	"reflect"
	"testing"
)

func TestPreviewRetryMessageIDs_ThreadWithDistinctRoot(t *testing.T) {
	rc := replyContext{
		messageID:  "om_trigger",
		chatID:     "oc_chat",
		sessionKey: "feishu:oc_chat:root:om_thread_root",
	}
	got := previewRetryMessageIDs(rc)
	want := []string{"om_trigger", "om_thread_root"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPreviewRetryMessageIDs_TriggerEqualsRoot(t *testing.T) {
	rc := replyContext{
		messageID:  "om_same",
		chatID:     "oc_chat",
		sessionKey: "feishu:oc_chat:root:om_same",
	}
	got := previewRetryMessageIDs(rc)
	want := []string{"om_same"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("duplicate not deduped: got %v, want %v", got, want)
	}
}

func TestPreviewRetryMessageIDs_NoThreadSession(t *testing.T) {
	rc := replyContext{
		messageID:  "om_trigger",
		chatID:     "oc_chat",
		sessionKey: "feishu:oc_chat:u_user",
	}
	got := previewRetryMessageIDs(rc)
	want := []string{"om_trigger"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPreviewRetryMessageIDs_NoTriggerFallsBackToRootOnly(t *testing.T) {
	rc := replyContext{
		messageID:  "",
		chatID:     "oc_chat",
		sessionKey: "feishu:oc_chat:root:om_thread_root",
	}
	got := previewRetryMessageIDs(rc)
	want := []string{"om_thread_root"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPreviewRetryMessageIDs_NeitherTriggerNorRoot(t *testing.T) {
	rc := replyContext{chatID: "oc_chat"}
	got := previewRetryMessageIDs(rc)
	if len(got) != 0 {
		t.Errorf("got %v, want empty slice", got)
	}
}
