// core/chat_settings.go
package core

import "sync"

// Setting key constants.
const (
	SettingThreadIsolation = "thread_isolation"
	SettingQuiet           = "quiet"
	SettingTTSMode         = "tts_mode"
	SettingLanguage        = "language"
	SettingRequireMention  = "require_mention"
)

// ChatSettings provides per-chat and per-session setting overrides
// with a three-layer fallback: session → chat → nil (caller uses config default).
type ChatSettings struct {
	mu               sync.RWMutex
	chatOverrides    map[string]map[string]any // chatID → {key: value}
	sessionOverrides map[string]map[string]any // sessionKey → {key: value}
}

func NewChatSettings() *ChatSettings {
	return &ChatSettings{
		chatOverrides:    make(map[string]map[string]any),
		sessionOverrides: make(map[string]map[string]any),
	}
}

// Get returns the effective value for a setting key.
// Lookup order: sessionOverrides[sessionKey] → chatOverrides[chatID] → nil.
func (cs *ChatSettings) Get(chatID, sessionKey, key string) any {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if sessionKey != "" {
		if m, ok := cs.sessionOverrides[sessionKey]; ok {
			if v, ok := m[key]; ok {
				return v
			}
		}
	}
	if chatID != "" {
		if m, ok := cs.chatOverrides[chatID]; ok {
			if v, ok := m[key]; ok {
				return v
			}
		}
	}
	return nil
}

// SetChat sets a setting override at the chat level.
func (cs *ChatSettings) SetChat(chatID, key string, value any) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.chatOverrides[chatID] == nil {
		cs.chatOverrides[chatID] = make(map[string]any)
	}
	cs.chatOverrides[chatID][key] = value
}

// SetSession sets a setting override at the session level, taking priority over chat-level.
func (cs *ChatSettings) SetSession(sessionKey, key string, value any) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.sessionOverrides[sessionKey] == nil {
		cs.sessionOverrides[sessionKey] = make(map[string]any)
	}
	cs.sessionOverrides[sessionKey][key] = value
}

// DeleteChat removes a single key from the chat-level overrides.
func (cs *ChatSettings) DeleteChat(chatID, key string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if m, ok := cs.chatOverrides[chatID]; ok {
		delete(m, key)
		if len(m) == 0 {
			delete(cs.chatOverrides, chatID)
		}
	}
}

// DeleteSession removes a single key from the session-level overrides.
func (cs *ChatSettings) DeleteSession(sessionKey, key string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if m, ok := cs.sessionOverrides[sessionKey]; ok {
		delete(m, key)
		if len(m) == 0 {
			delete(cs.sessionOverrides, sessionKey)
		}
	}
}

// Snapshot returns deep copies for serialization.
func (cs *ChatSettings) Snapshot() (map[string]map[string]any, map[string]map[string]any) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	chat := make(map[string]map[string]any, len(cs.chatOverrides))
	for k, m := range cs.chatOverrides {
		c := make(map[string]any, len(m))
		for k2, v := range m {
			c[k2] = v
		}
		chat[k] = c
	}
	sess := make(map[string]map[string]any, len(cs.sessionOverrides))
	for k, m := range cs.sessionOverrides {
		c := make(map[string]any, len(m))
		for k2, v := range m {
			c[k2] = v
		}
		sess[k] = c
	}
	return chat, sess
}

// Load restores state from deserialized data, deep-copying to avoid aliasing.
func (cs *ChatSettings) Load(chat, sess map[string]map[string]any) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.chatOverrides = deepCopySettings(chat)
	cs.sessionOverrides = deepCopySettings(sess)
}

func deepCopySettings(src map[string]map[string]any) map[string]map[string]any {
	if src == nil {
		return make(map[string]map[string]any)
	}
	dst := make(map[string]map[string]any, len(src))
	for k, m := range src {
		inner := make(map[string]any, len(m))
		for k2, v := range m {
			inner[k2] = v
		}
		dst[k] = inner
	}
	return dst
}
