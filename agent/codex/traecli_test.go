package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

// installFakeBin puts a no-op executable named bin on PATH so the agent
// constructors (which call exec.LookPath) succeed without the real binary.
func installFakeBin(t *testing.T, bin string) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, bin+".cmd")
		if err := os.WriteFile(path, []byte("@echo off\r\n"), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", bin, err)
		}
	} else {
		path := filepath.Join(dir, bin)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", bin, err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newTestTraeCLI(t *testing.T, opts map[string]any) *Agent {
	t.Helper()
	installFakeBin(t, "traecli")
	a, err := NewTraeCLI(opts)
	if err != nil {
		t.Fatalf("NewTraeCLI: %v", err)
	}
	return a.(*Agent)
}

func newTestCodex(t *testing.T, opts map[string]any) *Agent {
	t.Helper()
	installFakeBin(t, "codex")
	a, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a.(*Agent)
}

func TestTraeCLIProfile_Identity(t *testing.T) {
	a := newTestTraeCLI(t, map[string]any{})
	if got := a.Name(); got != "traecli" {
		t.Errorf("Name() = %q, want traecli", got)
	}
	if got := a.CLIBinaryName(); got != "traecli" {
		t.Errorf("CLIBinaryName() = %q, want traecli", got)
	}
	if got := a.CLIDisplayName(); got != "TRAE CLI" {
		t.Errorf("CLIDisplayName() = %q, want TRAE CLI", got)
	}
}

// TestTraeCLIProfile_VariantFromOpts verifies the constructor honors an explicit
// variant opt (used by WorkspaceAgentOptions recreation, where model is the bare
// slug) over the model suffix.
func TestTraeCLIProfile_VariantFromOpts(t *testing.T) {
	a := newTestTraeCLI(t, map[string]any{"model": "gpt-5.5", "variant": "max"})
	if got := a.GetModel(); got != "gpt-5.5/max" {
		t.Errorf("GetModel() = %q, want gpt-5.5/max", got)
	}
}

func TestCodexProfile_Identity(t *testing.T) {
	a := newTestCodex(t, map[string]any{})
	if got := a.Name(); got != "codex" {
		t.Errorf("Name() = %q, want codex", got)
	}
	if got := a.CLIBinaryName(); got != "codex" {
		t.Errorf("CLIBinaryName() = %q, want codex", got)
	}
	if got := a.CLIDisplayName(); got != "Codex" {
		t.Errorf("CLIDisplayName() = %q, want Codex", got)
	}
}

// TestCodexProfile_BuildExecArgsNoVariant locks the codex path byte-for-byte:
// a codex session must never emit a model_backend_variant config flag.
func TestCodexProfile_BuildExecArgsNoVariant(t *testing.T) {
	cs, err := newCodexSession(context.Background(), "codex", nil, "/tmp/project", "gpt-5.5", "high", "full-auto", "", "", nil, "")
	if err != nil {
		t.Fatalf("newCodexSession: %v", err)
	}
	// codex sessions never set these; assert the defaults.
	if cs.supportsVariant {
		t.Fatalf("codex session supportsVariant = true, want false")
	}

	args := cs.buildExecArgs("hello", nil)

	want := []string{
		"exec",
		"--skip-git-repo-check",
		"--full-auto",
		"--model",
		"gpt-5.5",
		"-c",
		`model_reasoning_effort="high"`,
		"--json",
		"--cd",
		"/tmp/project",
		"-",
	}
	if len(args) != len(want) {
		t.Fatalf("args len = %d, want %d, args=%v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q, args=%v", i, args[i], want[i], args)
		}
	}
}

func TestTraeCLIProfile_SetModelVariantParse(t *testing.T) {
	cases := []struct {
		input       string
		wantModel   string
		wantVariant string
		wantMaxFlag bool
	}{
		{"gpt-5.5/max", "gpt-5.5", "max", true},
		{"gpt-5.5__max", "gpt-5.5", "max", true},
		{"gpt-5.5 (max)", "gpt-5.5", "max", true},
		{"gpt-5.5", "gpt-5.5", "standard", false},
		{"openrouter-3o/max", "openrouter-3o", "max", true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			a := newTestTraeCLI(t, map[string]any{})
			a.SetModel(tc.input)

			if a.model != tc.wantModel {
				t.Errorf("model = %q, want %q", a.model, tc.wantModel)
			}
			if a.variant != tc.wantVariant {
				t.Errorf("variant = %q, want %q", a.variant, tc.wantVariant)
			}

			wantGet := tc.wantModel
			if tc.wantVariant == "max" {
				wantGet = tc.wantModel + "/max"
			}
			if got := a.GetModel(); got != wantGet {
				t.Errorf("GetModel() = %q, want %q", got, wantGet)
			}

			sess, err := a.StartSession(context.Background(), "")
			if err != nil {
				t.Fatalf("StartSession: %v", err)
			}
			cs, ok := sess.(*codexSession)
			if !ok {
				t.Fatalf("session type = %T, want *codexSession", sess)
			}
			defer cs.Close()

			args := cs.buildExecArgs("hello", nil)
			hasMaxFlag := containsSequence(args, []string{"-c", `model_backend_variant="max"`})
			if hasMaxFlag != tc.wantMaxFlag {
				t.Errorf("model_backend_variant present = %v, want %v; args=%v", hasMaxFlag, tc.wantMaxFlag, args)
			}
		})
	}
}

func TestTraeCLIProfile_AvailableModelsFromCache(t *testing.T) {
	// Force the deterministic offline cache path (no app-server spawn).
	old := traecliFetchModelsFromAppServer
	traecliFetchModelsFromAppServer = nil
	t.Cleanup(func() { traecliFetchModelsFromAppServer = old })

	home := t.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata", "traecli_models_cache.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), src, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	a := newTestTraeCLI(t, map[string]any{"codex_home": home})
	models := a.AvailableModels(context.Background())

	names := make(map[string]int)
	for _, m := range models {
		names[m.Name]++
	}

	// Every model appears once as a bare entry.
	for _, bare := range []string{"GPT-5.5", "openrouter-3o", "glm-5", "gpt-5.2"} {
		if names[bare] != 1 {
			t.Errorf("bare model %q count = %d, want 1; models=%v", bare, names[bare], models)
		}
	}

	// Exactly the 2 max-capable models also appear as <slug>/max.
	maxCount := 0
	for name := range names {
		if strings.HasSuffix(name, "/max") {
			maxCount++
		}
	}
	if maxCount != 2 {
		t.Errorf("/max entries = %d, want 2; models=%v", maxCount, models)
	}
	if names["GPT-5.5/max"] != 1 {
		t.Errorf("GPT-5.5/max count = %d, want 1", names["GPT-5.5/max"])
	}
	if names["openrouter-3o/max"] != 1 {
		t.Errorf("openrouter-3o/max count = %d, want 1", names["openrouter-3o/max"])
	}

	// Non-max models produce no /max entry.
	if names["glm-5/max"] != 0 || names["gpt-5.2/max"] != 0 {
		t.Errorf("unexpected /max entry for non-max model; models=%v", models)
	}

	// No duplicate entries overall.
	for name, c := range names {
		if c != 1 {
			t.Errorf("duplicate entry %q count = %d", name, c)
		}
	}
}

// TestTraeCLIProfile_AvailableModelsAppServerFallback locks the source ordering:
// when the app-server fetch fails the cache is used, and when it succeeds it
// takes precedence over the cache.
func TestTraeCLIProfile_AvailableModelsAppServerFallback(t *testing.T) {
	home := t.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata", "traecli_models_cache.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), src, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	old := traecliFetchModelsFromAppServer
	t.Cleanup(func() { traecliFetchModelsFromAppServer = old })

	// 1. app-server fails -> fall back to the cache.
	traecliFetchModelsFromAppServer = func(context.Context, string, string, []string) ([]core.ModelOption, error) {
		return nil, fmt.Errorf("not logged in")
	}
	a := newTestTraeCLI(t, map[string]any{"codex_home": home})
	models := a.AvailableModels(context.Background())
	found := false
	for _, m := range models {
		if m.Name == "GPT-5.5" {
			found = true
		}
	}
	if !found {
		t.Errorf("app-server error should fall back to cache; got %v", models)
	}

	// 2. app-server succeeds -> its result wins over the cache.
	traecliFetchModelsFromAppServer = func(context.Context, string, string, []string) ([]core.ModelOption, error) {
		return []core.ModelOption{{Name: "live-only-model"}}, nil
	}
	models = a.AvailableModels(context.Background())
	if len(models) != 1 || models[0].Name != "live-only-model" {
		t.Errorf("app-server result should take precedence over cache; got %v", models)
	}
}

func TestTraeCLIProfile_AvailableModelsCaseInsensitiveDedup(t *testing.T) {
	old := traecliFetchModelsFromAppServer
	traecliFetchModelsFromAppServer = nil
	t.Cleanup(func() { traecliFetchModelsFromAppServer = old })

	home := t.TempDir()
	cache := `{"models":[
		{"slug":"GPT-5.5","visibility":"list","supported_in_api":true,"business_metadata":{"variants":{"max_key":"k"}}},
		{"slug":"gpt-5.5","visibility":"list","supported_in_api":true,"business_metadata":{"variants":{"max_key":"k"}}}
	]}`
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(cache), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	a := newTestTraeCLI(t, map[string]any{"codex_home": home})
	models := a.AvailableModels(context.Background())

	// Case-insensitive dedup: one bare + one /max, despite two mixed-case slugs.
	if len(models) != 2 {
		t.Fatalf("models len = %d, want 2 (case-insensitive dedup); models=%v", len(models), models)
	}
}

func TestParseModelListResult(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "traecli_model_list_response.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	models := parseModelListResult(raw)

	names := make(map[string]int)
	for _, m := range models {
		names[m.Name]++
	}

	for _, bare := range []string{"gpt-5.5", "openrouter-3o", "glm-5", "gpt-5.2"} {
		if names[bare] != 1 {
			t.Errorf("bare model %q count = %d, want 1; models=%v", bare, names[bare], models)
		}
	}
	if names["gpt-5.5/max"] != 1 {
		t.Errorf("gpt-5.5/max count = %d, want 1; models=%v", names["gpt-5.5/max"], models)
	}
	if names["openrouter-3o/max"] != 1 {
		t.Errorf("openrouter-3o/max count = %d, want 1; models=%v", names["openrouter-3o/max"], models)
	}
	maxCount := 0
	for name := range names {
		if strings.HasSuffix(name, "/max") {
			maxCount++
		}
	}
	if maxCount != 2 {
		t.Errorf("/max entries = %d, want 2; models=%v", maxCount, models)
	}
	for name, c := range names {
		if c != 1 {
			t.Errorf("duplicate entry %q count = %d", name, c)
		}
	}
}

func TestParseModelListResult_AcceptsBareResult(t *testing.T) {
	// Bare result payload (no top-level "result" wrapper) must also parse.
	raw := []byte(`{"data":[{"id":"gpt-5.5","businessMetadata":{"variants":{"max_key":"k"}}}]}`)
	models := parseModelListResult(raw)
	if len(models) != 2 {
		t.Fatalf("models len = %d, want 2; models=%v", len(models), models)
	}
}

// TestIntegration_TraeCLIViaCliPath exercises the live traecli binary end to
// end: it sends a prompt, asserts a non-empty response and session id, and
// verifies AvailableModels returns the dynamic catalog (more than the OpenAI
// placeholders, including at least one /max entry). Gated behind CC_TRAECLI_E2E
// because it requires a logged-in traecli.
func TestIntegration_TraeCLIViaCliPath(t *testing.T) {
	if os.Getenv("CC_TRAECLI_E2E") == "" {
		t.Skip("CC_TRAECLI_E2E not set, skipping traecli live e2e")
	}

	a, err := NewTraeCLI(map[string]any{
		"work_dir": t.TempDir(),
		"mode":     "yolo",
	})
	if err != nil {
		t.Fatalf("NewTraeCLI: %v", err)
	}
	agent := a.(*Agent)

	ctx := context.Background()
	sess, err := a.StartSession(ctx, "")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer sess.Close()

	if err := sess.Send("reply with exactly 'traecli-ok' and nothing else", nil, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var gotText string
	for {
		evt, ok := <-sess.Events()
		if !ok {
			break
		}
		if evt.Type == core.EventError {
			t.Fatalf("error event: %v", evt.Error)
		}
		if evt.Type == core.EventText {
			gotText += evt.Content
		}
		if evt.Type == core.EventResult && evt.Done {
			break
		}
	}
	if strings.TrimSpace(gotText) == "" {
		t.Fatalf("empty response from traecli")
	}
	if sess.CurrentSessionID() == "" {
		t.Fatalf("empty session id")
	}

	models := agent.AvailableModels(ctx)
	if len(models) <= 6 {
		t.Errorf("AvailableModels returned %d entries, want > 6 (dynamic catalog)", len(models))
	}
	hasMax := false
	for _, m := range models {
		if strings.HasSuffix(m.Name, "/max") {
			hasMax = true
			break
		}
	}
	if !hasMax {
		t.Errorf("AvailableModels has no /max entry; models=%v", models)
	}
}
