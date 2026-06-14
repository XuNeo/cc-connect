# TRAE CLI Agent with Model Variant Support — Design

Date: 2026-06-14
Status: Approved (pending spec review)
Branch / worktree: `feat-traecli-agent` at `/tmp/cc-traecli`

## Problem

cc-connect currently drives ByteDance's internal `traecli` (a fork of OpenAI Codex)
through the existing `type = "codex"` agent plus a `cli_path = "traecli"` override
("Plan A", already shipped and e2e-tested). Message send/receive works, but the
`/model` command is broken for traecli users:

1. **Wrong model list.** The codex driver's `readCodexCachedModels` (agent/codex/codex.go:271)
   reads the model cache only from the `CODEX_HOME` env var or `~/.codex`, ignoring the
   configured `codex_home`. It therefore never finds traecli's cache at
   `~/.trae/cli/models_cache.json` and falls back to 6 hardcoded OpenAI placeholder names.
2. **No variant switching.** The codex driver has no concept of `model_backend_variant`
   and never passes it; traecli's standard/max variant cannot be selected.
3. **`__max` suffix does not switch variant.** Verified empirically: `-m gpt-5.5__max`
   and `-m gpt-5.5` land identically; only `-c model_backend_variant="..."` actually
   changes the variant, and it takes precedence over any `-m` suffix.
4. **Codex has no variant concept.** `model_backend_variant` is traecli-specific and
   must not pollute the shared codex driver path.

## Verified facts (constrain the design)

- traecli's model list is **dynamic**, not static. Evidence: `models_cache.json` carries
  a `fetched_at` timestamp, `provider_mode = replaces_bundled_catalog`, and the
  app-server exposes a `model/list` JSON-RPC method that returns the live catalog.
  → The model list must never be hardcoded.
- Current snapshot: 23 models. Only 6 have a max variant
  (openrouter-3o, openrouter-2o, Doubao-Seed-2.0-Code, Doubao_1_6, gpt-5.5, gpt-5.4).
  openrouter/gpt max = 1M context; doubao max = 200K. The other 17 have no max.
- Two data sources use different slug casing: `models_cache.json` has `GPT-5.5`;
  app-server `model/list` returns `gpt-5.5`. traecli's `-m` normalizes either, both
  land as the same model.
- app-server `model/list` returns the catalog under `result.data[]` (not `result.models`).
  Each entry's variant info is at `businessMetadata.variants.max_key` (non-empty only
  for models that have a max variant).
- Forcing `variant=max` on a model without a max variant does not error, but the model
  effectively runs as standard ("fake max"). So variant must only be offered for models
  whose `max_key` is non-empty.
- Engine routing: `modelSwitchNeedsLookup` (core/engine.go:7711) treats any input
  containing `/` as a full identifier and passes it straight to `SetModel` without
  list lookup. So a model name like `gpt-5.5/max` reaches the agent's `SetModel`
  intact and is NOT split into provider/model.

## Requirements

Must:
- `/model` lists traecli's real, dynamic models (not stale, not hardcoded).
- Selecting a model actually switches it.
- Do not change existing Codex behavior at all (byte-for-byte identical command construction).

Should:
- Select standard/max variant, but only for models that actually have a max variant.

## Decisions

| Dimension | Decision |
|---|---|
| Route | Dedicated `type = "traecli"` agent, implemented via a `profile` inside the codex package |
| Registration | Same package registers both: `RegisterAgent("codex", New)` (unchanged) and `RegisterAgent("traecli", NewTraeCLI)` |
| Model list source | app-server `model/list` first, fall back to `~/.trae/cli/models_cache.json` |
| Variant display/return value | `gpt-5.5/max` (slash form); manual input also accepts `__max`, ` (max)`, `/max` |
| Variant scope | Only models with a non-empty `max_key` produce a `/max` list entry |
| Variant argument | Always `-c model_backend_variant="max"`; never appended to `-m` |
| Range | traecli only; codex behavior untouched; codex's `codex_home` cache bug NOT fixed here |

## Architecture: profile inside the codex package

Introduce a `profile` value that captures all identity/behavior differences:

```go
type profile struct {
    name            string // "codex" | "traecli"
    defaultBin      string // "codex" | "traecli"
    displayName     string // "Codex" | "TRAE CLI"
    homeEnv         string // "CODEX_HOME" | "TRAE_HOME" (informational)
    defaultHome     string // "~/.codex" | "~/.trae/cli"
    supportsVariant bool   // false for codex, true for traecli
    modelListViaAppServer bool // false for codex, true for traecli
}
```

- `New(opts)` → `newWithProfile(opts, codexProfile)` — existing behavior, byte-for-byte.
- `NewTraeCLI(opts)` → `newWithProfile(opts, traecliProfile)`.
- `init()` adds `core.RegisterAgent("traecli", NewTraeCLI)`; the codex registration is unchanged.
- `Name()`, `CLIBinaryName()`, `CLIDisplayName()` read from `profile`.
- The `Agent` struct gains `prof profile` and a `variant string` field.

The agent reuses the entire existing event-protocol parsing (session.go, list.go,
context_usage.go) unchanged, since traecli and codex speak the identical
`exec --json` protocol.

## Model list (traecli profile only)

`AvailableModels(ctx)`:
1. If `profile.modelListViaAppServer`: spawn `traecli app-server`, run the JSON-RPC
   handshake (reuse the pattern in `loadCodexRuntimeConfig`, session.go:601), call
   `model/list`, parse `result.data[]`.
2. On any failure (no login, timeout, spawn error): fall back to reading
   `<codexHome>/models_cache.json` where `codexHome` is the configured `codex_home`
   (default `~/.trae/cli`).
3. For each model, emit one `ModelOption{Name: slug}`. If the model's
   `variants.max_key` is non-empty, ALSO emit `ModelOption{Name: slug + "/max"}`.

Notes:
- Use the slug as returned by the chosen source. De-duplicate case-insensitively to
  avoid showing the same model twice if both casings appear.
- The JSON-RPC `result.data[]` → `[]ModelOption` mapping (including `/max` expansion)
  is extracted into a pure function `parseModelListResult(raw []byte) []ModelOption`
  so it is unit-testable without spawning a subprocess. The cache path reuses the same
  expansion helper on the cache's `models[]` array.
- A test seam controls the app-server attempt: when app-server is unavailable
  (no login / spawn fails) or disabled in tests, `AvailableModels` deterministically
  uses the cache path. Tests exercise the cache path offline and the parser directly.
- The codex profile keeps its original `AvailableModels` path (reads `~/.codex` cache,
  OpenAI fallback). It is unaffected because `modelListViaAppServer` is false and the
  variant expansion is gated on `profile.supportsVariant`.

## Variant parse and argument construction (traecli profile only)

`SetModel(input)`:
- Lenient parser strips a trailing variant marker in any of these forms:
  `"<slug>/max"`, `"<slug>__max"`, `"<slug> (max)"` → sets `model = <slug>`, `variant = "max"`.
- No marker → `model = input`, `variant = "standard"` (or empty; traecli defaults to
  standard unless the global toml overrides it).

`GetModel()`:
- traecli profile: returns the round-tripped display form, i.e. `model + "/max"` when
  `variant == "max"`, otherwise just `model`. This keeps the `/model` card's
  "current selection" indicator consistent with the list entries.
- codex profile: unchanged (returns the model via the existing provider-aware path).

`buildExecArgs`:
- `if cs.profile.supportsVariant && cs.variant == "max" { args = append(args, "-c", `model_backend_variant="max"`) }`
- The codex profile never sets `variant`, so this branch never fires for codex.

## /model command flow

No core changes. The variant rides entirely on the existing `ModelOption.Name` +
`SetModel` channel:
- The list renderer shows each `Name` (e.g. `gpt-5.5`, `gpt-5.5/max`).
- Selecting an item re-issues `/model switch <name>`; because `gpt-5.5/max` contains
  `/`, the engine passes it straight to `SetModel`, which parses out the variant.
- Manual `/model gpt-5.5/max` (or `__max` / ` (max)`) works identically.

## Wiring

- New file `cmd/cc-connect/plugin_agent_traecli.go` with `//go:build !no_traecli`,
  blank-importing the codex package.
- `Makefile`: add `traecli` to `ALL_AGENTS`.
- `config.example.toml`: change the traecli example from `type="codex" + cli_path`
  to `type="traecli"`.
- User's `~/.cc-connect/config.toml`: switch `type` from `codex` to `traecli`,
  drop the `cli_path` hack (separate follow-up, not part of repo changes).

## Testing

All tests live in the existing `package codex` (so they can call unexported
constructors/helpers like `newCodexSession`, `newWithProfile`, `buildExecArgs`,
and reuse the existing `containsSequence` helper from `session_test.go`). New
file: `agent/codex/traecli_test.go`.

### Test commands

```bash
# from the worktree root /tmp/cc-traecli
go test ./agent/codex/ -run 'TestTraeCLI|TestCodexProfile|TestBuildExecArgs' -v   # focused
go test ./agent/codex/                                                            # whole package, catches regressions
go test ./...                                                                     # full suite gate before commit
CC_TRAECLI_E2E=1 go test ./agent/codex/ -run TestIntegration_TraeCLI -v          # gated live e2e (needs traecli login)
```
(Build cache writes require running outside the filesystem sandbox.)

### 1. Identity (table-driven)

`TestTraeCLIProfile_Identity`: construct via `NewTraeCLI(map[string]any{})` and assert
`Name()=="traecli"`, `CLIBinaryName()=="traecli"`, `CLIDisplayName()=="TRAE CLI"`.
`TestCodexProfile_Identity`: construct via `New(...)` and assert the values stay
`codex` / `codex` / `Codex`. This pins both profiles in one place.

### 2. Codex regression (locks "no break") — the critical guard

These assert the codex path is byte-for-byte unchanged:
- `TestCodexProfile_BuildExecArgsNoVariant`: build a codex session with any model and
  assert the args contain **no** element matching `model_backend_variant`. Use the
  existing exact-sequence style of `TestBuildExecArgs_IncludesReasoningEffort`
  (full `want` slice equality) so an accidental extra `-c` is caught.
- `TestCodexProfile_AvailableModelsIgnoresTraeHome`: point the codex agent's
  `codex_home` at a temp dir seeded with a `models_cache.json`, and a separate
  fake `~/.trae` fixture; assert the codex profile does **not** surface traecli
  models and never produces a `/max` entry (variant expansion gated off).

### 3. Variant parsing (table-driven, the core new logic)

`TestTraeCLIProfile_SetModelVariantParse` with a case table:

| input | want model | want variant |
|---|---|---|
| `gpt-5.5/max` | `gpt-5.5` | `max` |
| `gpt-5.5__max` | `gpt-5.5` | `max` |
| `gpt-5.5 (max)` | `gpt-5.5` | `max` |
| `gpt-5.5` | `gpt-5.5` | `standard` (or empty) |
| `openrouter-3o/max` | `openrouter-3o` | `max` |

For each row: `SetModel(input)`, then assert `GetModel()` round-trips to the
display form (`<model>/max` when max, else `<model>`), and that a freshly built
session's `buildExecArgs` contains `-c model_backend_variant="max"` exactly when
variant is max (via `containsSequence`), and is absent otherwise.

### 4. Model list expansion (cache-fed fixture, deterministic)

`TestTraeCLIProfile_AvailableModelsFromCache`: write a temp `models_cache.json`
fixture containing a representative subset — 2 models with non-empty
`variants.max_key` (e.g. `gpt-5.5`, `openrouter-3o`) and 2 without (e.g. `glm-5`,
`gpt-5.2`) — set the agent's `codex_home` to that temp dir, force the cache path
(app-server disabled in the test by leaving login unavailable / a test seam that
skips app-server), and assert:
- every model appears once as a bare entry,
- exactly the 2 max-capable models also appear as `<slug>/max`,
- the 2 without `max_key` produce **no** `/max` entry,
- no duplicate entries when the fixture has mixed-case slugs (case-insensitive dedup).

This test is fully offline and deterministic (no network, no traecli binary).

### 5. App-server source (best-effort, gated)

The app-server `model/list` path depends on a logged-in traecli and a spawned
subprocess, so it is **not** unit-tested offline. It is covered by:
- `TestTraeCLIProfile_AvailableModelsFromCache` proving the fallback path, and
- the gated e2e below proving the live path end to end.
A thin parser function (`parseModelListResult([]byte) []ModelOption`) is extracted
so the JSON-RPC `result.data[]` → `ModelOption` mapping (including `/max` expansion)
is unit-tested directly with a captured real `model/list` response fixture, without
spawning anything: `TestParseModelListResult`.

### 6. Gated live e2e

Reuse the existing `agent/codex/traecli_test.go` e2e pattern, switched to
`NewTraeCLI`, behind `CC_TRAECLI_E2E=1`. It sends a prompt and asserts a non-empty
response and session ID. Add one extra step: call `AvailableModels(ctx)` and assert
it returns more than the 6 OpenAI placeholder names and contains at least one `/max`
entry — proving the app-server live path works against the real binary.

### Fixtures

- `agent/codex/testdata/traecli_models_cache.json` — trimmed real cache (mixed-case
  slugs, mix of max/non-max) for test 4.
- `agent/codex/testdata/traecli_model_list_response.json` — a captured real
  `model/list` JSON-RPC `result` payload for test 5's `TestParseModelListResult`.

## Known trade-offs

- Variant is only meaningful for the 6 models the traecli backend currently exposes;
  this set is server-driven and may change. We never hardcode it — `/max` entries are
  derived from live `max_key` data.
- app-server `model/list` adds a few seconds of latency and depends on login state,
  hence the cache fallback.
- The codex `codex_home` cache bug is intentionally left unfixed (out of scope).

## Out of scope

- Reasoning-effort UX changes for traecli (traecli models report empty
  `supported_reasoning_levels`).
- Fixing codex's `readCodexCachedModels` to honor configured `codex_home`.
- A dedicated `/variant` command or a two-level card picker.
