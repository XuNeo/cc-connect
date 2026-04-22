# /quiet Scope-Selection Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/quiet` actually stick in thread-isolation groups by mirroring `/atme`'s scope auto-selection (session when in a thread, chat otherwise), and adding `on/off/reset` subcommands so state is deterministic.

**Architecture:** Today `cmdQuiet` unconditionally calls `chatSettings.SetSession(msg.SessionKey, ...)`. In thread-isolation mode the `sessionKey` of a top-level `/quiet` message is derived from that message's own `MessageId`, producing a one-shot key that no other message will ever read. The fix: pick scope the same way `cmdAtme` does — if `msg.IsThread && msg.SessionKey != ""`, write to session; else write to chat (`SetChat(msg.ChatID, ...)`). DMs (no `ChatID`) keep writing to session for the user's personal session. We also add `on/off/reset` subcommands (parity with `/atme`) while keeping the no-arg toggle for backwards compatibility — no-arg now prints current status instead of flipping, matching `/atme` UX.

**Tech Stack:** Go 1.24, existing cc-connect test harness (`go test -tags no_web ./core/ -count=1`).

---

## Scope Check

Single-file change in `core/engine.go` (`cmdQuiet` + a small status helper) plus test adjustments in `core/engine_test.go`. No concurrency changes; `ChatSettings` already has a `sync.RWMutex`.

## File Structure

- `core/engine.go` (modify)
  - `cmdQuiet` (currently ~6328-6338): rewrite to pick scope via `msg.IsThread && msg.SessionKey != ""`, support `on`/`off`/`reset`/no-arg-status, and handle DM (`msg.ChatID == ""`) by writing session-level for the user's personal session.
  - Add `quietShowStatus` helper (mirror of `atmeShowStatus`).

- `core/engine_test.go` (modify)
  - Repurpose `TestCmdQuiet_TogglesPerSession` (10769-10800) → `TestCmdQuiet_ThreadScopeTogglesSession`: same intent but asserts session-scope writes only when `IsThread=true`.
  - Repurpose `TestCmdQuiet_TogglesDisplay` (4189-4212) → thread-scoped message so toggle still works.
  - Repurpose `TestCmdQuiet_SessionIsolation` (10802-10821) → `TestCmdQuiet_TopLevelScopeWritesChat`: verifies top-level `/quiet` writes chat layer and is visible across different per-message sessionKeys.
  - Repurpose `TestCmdQuiet_DoesNotWriteConfigToml` (10823-10841) → set `IsThread=true` so it exercises the session path without touching toml.
  - Repurpose `TestCmdQuiet_IgnoresArgs` (10843-10859) → `TestCmdQuiet_UnknownArgShowsUsage`: unknown arg now reports usage, does not toggle.
  - Repurpose `TestCmdQuiet_RapidToggle` (10861-10876) → `TestCmdQuiet_OnOffOnDeterministic`: use explicit `on`/`off` subcommands.
  - Keep `TestProcessInteractiveEvents_QuietOverrideSuppresses*` (1240, 1164) and `TestIsQuiet_*` / `TestIsThinkingHidden_QuietOverrideWins` unchanged — they exercise the `isQuiet` lookup path which we don't touch.
  - Add new tests (see tasks below).

## Concurrency Note

`ChatSettings.Get/SetSession/SetChat` already take `sync.RWMutex`. `cmdQuiet` is called from the command dispatcher on a per-message goroutine, same as `cmdAtme`. No locking changes.

## Backwards-Compatibility Note

Existing persisted `session_settings.*.quiet` entries in `~/.cc-connect/sessions/*.json` remain valid. Session-layer overrides continue to take priority over chat-layer in `ChatSettings.Get`, so any pre-fix per-thread tuning the user explicitly intended still wins. New top-level `/quiet` calls write chat-level, which becomes the default for that group. If the user wants a clean slate, they can run `/quiet reset`.

---

## Task 1: Add failing tests for thread-scope session write

**Files:**
- Modify: `core/engine_test.go`

- [ ] **Step 1: Replace `TestCmdQuiet_TogglesPerSession` with thread-scope variant**

Find `TestCmdQuiet_TogglesPerSession` at `core/engine_test.go:10769-10800` and replace the entire function with this version (uses explicit `off`-from-default so it fails against old code that ignores args and toggles to `true`):

```go
func TestCmdQuiet_ThreadScopeTogglesSession(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)
	e.display.ThinkingMessages = true
	e.display.ToolMessages = true

	msg := &Message{
		SessionKey: "feishu:oc_test:root:om_001",
		ChatID:     "oc_test",
		IsThread:   true,
		ReplyCtx:   "ctx",
	}

	// /quiet off from default (not-quiet) — must store explicit false, not toggle to true
	e.cmdQuiet(p, msg, []string{"off"})
	if v := e.chatSettings.Get("", msg.SessionKey, SettingQuiet); v != false {
		t.Fatalf("expected session-layer quiet=false after /quiet off, got %v", v)
	}
	if v := e.chatSettings.Get("oc_test", "", SettingQuiet); v != nil {
		t.Fatalf("expected NO chat-layer override for thread-scoped /quiet, got %v", v)
	}

	// /quiet on
	e.cmdQuiet(p, msg, []string{"on"})
	if v := e.chatSettings.Get("", msg.SessionKey, SettingQuiet); v != true {
		t.Fatalf("expected session-layer quiet=true after /quiet on, got %v", v)
	}

	// Verify global display is NOT mutated
	if !e.display.ThinkingMessages || !e.display.ToolMessages {
		t.Fatal("cmdQuiet should not mutate e.display")
	}
}
```

- [ ] **Step 2: Run test to verify it FAILS**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run TestCmdQuiet_ThreadScopeTogglesSession -count=1 -v 2>&1 | tail -15
```

Expected: FAIL at `expected session-layer quiet=false after /quiet off, got true` — old `cmdQuiet` ignores args and toggles `nil → true` instead of honoring explicit `off`.

---

## Task 2: Add failing tests for top-level chat-scope write

**Files:**
- Modify: `core/engine_test.go`

- [ ] **Step 1: Replace `TestCmdQuiet_SessionIsolation`** at `core/engine_test.go:10802-10821`

Replace the old function (which assumed session-only scope) with:

```go
func TestCmdQuiet_TopLevelScopeWritesChat(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)
	e.display.ThinkingMessages = true
	e.display.ToolMessages = true

	// First top-level /quiet in group "chat1", its own ephemeral sessionKey
	msg1 := &Message{
		SessionKey: "feishu:chat1:root:om_first",
		ChatID:     "chat1",
		IsThread:   false,
		ReplyCtx:   "ctx",
	}
	e.cmdQuiet(p, msg1, []string{"on"})

	// Expect chat-layer write, no session-layer write.
	if v := e.chatSettings.Get("chat1", "", SettingQuiet); v != true {
		t.Fatalf("expected chat-layer quiet=true for top-level /quiet, got %v", v)
	}
	if v := e.chatSettings.Get("", "feishu:chat1:root:om_first", SettingQuiet); v != nil {
		t.Fatalf("expected NO session-layer override for top-level /quiet, got %v", v)
	}

	// A SECOND top-level message in the same chat with a different sessionKey
	// must see the /quiet effect — this is the bug we are fixing.
	msg2 := &Message{
		SessionKey: "feishu:chat1:root:om_second",
		ChatID:     "chat1",
		IsThread:   false,
	}
	if !e.isQuiet(msg2.ChatID, msg2.SessionKey) {
		t.Fatal("second top-level msg in same chat should inherit chat-layer quiet=true")
	}
}
```

- [ ] **Step 2: Run to verify it FAILS**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run TestCmdQuiet_TopLevelScopeWritesChat -count=1 -v 2>&1 | tail -15
```

Expected: FAIL at `expected chat-layer quiet=true for top-level /quiet, got <nil>` because current code always writes to session.

---

## Task 3: Add failing tests for `/quiet reset`

**Files:**
- Modify: `core/engine_test.go` (append at end of file)

- [ ] **Step 1: Add new test**

Append to `core/engine_test.go`:

```go
func TestCmdQuiet_Reset_ThreadScope(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)

	msg := &Message{
		SessionKey: "feishu:chat1:root:om_t",
		ChatID:     "chat1",
		IsThread:   true,
		ReplyCtx:   "ctx",
	}

	e.cmdQuiet(p, msg, []string{"on"})
	if v := e.chatSettings.Get("", msg.SessionKey, SettingQuiet); v != true {
		t.Fatalf("precondition: session-layer quiet=true, got %v", v)
	}

	e.cmdQuiet(p, msg, []string{"reset"})
	if v := e.chatSettings.Get("", msg.SessionKey, SettingQuiet); v != nil {
		t.Fatalf("expected session-layer cleared after /quiet reset, got %v", v)
	}
}

func TestCmdQuiet_Reset_ChatScope(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)

	msg := &Message{
		SessionKey: "feishu:chat1:root:om_top",
		ChatID:     "chat1",
		IsThread:   false,
		ReplyCtx:   "ctx",
	}

	e.cmdQuiet(p, msg, []string{"on"})
	if v := e.chatSettings.Get("chat1", "", SettingQuiet); v != true {
		t.Fatalf("precondition: chat-layer quiet=true, got %v", v)
	}

	e.cmdQuiet(p, msg, []string{"reset"})
	if v := e.chatSettings.Get("chat1", "", SettingQuiet); v != nil {
		t.Fatalf("expected chat-layer cleared after /quiet reset, got %v", v)
	}
}
```

- [ ] **Step 2: Run to verify both FAIL**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run 'TestCmdQuiet_Reset_' -count=1 -v 2>&1 | tail -20
```

Expected: both FAIL — old `cmdQuiet` has no `reset` branch; it just toggles to the opposite value on any call.

---

## Task 4: Add failing test for no-arg status (mirror /atme)

**Files:**
- Modify: `core/engine_test.go` (append at end of file)

- [ ] **Step 1: Add new test**

```go
func TestCmdQuiet_NoArgShowsStatus(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)

	msg := &Message{
		SessionKey: "feishu:chat1:root:om_x",
		ChatID:     "chat1",
		IsThread:   true,
		ReplyCtx:   "ctx",
	}

	// Default: no override anywhere. No-arg /quiet should NOT mutate state.
	e.cmdQuiet(p, msg, nil)
	if v := e.chatSettings.Get("", msg.SessionKey, SettingQuiet); v != nil {
		t.Fatalf("/quiet with no args must not mutate session layer, got %v", v)
	}
	if v := e.chatSettings.Get("chat1", "", SettingQuiet); v != nil {
		t.Fatalf("/quiet with no args must not mutate chat layer, got %v", v)
	}
	if len(p.sent) != 1 {
		t.Fatalf("expected one status reply, got %d", len(p.sent))
	}

	// With session override set, /quiet should report it.
	e.chatSettings.SetSession(msg.SessionKey, SettingQuiet, true)
	p.sent = nil
	e.cmdQuiet(p, msg, nil)
	if len(p.sent) != 1 {
		t.Fatalf("expected one status reply, got %d", len(p.sent))
	}
	if !strings.Contains(p.sent[0], "ON") {
		t.Fatalf("expected status reply to contain ON, got %q", p.sent[0])
	}
}
```

- [ ] **Step 2: Run to verify it FAILS**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run TestCmdQuiet_NoArgShowsStatus -count=1 -v 2>&1 | tail -15
```

Expected: FAIL — old `cmdQuiet` always toggles on no-arg, so `v != nil` trips.

---

## Task 5: Add failing test for DM fallback (empty ChatID)

**Files:**
- Modify: `core/engine_test.go` (append at end of file)

- [ ] **Step 1: Add new test**

Append to `core/engine_test.go`:

```go
func TestCmdQuiet_DMFallbackToSession(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)

	// DM: ChatID empty; scope must fall back to session.
	msg := &Message{
		SessionKey: "weixin:dm:u123",
		ChatID:     "",
		IsThread:   false,
		ReplyCtx:   "ctx",
	}

	// /quiet off from default must explicitly store false, not toggle to true.
	e.cmdQuiet(p, msg, []string{"off"})
	if v := e.chatSettings.Get("", msg.SessionKey, SettingQuiet); v != false {
		t.Fatalf("expected DM /quiet off to write session-layer quiet=false, got %v", v)
	}
}
```

- [ ] **Step 2: Run to verify it FAILS**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run TestCmdQuiet_DMFallbackToSession -count=1 -v 2>&1 | tail -15
```

Expected: FAIL — old code ignores args, toggles `nil → true` instead of honoring `off`.

---

## Task 6: Add i18n keys for /quiet status and reset

**Files:**
- Modify: `core/i18n.go`

- [ ] **Step 1: Add two new MsgKey constants**

Find the `MsgQuietGlobalOff` line around `core/i18n.go:153` and insert two new lines immediately after it:

```go
	MsgQuietGlobalOff            MsgKey = "quiet_global_off"
	MsgQuietReset                MsgKey = "quiet_reset"
	MsgQuietStatus               MsgKey = "quiet_status"
```

- [ ] **Step 2: Add the template entries**

Find the `MsgQuietGlobalOff: { ... },` block around `core/i18n.go:792-798` and insert these two blocks immediately after its closing `},`:

```go
	MsgQuietReset: {
		LangEnglish:            "↩️ Quiet setting reset — this %s now follows the config default.",
		LangChinese:            "↩️ 安静模式已重置 — 此%s现在使用配置默认值。",
		LangTraditionalChinese: "↩️ 安靜模式已重設 — 此%s現在使用配置預設值。",
		LangJapanese:           "↩️ 静音設定をリセット — この%sは設定のデフォルトに従います。",
		LangSpanish:            "↩️ Configuración de modo silencioso restablecida — este %s ahora sigue el valor predeterminado.",
	},
	MsgQuietStatus: {
		LangEnglish:            "Quiet mode for this %s: **%s** (source: %s)",
		LangChinese:            "此%s的安静模式状态: **%s**(来源: %s)",
		LangTraditionalChinese: "此%s的安靜模式狀態: **%s**(來源: %s)",
		LangJapanese:           "この%sの静音モード: **%s**(ソース: %s)",
		LangSpanish:            "Modo silencioso en este %s: **%s** (fuente: %s)",
	},
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
cd ~/projects/cc-connect && go build ./core/ 2>&1 | tail -5
```

Expected: no output.

---

## Task 7: Implement cmdQuiet with scope auto-select + on/off/reset

**Files:**
- Modify: `core/engine.go:6328-6338` (the full `cmdQuiet` function)

- [ ] **Step 1: Replace cmdQuiet**

Open `core/engine.go` and find `func (e *Engine) cmdQuiet` at line 6328. Replace the entire function (lines 6328-6338) with:

```go
// cmdQuiet toggles quiet mode for the current chat or session.
// Scope is auto-selected: threaded messages write to session-level, others
// write to chat-level. DMs (no ChatID) fall back to session-level.
// /quiet            — show current status
// /quiet on         — enable quiet in this scope
// /quiet off        — disable quiet in this scope
// /quiet reset      — clear override, fall back to next layer
func (e *Engine) cmdQuiet(p Platform, msg *Message, args []string) {
	scopeIsSession := (msg.IsThread && msg.SessionKey != "") || msg.ChatID == ""
	scopeLabel := e.i18n.T(MsgScopeChat)
	if scopeIsSession {
		scopeLabel = e.i18n.T(MsgScopeThread)
	}

	if len(args) == 0 {
		e.quietShowStatus(p, msg, scopeLabel)
		return
	}

	switch strings.ToLower(args[0]) {
	case "on":
		if scopeIsSession {
			e.chatSettings.SetSession(msg.SessionKey, SettingQuiet, true)
		} else {
			e.chatSettings.SetChat(msg.ChatID, SettingQuiet, true)
		}
		e.sessions.Save()
		e.reply(p, msg.ReplyCtx, e.i18n.T(MsgQuietOn))
	case "off":
		if scopeIsSession {
			e.chatSettings.SetSession(msg.SessionKey, SettingQuiet, false)
		} else {
			e.chatSettings.SetChat(msg.ChatID, SettingQuiet, false)
		}
		e.sessions.Save()
		e.reply(p, msg.ReplyCtx, e.i18n.T(MsgQuietOff))
	case "reset":
		if scopeIsSession {
			e.chatSettings.DeleteSession(msg.SessionKey, SettingQuiet)
		} else {
			e.chatSettings.DeleteChat(msg.ChatID, SettingQuiet)
		}
		e.sessions.Save()
		e.reply(p, msg.ReplyCtx, fmt.Sprintf(e.i18n.T(MsgQuietReset), scopeLabel))
	default:
		e.reply(p, msg.ReplyCtx, "Usage: /quiet [on|off|reset]")
	}
}

// quietShowStatus reports the effective quiet value and its source layer,
// mirroring atmeShowStatus.
func (e *Engine) quietShowStatus(p Platform, msg *Message, scopeLabel string) {
	if v := e.chatSettings.Get("", msg.SessionKey, SettingQuiet); v != nil {
		if b, ok := v.(bool); ok {
			state := "OFF"
			if b {
				state = "ON"
			}
			e.reply(p, msg.ReplyCtx, fmt.Sprintf(e.i18n.T(MsgQuietStatus), scopeLabel, state, "session"))
			return
		}
	}
	if v := e.chatSettings.Get(msg.ChatID, "", SettingQuiet); v != nil {
		if b, ok := v.(bool); ok {
			state := "OFF"
			if b {
				state = "ON"
			}
			e.reply(p, msg.ReplyCtx, fmt.Sprintf(e.i18n.T(MsgQuietStatus), scopeLabel, state, "chat"))
			return
		}
	}
	defaultState := "OFF"
	if !e.display.ThinkingMessages && !e.display.ToolMessages {
		defaultState = "ON"
	}
	e.reply(p, msg.ReplyCtx, fmt.Sprintf(e.i18n.T(MsgQuietStatus), scopeLabel, defaultState, "config"))
}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
cd ~/projects/cc-connect && go build ./core/ 2>&1 | tail -5
```

Expected: no output (clean build). `fmt` and `strings` are already imported in `engine.go`.

- [ ] **Step 3: Run the failing tests from Tasks 1-5 — all should now PASS**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run 'TestCmdQuiet_ThreadScopeTogglesSession|TestCmdQuiet_TopLevelScopeWritesChat|TestCmdQuiet_Reset_|TestCmdQuiet_NoArgShowsStatus|TestCmdQuiet_DMFallbackToSession' -count=1 -v 2>&1 | tail -25
```

Expected: all PASS.

---

## Task 8: Update the tests that assumed old toggle-on-no-arg behavior

**Files:**
- Modify: `core/engine_test.go`

- [ ] **Step 1: Replace `TestCmdQuiet_TogglesDisplay`** at `core/engine_test.go:4189-4212`

Old test relies on no-arg toggling. Rewrite as:

```go
func TestCmdQuiet_TogglesDisplay(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)
	e.display.ThinkingMessages = true
	e.display.ToolMessages = true

	msg := &Message{
		SessionKey: "feishu:chat1:root:om_toggle",
		ChatID:     "chat1",
		IsThread:   true,
		ReplyCtx:   "ctx",
	}

	// /quiet on
	e.cmdQuiet(p, msg, []string{"on"})
	if !e.isQuiet("chat1", "feishu:chat1:root:om_toggle") {
		t.Fatal("after /quiet on: expected isQuiet=true")
	}
	if len(p.sent) != 1 || !strings.Contains(p.sent[0], "Quiet mode ON") {
		t.Fatalf("expected 'Quiet mode ON' reply, got: %v", p.sent)
	}

	// /quiet off
	p.sent = nil
	e.cmdQuiet(p, msg, []string{"off"})
	if e.isQuiet("chat1", "feishu:chat1:root:om_toggle") {
		t.Fatal("after /quiet off: expected isQuiet=false")
	}
	if len(p.sent) != 1 || !strings.Contains(p.sent[0], "Quiet mode OFF") {
		t.Fatalf("expected 'Quiet mode OFF' reply, got: %v", p.sent)
	}
}
```

- [ ] **Step 2: Replace `TestCmdQuiet_DoesNotWriteConfigToml`** at `core/engine_test.go:10823-10841`

Old test used no-arg toggle. New version uses explicit `on`:

```go
func TestCmdQuiet_DoesNotWriteConfigToml(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)
	e.display.ThinkingMessages = true
	e.display.ToolMessages = true

	called := false
	e.displaySaveFunc = func(tm *bool, tmax *int, toolmax *int, tool *bool) error {
		called = true
		return nil
	}

	msg := &Message{
		SessionKey: "feishu:chat1:root:om_cfg",
		ChatID:     "chat1",
		IsThread:   true,
		ReplyCtx:   "ctx",
	}
	e.cmdQuiet(p, msg, []string{"on"})

	if called {
		t.Fatal("cmdQuiet should not call displaySaveFunc")
	}
}
```

- [ ] **Step 3: Replace `TestCmdQuiet_IgnoresArgs`** at `core/engine_test.go:10843-10859`

Old test asserted unknown args still toggle. New semantics: unknown args print usage without mutating state. Rename and rewrite:

```go
func TestCmdQuiet_UnknownArgShowsUsage(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)
	e.display.ThinkingMessages = true
	e.display.ToolMessages = true

	msg := &Message{
		SessionKey: "feishu:chat1:root:om_bad",
		ChatID:     "chat1",
		IsThread:   true,
		ReplyCtx:   "ctx",
	}
	e.cmdQuiet(p, msg, []string{"blah"})

	// State must NOT be mutated
	if v := e.chatSettings.Get("", msg.SessionKey, SettingQuiet); v != nil {
		t.Fatalf("unknown arg must not mutate session-layer, got %v", v)
	}
	if v := e.chatSettings.Get("chat1", "", SettingQuiet); v != nil {
		t.Fatalf("unknown arg must not mutate chat-layer, got %v", v)
	}
	if len(p.sent) != 1 {
		t.Fatalf("expected exactly 1 usage reply, got %d", len(p.sent))
	}
	if !strings.Contains(p.sent[0], "Usage:") {
		t.Fatalf("expected Usage: reply, got: %q", p.sent[0])
	}
}
```

- [ ] **Step 4: Replace `TestCmdQuiet_RapidToggle`** at `core/engine_test.go:10861-10876`

Old test chained 3 no-arg toggles. New version uses explicit on/off/on:

```go
func TestCmdQuiet_OnOffOnDeterministic(t *testing.T) {
	p := &stubPlatformEngine{n: "test"}
	e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)
	e.display.ThinkingMessages = true
	e.display.ToolMessages = true

	msg := &Message{
		SessionKey: "feishu:chat1:root:om_rapid",
		ChatID:     "chat1",
		IsThread:   true,
		ReplyCtx:   "ctx",
	}

	e.cmdQuiet(p, msg, []string{"on"})
	e.cmdQuiet(p, msg, []string{"off"})
	e.cmdQuiet(p, msg, []string{"on"})

	if !e.isQuiet("chat1", msg.SessionKey) {
		t.Fatal("expected quiet after on/off/on")
	}
}
```

- [ ] **Step 5: Run the full /quiet test set**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run TestCmdQuiet -count=1 -v 2>&1 | tail -40
```

Expected: all `TestCmdQuiet_*` PASS, no FAIL.

---

## Task 9: Regression sweep — isQuiet / isThinkingHidden / display pipeline

**Files:**
- No code changes; just verification runs.

- [ ] **Step 1: Run the isQuiet / display integration tests**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run 'TestIsQuiet_|TestIsThinkingHidden|TestProcessInteractiveEvents_Quiet' -count=1 -v 2>&1 | tail -30
```

Expected: all PASS. These exercise the lookup side (`isQuiet`, `isThinkingHidden`, `isToolHidden`) which still uses `chatSettings.Get(chatID, sessionKey, ...)` and traverses session→chat→config the same way.

- [ ] **Step 2: Run the /atme and /thread tests to confirm no collateral damage**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -run 'TestCmdAtme|TestCmdThread' -count=1 -v 2>&1 | tail -30
```

Expected: all PASS.

---

## Task 10: Full test suite + build + commit

- [ ] **Step 1: Full core package test**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./core/ -count=1 2>&1 | tail -10
```

Expected: `ok  github.com/chenhg5/cc-connect/core`.

- [ ] **Step 2: Full repo test (catch cross-package regressions)**

```bash
cd ~/projects/cc-connect && go test -tags no_web ./... -count=1 2>&1 | tail -30
```

Expected: every `ok`, no `FAIL`.

- [ ] **Step 3: Vet + build**

```bash
cd ~/projects/cc-connect && go vet ./... 2>&1 | head -10 && go build -o /tmp/cc-connect-quiet-fix ./cmd/cc-connect 2>&1 | head -10
```

Expected: no output from either.

- [ ] **Step 4: Commit**

```bash
cd ~/projects/cc-connect && git add core/engine.go core/engine_test.go core/i18n.go docs/superpowers/plans/2026-04-22-quiet-scope-fix.md && git commit -s -m "fix(core): /quiet auto-selects scope; add on/off/reset subcommands

/quiet unconditionally wrote session-level overrides, which broke it in
thread-isolation groups: each top-level message derives its sessionKey
from its own MessageId, so \`/quiet\` saved to a one-shot key that no
subsequent message ever read. Users typed /quiet repeatedly with no
visible effect and a pile of dead \`session_settings.*.quiet\` entries
accumulated in sessions.json.

Mirror /atme's scope auto-selection:
- Threaded msg (IsThread && SessionKey != \"\") -> session layer
- DM (ChatID == \"\")                          -> session layer
- Top-level group msg                          -> chat layer

Also add on/off/reset subcommands for deterministic state, and make
no-arg /quiet print current status instead of toggling (parity with
/atme). Added MsgQuietStatus/MsgQuietReset i18n templates.

Existing session-layer overrides in sessions.json continue to win via
the layered Get() lookup, so any per-thread tuning the user explicitly
intended is preserved. Dead one-shot entries are harmless (just noise)
and will naturally age out as those threads close; no migration needed.
"
```

---

## Task 11: Deploy + manual smoke test

- [ ] **Step 1: Rebuild the daemon binary**

```bash
cd ~/projects/cc-connect && go build -o ~/bin/cc-connect ./cmd/cc-connect 2>&1 | head
```

Expected: no output.

- [ ] **Step 2: Ask user to restart the daemon and test**

Message to user (车间主任): "已完成。请重启 cc-connect daemon (`kill $(pgrep -xf '/home/neo/bin/cc-connect') && cd ~/projects/cc-connect && nohup ~/bin/cc-connect >> ~/.cc-connect.log 2>&1 &`),然后在日常工作助手群里测试：
1. `/quiet` — 应显示当前状态(不修改)
2. `/quiet on` — 群级开启
3. 新开一条顶层消息问点简单问题 — 应看不到 thinking/tool 消息
4. `/quiet reset` — 回到默认"

- [ ] **Step 3: If user reports thinking/tool still visible in new top-level**

Debug path:
```bash
tail -f ~/.cc-connect.log | grep -iE 'quiet|chat_setting'
```
Verify chat-layer write shows up and `isQuiet` returns true for the second top-level's sessionKey.

---

## Task 12: Push to fork

- [ ] **Step 1: Fast-forward push**

```bash
cd ~/projects/cc-connect && git push fork main 2>&1
```

Expected: `main -> main` (non-forced, because we're only adding commits on top of the already-pushed tip).

No force-push needed — this task only appends new commits after the last rebase-then-push sync.

---

## Review Checklist (run before declaring done)

- [ ] `cmdQuiet` scope logic: `(IsThread && SessionKey != "") || ChatID == ""` → session; else chat
- [ ] All four subcommands implemented: on/off/reset/no-arg-status
- [ ] No-arg no longer mutates state (breaking change, intentional, documented in commit message)
- [ ] `sessions.Save()` called on every state-changing branch (on/off/reset)
- [ ] `isQuiet`/`isThinkingHidden`/`isToolHidden` unchanged — they already layer-traverse correctly via `chatSettings.Get`
- [ ] Two new i18n keys added (`MsgQuietStatus`, `MsgQuietReset`) with 5 languages each
- [ ] Existing dead `session_settings.*.quiet` entries are harmless and not cleaned up (no migration)
- [ ] `go test -tags no_web ./... -count=1` clean
- [ ] `go vet ./...` clean
- [ ] Commit message explains the bug, the fix, and the backwards-compat story
