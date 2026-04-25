# Mid-Stream Message Injection Design

**Status**: Draft
**Date**: 2026-04-25
**Author**: 车间主任 (xuxingliang) with Claude

## Problem

When cc-connect is busy processing a turn for a user, any additional message
from the same user is queued (cap = 5) and fired as a **separate turn** after
the current turn's `EventResult` arrives. This causes two user-visible issues:

1. **Latency**: the second message waits for the first turn to complete, even
   when the agent is mid-tool-loop and could incorporate the new message.
2. **UX divergence from terminal**: in the terminal, Claude Code accepts
   stdin input at any time and folds it into the current turn naturally.

The existing code comment at `core/engine.go:1760-1764` claims:

> "The agent CLI may treat a mid-turn stdin message as part of the current
> turn, causing the event loop to hang waiting for a second EventResult that
> never arrives."

**This claim is wrong.** Experimental verification (2026-04-25) shows the
Claude Code CLI in `--input-format stream-json` mode accepts a second user
message mid-stream, folds it into the current turn at the next message
boundary (typically after a `tool_result`), and emits exactly ONE `result`
event at turn completion — no hang.

The real reason cc-connect's code hangs is that cc-connect's accounting
assumes "one user message = one result event". The CLI actually does
"one turn = one result event", where a turn can contain multiple user
messages.

## Goal

Change cc-connect's message routing so that mid-turn messages are injected
directly to the agent's stdin (via the existing `Send()` path) instead of
queued until turn completion. Behavior matches terminal Claude Code.

## Non-Goals

- Splitting a single turn's assistant output across multiple per-message reply
  contexts. All assistant output of a merged turn goes to the turn's original
  `replyCtx`.
- Adding a config switch. The new behavior is the only behavior.
- Changing persistence schema, CLI flags, or platform protocols.

## Design

### State Model

Three effective states per session, derived from existing fields:

| State | `Session.busy` | `state.agentSession` | Behavior on new message |
|---|---|---|---|
| `idle` | `false` | irrelevant | Caller acquires `TryLock`, becomes turn owner, spawns agent if needed, enters event loop |
| `starting` | `true` | `nil` | Append to `state.pendingMessages`; drained when agent process is live |
| `running` | `true` | non-nil and `Alive()==true` | Call `state.agentSession.Send()` directly (no queue) |

Edge:

| State | `Session.busy` | `state.agentSession` | Behavior |
|---|---|---|---|
| `dying` | `true` | non-nil and `Alive()==false` | Reply `MsgPreviousProcessing`; race window, agent died but event loop hasn't yet emitted its cleanup path |

**`replyCtx`**: established by the idle→starting transition. Mid-stream
messages do NOT overwrite `state.replyCtx`. All assistant output for the turn
flows to the established `replyCtx`.

### handleMessage Routing (engine.go:1636+)

```go
session := sessions.GetOrCreateActive(msg.SessionKey)
sessions.UpdateUserMeta(msg.SessionKey, msg.UserName, msg.ChatName)

if session.TryLock() {
    // idle → become turn owner
    ensureInteractiveStateForQueueing(interactiveKey, p, msg.ReplyCtx)
    go e.processInteractiveMessageWith(...)
    return
}

// busy — classify by agent liveness
e.interactiveMu.Lock()
state, ok := e.interactiveStates[interactiveKey]
e.interactiveMu.Unlock()

if !ok || state == nil || state.agentSession == nil {
    // starting → queue for post-spawn drain
    e.appendPendingMessage(state, p, msg)  // unbounded in practice
    return
}

if state.agentSession.Alive() {
    // running → inject directly
    prompt, imgs, files := buildPrompt(msg)
    if err := state.agentSession.Send(prompt, imgs, files); err != nil {
        e.reply(p, msg.ReplyCtx, e.i18n.T(MsgInjectFailed))
    }
    // No ack reply — the agent's response to this injected message
    // will appear as part of the current turn's output.
    return
}

// dying
e.reply(p, msg.ReplyCtx, e.i18n.T(MsgPreviousProcessing))
```

`/btw` handling is deleted from this router — any message now takes the same
path. `/btw` command parsing is kept as a no-op alias (see "`/btw`
Compatibility" below).

### Post-Spawn Drain (agent/.../session.go callers inside engine.go)

After the agent process is spawned and `state.agentSession` is set to a live
session, but **before** the turn owner's first `Send()`:

```go
// inside processInteractiveMessageWith, after agent Alive()
e.flushStartupQueue(state)
```

`flushStartupQueue` drains all messages from `state.pendingMessages` in order,
calling `state.agentSession.Send()` for each, then appends the turn owner's
original message and `Send()`s that too. All messages go into the same turn.

### Session / Agent Send Concurrency

All agent provider sessions already serialize `Send()` via a per-session
mutex (verified in `agent/claudecode/session.go:699-700` via `stdinMu`; other
providers use equivalent). Mid-stream injection from the handleMessage
goroutine while the event-loop goroutine is reading is therefore safe.

### Error Handling

| Scenario | Handling |
|---|---|
| `state.agentSession.Send()` returns error during injection | Reply `MsgInjectFailed` (new i18n key, add alongside existing `MsgBtwSendFailed`) to the injecting user. Turn continues. |
| Agent dies mid-turn (stdout close) | Existing crash path at `engine.go:3251` handles cleanup. |
| `state.pendingMessages` growth during long startup | Startup is seconds-scale; no practical cap. Add `maxStartupQueue = 100` as defensive bound (unreachable in normal use). |
| Orphan race: message queued during `starting` but the would-be turn-owner goroutine exited before reading the queue | `drainOrphanedQueue` retry path is preserved. Its scope narrows: it now only guards the `idle→starting→running` handoff, never the post-result window. |
| `TryLock` fails, but between that and `interactiveStates[key]` read the state goes nil | Treat as `dying` → reply `MsgPreviousProcessing`. |

### Deletions from engine.go

| Item | Location | Why |
|---|---|---|
| `maxQueuedMessages = 5` | line 28 | No runtime queue cap needed; startup queue has separate defensive `maxStartupQueue` |
| Runtime-busy branch of `queueMessageForBusySession` | lines 1746-1792 | Runtime-busy now goes through direct `Send()`, not queue |
| `MsgMessageQueued` reply invocation | line 1790 | No queueing during running state |
| `drainPendingMessages` post-result loop | lines 3135-3183 | Running state never queues, so nothing to drain post-result |
| `drainPendingMessages` secondary call | line 3314 / call at 2207 | Same reason |
| Erroneous "hang waiting for second EventResult" comment | 1760-1764 | Factually wrong — document the new behavior instead |
| `/btw` routing special-case | 1639-1657 | Command retained but no longer a distinct path |

Estimated diff: **≈ -250 / +80 lines**, net deletion.

### `/btw` Compatibility

- Command detection and `MsgBtwSent` reply are retained as pure
  backward-compatibility for users with existing scripts or muscle memory.
- Implementation path is merged with ordinary messages — both call
  `state.agentSession.Send()`. The only behavioral difference is `/btw`
  emits an explicit "sent" reply, while ordinary messages emit nothing until
  the turn's assistant output arrives.
- Documentation is updated to note `/btw` is now redundant.
- Deprecation and eventual removal are out of scope for this change.

### Turn Accounting

- `result` is still the single turn-completion signal. `busy=false` happens
  there, unchanged.
- Cost/tokens/usage accounting remains one-event-per-turn. Merged turns that
  contain multiple user messages are counted as one turn — consistent with
  what the CLI emits.
- `num_turns` from the `result` event is still not read (no behavior depends
  on it).

## Testing

### Experimental verification (done)

Script at `/tmp/cctest/test_midstream.py` already demonstrates:

- Claude CLI accepts stdin `user` message mid-turn
- Second message lands at next message boundary (after tool_result)
- One `result` event emitted with `num_turns=2`, `subtype=success`
- No hang, no error

Script will be cleaned up and committed as an integration test.

### Integration tests to add

Under `tests/` (or `core/engine_midstream_test.go` if Go unit test is
preferred):

1. Idle → starting → running transitions routed correctly
2. Messages during `starting` accumulate and flush after agent spawn in order
3. Messages during `running` go directly to `Send()`, never to
   `pendingMessages`
4. `/btw` command still emits `MsgBtwSent` reply
5. Agent death while busy=true yields `MsgPreviousProcessing` to injected
   message
6. `Send()` error during injection replies `MsgInjectFailed`
7. End-to-end: multi-message turn produces single `result`; `replyCtx` is
   turn-originator's

### Manual smoke

- Run cc-connect bound to a Feishu/WeChat session
- Kick off a long task (e.g., "list all .go files in this repo and describe
  each")
- While running, send 2-3 follow-up messages
- Verify: all messages incorporated into the same turn's response, single
  reply delivered to the first sender

## Rollout & Rollback

- Single PR, single logical commit (optionally split: "remove queue",
  "inject mid-stream", "update `/btw`").
- No config migration.
- No persistence schema change.
- Rollback: `git revert` the PR; redeploy. Users see old queue behavior
  restored within one release cycle.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Model behavior: mid-stream injection may cause Claude to finish current tool step before switching focus | Medium | Upstream CLI behavior; same as terminal Claude Code. Document as expected. |
| Provider variance: 11 agent providers, not all verified mid-stream-safe | Medium | Providers serialize via their own mutex; failures will surface as `Send()` errors handled by `MsgInjectFailed`. Escalation path: per-provider fallback to queue mode if one regresses. |
| UX loss of `MsgMessageQueued` ack | Low | User still sees typing indicator / assistant output; ack removal matches terminal UX. |
| Startup queue growth if agent hang during spawn | Low | `maxStartupQueue` cap + existing agent spawn timeout. |
| `/btw` users expect its confirmation timing | Low | Ack message retained with same text. |

## Open Questions

None at spec writing time. Implementation plan will surface any per-provider
quirks (e.g., if a specific HTTP-based agent requires turn-boundary
serialization that stdin-based ones do not).

## References

- Experimental verification: `/tmp/cctest/test_midstream.py` (move into
  repo under `tests/midstream/` as part of implementation)
- Existing queue code: `core/engine.go:1636-1671, 1746-1792, 3135-3183`
- Session state: `core/session.go:17-57`
- Claude Code agent Send: `agent/claudecode/session.go:576-644, 699-700`
