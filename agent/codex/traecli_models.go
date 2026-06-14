package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chenhg5/cc-connect/core"
)

// traecliModelOption is a normalized model entry decoupled from the two
// differently-shaped sources (app-server `model/list` and the on-disk cache).
type traecliModelOption struct {
	slug   string
	maxKey string // non-empty when the model exposes a "max" backend variant
}

// expandTraeCLIModelVariants turns normalized entries into ModelOptions,
// emitting a bare entry per model plus a `<slug>/max` entry when the model has
// a non-empty max_key. Names are de-duplicated case-insensitively so the same
// model never appears twice when the two sources use different slug casing.
func expandTraeCLIModelVariants(entries []traecliModelOption) []core.ModelOption {
	out := make([]core.ModelOption, 0, len(entries))
	seen := make(map[string]struct{}, len(entries)*2)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, core.ModelOption{Name: name})
	}
	for _, e := range entries {
		slug := strings.TrimSpace(e.slug)
		if slug == "" {
			continue
		}
		add(slug)
		if strings.TrimSpace(e.maxKey) != "" {
			add(slug + "/max")
		}
	}
	return out
}

// appServerModelEntry matches an entry in the app-server `model/list`
// `result.data[]` array (camelCase keys, slug under "id").
type appServerModelEntry struct {
	ID               string `json:"id"`
	BusinessMetadata struct {
		Variants struct {
			MaxKey string `json:"max_key"`
		} `json:"variants"`
	} `json:"businessMetadata"`
}

// parseModelListResult maps a JSON-RPC `model/list` response into ModelOptions.
// It accepts both the full envelope (`{"result":{"data":[...]}}`) and the bare
// result payload (`{"data":[...]}`) so it works against captured fixtures and
// the raw result returned by the app-server handshake. For each entry it emits
// a bare `ModelOption{Name: id}` plus `Name: id + "/max"` when
// businessMetadata.variants.max_key is a non-empty string.
func parseModelListResult(raw []byte) []core.ModelOption {
	var env struct {
		Result struct {
			Data []appServerModelEntry `json:"data"`
		} `json:"result"`
		Data []appServerModelEntry `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil
	}
	entries := env.Result.Data
	if len(entries) == 0 {
		entries = env.Data
	}
	opts := make([]traecliModelOption, 0, len(entries))
	for _, e := range entries {
		opts = append(opts, traecliModelOption{slug: e.ID, maxKey: e.BusinessMetadata.Variants.MaxKey})
	}
	return expandTraeCLIModelVariants(opts)
}

// readTraeCLICachedModels reads traecli's `<codexHome>/models_cache.json` and
// applies the same bare + `/max` expansion as the app-server path. The cache
// uses snake_case keys (slug under "slug", variants under "business_metadata").
// It respects the same visibility / supported_in_api filtering style as
// readCodexCachedModels.
func readTraeCLICachedModels(codexHome string) []core.ModelOption {
	codexHome = strings.TrimSpace(codexHome)
	if codexHome == "" {
		codexHome = traecliProfile.defaultHome
	}
	if codexHome == "" {
		return nil
	}
	path := filepath.Join(codexHome, "models_cache.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var payload struct {
		Models []struct {
			Slug             string `json:"slug"`
			DisplayName      string `json:"display_name"`
			Visibility       string `json:"visibility"`
			SupportedInAPI   bool   `json:"supported_in_api"`
			BusinessMetadata struct {
				Variants struct {
					MaxKey string `json:"max_key"`
				} `json:"variants"`
			} `json:"business_metadata"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil
	}
	opts := make([]traecliModelOption, 0, len(payload.Models))
	for _, m := range payload.Models {
		slug := strings.TrimSpace(m.Slug)
		if slug == "" {
			slug = strings.TrimSpace(m.DisplayName)
		}
		if slug == "" {
			continue
		}
		if m.Visibility != "" && m.Visibility != "list" {
			continue
		}
		if !m.SupportedInAPI {
			continue
		}
		opts = append(opts, traecliModelOption{slug: slug, maxKey: m.BusinessMetadata.Variants.MaxKey})
	}
	return expandTraeCLIModelVariants(opts)
}

// traecliFetchModelsFromAppServer is a test seam: tests can override it to
// force the deterministic offline cache path without spawning a subprocess.
var traecliFetchModelsFromAppServer = fetchTraeCLIModelsFromAppServer

// availableTraeCLIModels resolves the dynamic traecli model catalog: try the
// app-server `model/list` first, fall back to the on-disk cache on any error,
// and return nil when both are empty.
func (a *Agent) availableTraeCLIModels(ctx context.Context) []core.ModelOption {
	a.mu.RLock()
	cliBin := a.cliBin
	codexHome := a.codexHome
	workDir := a.workDir
	extraEnv := append([]string{}, a.sessionEnv...)
	a.mu.RUnlock()

	if traecliFetchModelsFromAppServer != nil {
		models, err := traecliFetchModelsFromAppServer(ctx, cliBin, workDir, extraEnv)
		if err != nil {
			slog.Debug("traecli: app-server model/list failed, falling back to cache", "error", err)
		} else if len(models) > 0 {
			return models
		}
	}

	if models := readTraeCLICachedModels(codexHome); len(models) > 0 {
		return models
	}
	return nil
}

// fetchTraeCLIModelsFromAppServer spawns `<cliBin> app-server`, performs the
// JSON-RPC initialize/initialized handshake (mirroring loadCodexRuntimeConfig),
// calls `model/list` with empty params, and parses `result.data[]`.
func fetchTraeCLIModelsFromAppServer(ctx context.Context, cliBin, workDir string, extraEnv []string) ([]core.ModelOption, error) {
	if strings.TrimSpace(cliBin) == "" {
		cliBin = traecliProfile.defaultBin
	}
	cmd := exec.CommandContext(ctx, cliBin, "app-server")
	cmd.Dir = workDir
	prepareCmdForKill(cmd)
	if len(extraEnv) > 0 {
		cmd.Env = core.MergeEnv(os.Environ(), extraEnv)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("traecli model list stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("traecli model list stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("traecli model list start app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	reader := bufio.NewReader(stdout)
	nextID := int64(1)

	if err := rpcRequestOverIO(stdin, reader, nextID, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "cc-connect-traecli-model-list",
			"title":   "CC Connect TRAE CLI Model List",
			"version": "0.1.0",
		},
	}, nil); err != nil {
		return nil, err
	}
	nextID++

	if err := rpcNotifyOverIO(stdin, "initialized", map[string]any{}); err != nil {
		return nil, err
	}

	var result json.RawMessage
	if err := rpcRequestOverIO(stdin, reader, nextID, "model/list", map[string]any{}, &result); err != nil {
		return nil, err
	}

	return parseModelListResult(result), nil
}
