// core/chat_settings_test.go
package core

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestChatSettings_GetFallback(t *testing.T) {
	cs := NewChatSettings()

	// No overrides set — should return nil
	if v := cs.Get("chat1", "sess1", "quiet"); v != nil {
		t.Fatalf("expected nil, got %v", v)
	}

	// Set chat-level, session query should inherit
	cs.SetChat("chat1", "quiet", true)
	if v := cs.Get("chat1", "sess1", "quiet"); v != true {
		t.Fatalf("expected true from chat fallback, got %v", v)
	}

	// Set session-level, should override chat
	cs.SetSession("sess1", "quiet", false)
	if v := cs.Get("chat1", "sess1", "quiet"); v != false {
		t.Fatalf("expected false from session override, got %v", v)
	}
}

func TestChatSettings_DeleteRestoresFallback(t *testing.T) {
	cs := NewChatSettings()
	cs.SetChat("chat1", "quiet", true)
	cs.SetSession("sess1", "quiet", false)

	cs.DeleteSession("sess1", "quiet")
	if v := cs.Get("chat1", "sess1", "quiet"); v != true {
		t.Fatalf("after delete session, expected chat fallback true, got %v", v)
	}

	cs.DeleteChat("chat1", "quiet")
	if v := cs.Get("chat1", "sess1", "quiet"); v != nil {
		t.Fatalf("after delete chat, expected nil, got %v", v)
	}
}

func TestChatSettings_SnapshotRoundTrip(t *testing.T) {
	cs := NewChatSettings()
	cs.SetChat("oc_abc", "thread_isolation", false)
	cs.SetChat("oc_abc", "language", "zh")
	cs.SetSession("feishu:oc_abc:root:om_001", "quiet", true)

	chatSnap, sessSnap := cs.Snapshot()

	cs2 := NewChatSettings()
	cs2.Load(chatSnap, sessSnap)

	// Verify round-trip
	if v := cs2.Get("oc_abc", "", "thread_isolation"); v != false {
		t.Fatalf("round-trip: expected false, got %v", v)
	}
	if v := cs2.Get("oc_abc", "", "language"); v != "zh" {
		t.Fatalf("round-trip: expected zh, got %v", v)
	}
	if v := cs2.Get("oc_abc", "feishu:oc_abc:root:om_001", "quiet"); v != true {
		t.Fatalf("round-trip: expected true, got %v", v)
	}
}

func TestChatSettings_JSONRoundTrip(t *testing.T) {
	cs := NewChatSettings()
	cs.SetChat("oc_abc", SettingThreadIsolation, false)
	cs.SetChat("oc_abc", SettingLanguage, "zh")
	cs.SetSession("feishu:oc_abc:root:om_001", SettingQuiet, true)

	chatSnap, sessSnap := cs.Snapshot()

	// Simulate real persistence: marshal → unmarshal through sessionSnapshot
	blob, err := json.Marshal(struct {
		ChatSettings    map[string]map[string]any `json:"chat_settings,omitempty"`
		SessionSettings map[string]map[string]any `json:"session_settings,omitempty"`
	}{chatSnap, sessSnap})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored struct {
		ChatSettings    map[string]map[string]any `json:"chat_settings,omitempty"`
		SessionSettings map[string]map[string]any `json:"session_settings,omitempty"`
	}
	if err := json.Unmarshal(blob, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	cs2 := NewChatSettings()
	cs2.Load(restored.ChatSettings, restored.SessionSettings)

	// bool survives JSON round-trip
	if v := cs2.Get("oc_abc", "", SettingThreadIsolation); v != false {
		t.Fatalf("JSON round-trip: thread_isolation expected false, got %v (%T)", v, v)
	}
	// string survives
	if v := cs2.Get("oc_abc", "", SettingLanguage); v != "zh" {
		t.Fatalf("JSON round-trip: language expected zh, got %v (%T)", v, v)
	}
	// bool in session-level survives
	if v := cs2.Get("", "feishu:oc_abc:root:om_001", SettingQuiet); v != true {
		t.Fatalf("JSON round-trip: quiet expected true, got %v (%T)", v, v)
	}
}

func TestChatSettings_ConcurrentAccess(t *testing.T) {
	cs := NewChatSettings()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			cs.SetChat("chat1", "quiet", true)
		}()
		go func() {
			defer wg.Done()
			_ = cs.Get("chat1", "", "quiet")
		}()
		go func() {
			defer wg.Done()
			cs.DeleteChat("chat1", "quiet")
		}()
		go func() {
			defer wg.Done()
			_, _ = cs.Snapshot()
		}()
	}
	wg.Wait()
}
