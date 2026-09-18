# Dashboard Runtime Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let operators change core proxy settings and the upstream key list from the dashboard, applied at runtime and persisted to the config file.

**Architecture:** New `/admin/config` GET and PATCH endpoints in `config_admin.go` merge a JSON patch into the live config, rewrite the YAML config file while preserving comments, reload from disk, then publish the new config through atomic pointers shared by the key manager, alert notifier, workspace client, and usage poller. The dashboard gets a Proxy Configuration dialog built from the existing render and event delegation patterns.

**Tech Stack:** Go 1.22, `gopkg.in/yaml.v3`, `net/http`, vanilla TypeScript, Vite 7.

**Spec:** `docs/superpowers/specs/2026-09-18-dashboard-runtime-config-design.md`

## Global Constraints

- All Go code lives in package `main` in the repository root.
- Use `gopkg.in/yaml.v3` for all YAML work; add no other dependency.
- Every commit is signed off: `git commit -s`.
- Config file writes keep the existing file mode, or use `0600` for new files; new config directories use `0700`.
- Never return or log upstream key plaintext. GET returns only `maskKeyHint` output.
- `validateConfig` stays the single source of truth for accepted values.
- Frontend code stays vanilla TypeScript; no new runtime dependencies.
- Work in the worktree `.worktrees/dashboard-runtime-config` on branch `dashboard-runtime-config`.
- Run every Go command from the worktree root; run every npm command from `web/dashboard`.

---

### Task 1: Config response, revision, and env locks

**Files:**
- Create: `config_admin.go`
- Test: `config_admin_test.go`

**Interfaces:**
- Consumes: `Config`, `UpstreamKeyConfig`, `maskKeyHint`, `defaultString` from `main.go` and `dashboard.go`.
- Produces:
  - `type configSettingsState struct{...}` with JSON tags `routing_strategy`, `session_ttl`, `balanced_idle_timeout`, `proactive_switch_threshold`, `retry_exhausted_after`, `usage_check_interval`, `disable_usage_polling`, `sanitize_developer_role`
  - `type configKeyState struct{ ID int; KeyHint string; Priority int; Weight int }`
  - `type configResponse struct{ ConfigSource string; Editable bool; Revision string; EnvLocked []string; Settings configSettingsState; ModelAliases map[string]string; Keys []configKeyState }`
  - `func effectiveKeyConfigs(cfg Config) []UpstreamKeyConfig`
  - `func normalizedKey(kc UpstreamKeyConfig) UpstreamKeyConfig`
  - `func settingsState(cfg Config) configSettingsState`
  - `func keyStates(cfg Config) []configKeyState`
  - `func configRevision(cfg Config) string`
  - `func envLockedFields() []string`
  - `func resolveWritableConfigPath(cfg Config) string`
  - `func buildConfigResponse(cfg Config, editable bool) configResponse`

- [ ] **Step 1: Write the failing tests**

Create `config_admin_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func testConfig() Config {
	cfg := defaultConfig()
	cfg.ProxyAPIKey = "proxy-key"
	cfg.UpstreamAPIKeys = []string{"sk-abcdefghijklmnop", "sk-qrstuvwxyz012345"}
	cfg.UpstreamKeyConfigs = []UpstreamKeyConfig{
		{Key: "sk-abcdefghijklmnop", Priority: 1, Weight: 3},
		{Key: "sk-qrstuvwxyz012345", Priority: 2, Weight: 1},
	}
	return cfg
}

func TestBuildConfigResponseMasksKeys(t *testing.T) {
	cfg := testConfig()
	resp := buildConfigResponse(cfg, true)
	if len(resp.Keys) != 2 {
		t.Fatalf("keys = %d", len(resp.Keys))
	}
	for i, k := range resp.Keys {
		if k.KeyHint == cfg.UpstreamKeyConfigs[i].Key {
			t.Fatalf("key %d leaked plaintext", i)
		}
	}
	if resp.Keys[0].Priority != 1 || resp.Keys[0].Weight != 3 {
		t.Fatalf("unexpected first key: %+v", resp.Keys[0])
	}
	if resp.ConfigSource != "none" {
		t.Fatalf("config source = %q", resp.ConfigSource)
	}
}

func TestConfigRevisionChangesWithKeyPriorityAndSecret(t *testing.T) {
	cfg := testConfig()
	base := configRevision(cfg)
	if base == "" {
		t.Fatal("empty revision")
	}
	if configRevision(cfg) != base {
		t.Fatal("revision not stable for equal state")
	}

	priority := cfg
	priority.UpstreamKeyConfigs = append([]UpstreamKeyConfig(nil), cfg.UpstreamKeyConfigs...)
	priority.UpstreamKeyConfigs[1].Priority = 1
	if configRevision(priority) == base {
		t.Fatal("revision did not change after priority update")
	}

	rotated := cfg
	rotated.UpstreamKeyConfigs = append([]UpstreamKeyConfig(nil), cfg.UpstreamKeyConfigs...)
	// Same first 7 and last 4 characters as the original, so the hint is identical.
	rotated.UpstreamKeyConfigs[0].Key = "sk-abcd-DIFFERENT-BODY-mnop"
	if configRevision(rotated) == base {
		t.Fatal("revision did not change after key rotation")
	}
}

func TestEnvLockedFieldsReportsSetVariables(t *testing.T) {
	t.Setenv("ROUTING_STRATEGY", "balanced")
	t.Setenv("MODEL_ALIASES", "a=b")
	locked := envLockedFields()
	joined := strings.Join(locked, ",")
	for _, want := range []string{"routing_strategy", "model_aliases"} {
		found := false
		for _, name := range locked {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}

func TestResolveWritableConfigPath(t *testing.T) {
	cfg := testConfig()
	cfg.ConfigSourcePath = "/tmp/switchboard-test/config.yaml"
	if got := resolveWritableConfigPath(cfg); got != cfg.ConfigSourcePath {
		t.Fatalf("got %q", got)
	}
	cfg.ConfigSourcePath = ""
	t.Setenv("SWITCHBOARD_GO_CONFIG", "/tmp/explicit.yaml")
	if got := resolveWritableConfigPath(cfg); got != "/tmp/explicit.yaml" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./ -run 'TestBuildConfigResponseMasksKeys|TestConfigRevision|TestEnvLockedFields|TestResolveWritableConfigPath' -v`
Expected: FAIL to build with `undefined: buildConfigResponse` and similar.

- [ ] **Step 3: Write the implementation**

Create `config_admin.go`:

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// configSettingsState is the JSON shape of the settings the dashboard edits.
type configSettingsState struct {
	RoutingStrategy          string  `json:"routing_strategy"`
	SessionTTL               string  `json:"session_ttl"`
	BalancedIdleTimeout      string  `json:"balanced_idle_timeout"`
	ProactiveSwitchThreshold float64 `json:"proactive_switch_threshold"`
	RetryExhaustedAfter      string  `json:"retry_exhausted_after"`
	UsageCheckInterval       string  `json:"usage_check_interval"`
	DisableUsagePolling      bool    `json:"disable_usage_polling"`
	SanitizeDeveloperRole    bool    `json:"sanitize_developer_role"`
}

type configKeyState struct {
	ID       int    `json:"id"`
	KeyHint  string `json:"key_hint"`
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
}

type configResponse struct {
	ConfigSource string              `json:"config_source"`
	Editable     bool                `json:"editable"`
	Revision     string              `json:"revision"`
	EnvLocked    []string            `json:"env_locked"`
	Settings     configSettingsState `json:"settings"`
	ModelAliases map[string]string   `json:"model_aliases"`
	Keys         []configKeyState    `json:"keys"`
}

// effectiveKeyConfigs falls back to the plain key list so env-only configs and
// tests still report priorities and weights.
func effectiveKeyConfigs(cfg Config) []UpstreamKeyConfig {
	if len(cfg.UpstreamKeyConfigs) > 0 {
		return cfg.UpstreamKeyConfigs
	}
	out := make([]UpstreamKeyConfig, len(cfg.UpstreamAPIKeys))
	for i, k := range cfg.UpstreamAPIKeys {
		out[i] = UpstreamKeyConfig{Key: k, Priority: 1, Weight: 1}
	}
	return out
}

func normalizedKey(kc UpstreamKeyConfig) UpstreamKeyConfig {
	if kc.Priority <= 0 {
		kc.Priority = 1
	}
	if kc.Weight <= 0 {
		kc.Weight = 1
	}
	return kc
}

func settingsState(cfg Config) configSettingsState {
	return configSettingsState{
		RoutingStrategy:          defaultString(cfg.RoutingStrategy, "session_sticky"),
		SessionTTL:               cfg.SessionTTL.String(),
		BalancedIdleTimeout:      cfg.BalancedIdleTimeout.String(),
		ProactiveSwitchThreshold: cfg.ProactiveSwitchThreshold,
		RetryExhaustedAfter:      cfg.RetryExhaustedAfter.String(),
		UsageCheckInterval:       cfg.UsageCheckInterval.String(),
		DisableUsagePolling:      cfg.DisableUsagePolling,
		SanitizeDeveloperRole:    cfg.SanitizeDeveloperRole,
	}
}

func keyStates(cfg Config) []configKeyState {
	configs := effectiveKeyConfigs(cfg)
	out := make([]configKeyState, len(configs))
	for i, kc := range configs {
		kc = normalizedKey(kc)
		out[i] = configKeyState{
			ID:       i,
			KeyHint:  maskKeyHint(kc.Key),
			Priority: kc.Priority,
			Weight:   kc.Weight,
		}
	}
	return out
}

type revisionKey struct {
	KeySHA256 string `json:"key_sha256"`
	Priority  int    `json:"priority"`
	Weight    int    `json:"weight"`
}

// configRevision hashes the editable state so two browser tabs cannot clobber
// each other. Plaintext keys never enter the hash input.
func configRevision(cfg Config) string {
	configs := effectiveKeyConfigs(cfg)
	keys := make([]revisionKey, len(configs))
	for i, kc := range configs {
		kc = normalizedKey(kc)
		sum := sha256.Sum256([]byte(kc.Key))
		keys[i] = revisionKey{
			KeySHA256: hex.EncodeToString(sum[:]),
			Priority:  kc.Priority,
			Weight:    kc.Weight,
		}
	}
	doc := struct {
		Settings     configSettingsState `json:"settings"`
		ModelAliases map[string]string   `json:"model_aliases"`
		Keys         []revisionKey       `json:"keys"`
	}{
		Settings:     settingsState(cfg),
		ModelAliases: cfg.ModelAliases,
		Keys:         keys,
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var configEnvLocks = []struct {
	Field string
	Env   string
}{
	{"routing_strategy", "ROUTING_STRATEGY"},
	{"session_ttl", "SESSION_TTL"},
	{"balanced_idle_timeout", "BALANCED_IDLE_TIMEOUT"},
	{"usage_check_interval", "USAGE_CHECK_INTERVAL"},
	{"disable_usage_polling", "DISABLE_USAGE_POLLING"},
	{"proactive_switch_threshold", "PROACTIVE_SWITCH_THRESHOLD"},
	{"retry_exhausted_after", "RETRY_EXHAUSTED_AFTER"},
	{"sanitize_developer_role", "SANITIZE_DEVELOPER_ROLE"},
	{"model_aliases", "MODEL_ALIASES"},
	{"keys", "OPENCODE_GO_API_KEYS"},
}

func envLockedFields() []string {
	out := []string{}
	for _, l := range configEnvLocks {
		if strings.TrimSpace(os.Getenv(l.Env)) != "" {
			out = append(out, l.Field)
		}
	}
	return out
}

// resolveWritableConfigPath returns the file PATCH writes, or "" when none can
// be resolved.
func resolveWritableConfigPath(cfg Config) string {
	if path := strings.TrimSpace(cfg.ConfigSourcePath); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("SWITCHBOARD_GO_CONFIG")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".config", "switchboard-go", "config.yaml")
}

func buildConfigResponse(cfg Config, editable bool) configResponse {
	aliases := make(map[string]string, len(cfg.ModelAliases))
	for k, v := range cfg.ModelAliases {
		aliases[k] = v
	}
	return configResponse{
		ConfigSource: defaultString(cfg.ConfigSourcePath, "none"),
		Editable:     editable,
		Revision:     configRevision(cfg),
		EnvLocked:    envLockedFields(),
		Settings:     settingsState(cfg),
		ModelAliases: aliases,
		Keys:         keyStates(cfg),
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./ -run 'TestBuildConfigResponseMasksKeys|TestConfigRevision|TestEnvLockedFields|TestResolveWritableConfigPath' -v`
Expected: PASS.

- [ ] **Step 5: Run gofmt and the full suite**

Run: `gofmt -l config_admin.go config_admin_test.go`
Expected: no output.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add config_admin.go config_admin_test.go
git commit -s -m "feat(config): add dashboard config response and revision helpers"
```

---

### Task 2: Patch merge

**Files:**
- Modify: `config_admin.go`
- Test: `config_admin_test.go`

**Interfaces:**
- Consumes: `effectiveKeyConfigs`, `normalizedKey` from Task 1, `defaultConfig`, `Config`, `UpstreamKeyConfig`.
- Produces:
  - `type configPatchRequest struct{ IfRevision *string; Settings map[string]json.RawMessage; ModelAliases map[string]*string; Keys *[]configKeyPatch }`
  - `type configKeyPatch struct{ ID *int; Key *string; Priority *int; Weight *int }`
  - `func applyConfigPatch(current Config, req configPatchRequest) (Config, []string, error)`
  - `func envLockViolation(req configPatchRequest, locked []string) string`
  - `func keysFromConfigs(configs []UpstreamKeyConfig) []string`

- [ ] **Step 1: Write the failing tests**

Append to `config_admin_test.go` and add `"encoding/json"` to its import block (`strings` is already there from Task 1):

```go
func patchJSON(t *testing.T, body string) configPatchRequest {
	t.Helper()
	var req configPatchRequest
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestApplyConfigPatchSettingsAndNullReset(t *testing.T) {
	cfg := testConfig()
	next, changed, err := applyConfigPatch(cfg, patchJSON(t, `{"settings":{"routing_strategy":"balanced","proactive_switch_threshold":80}}`))
	if err != nil {
		t.Fatal(err)
	}
	if next.RoutingStrategy != "balanced" || next.ProactiveSwitchThreshold != 80 {
		t.Fatalf("unexpected settings: %+v", settingsState(next))
	}
	if len(changed) != 2 {
		t.Fatalf("changed = %v", changed)
	}

	next, _, err = applyConfigPatch(next, patchJSON(t, `{"settings":{"routing_strategy":null,"proactive_switch_threshold":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	if next.RoutingStrategy != "session_sticky" || next.ProactiveSwitchThreshold != 95 {
		t.Fatalf("null did not reset to defaults: %+v", settingsState(next))
	}
}

func TestApplyConfigPatchRejectsUnknownSetting(t *testing.T) {
	_, _, err := applyConfigPatch(testConfig(), patchJSON(t, `{"settings":{"nope":1}}`))
	if err == nil {
		t.Fatal("expected error for unknown setting")
	}
}

func TestApplyConfigPatchRejectsBadDuration(t *testing.T) {
	_, _, err := applyConfigPatch(testConfig(), patchJSON(t, `{"settings":{"session_ttl":"soon"}}`))
	if err == nil {
		t.Fatal("expected error for bad duration")
	}
}

func TestApplyConfigPatchAliases(t *testing.T) {
	cfg := testConfig()
	cfg.ModelAliases = map[string]string{"keep": "a", "drop": "b"}
	next, _, err := applyConfigPatch(cfg, patchJSON(t, `{"model_aliases":{"drop":null,"add":"c"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next.ModelAliases["drop"]; ok {
		t.Fatal("drop not removed")
	}
	if next.ModelAliases["add"] != "c" || next.ModelAliases["keep"] != "a" {
		t.Fatalf("aliases = %v", next.ModelAliases)
	}
}

func TestApplyConfigPatchKeys(t *testing.T) {
	cfg := testConfig()
	next, changed, err := applyConfigPatch(cfg, patchJSON(t, `{"keys":[{"id":0,"priority":2},{"id":1,"key":"sk-rotated-abcdefghijkl"},{"key":"sk-new-key-123456","priority":1,"weight":4}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(next.UpstreamKeyConfigs) != 3 {
		t.Fatalf("keys = %d", len(next.UpstreamKeyConfigs))
	}
	first := next.UpstreamKeyConfigs[0]
	if first.Key != cfg.UpstreamKeyConfigs[0].Key || first.Priority != 2 || first.Weight != 3 {
		t.Fatalf("first key = %+v", first)
	}
	if next.UpstreamKeyConfigs[1].Key != "sk-rotated-abcdefghijkl" {
		t.Fatalf("second key = %+v", next.UpstreamKeyConfigs[1])
	}
	last := next.UpstreamKeyConfigs[2]
	if last.Key != "sk-new-key-123456" || last.Priority != 1 || last.Weight != 4 {
		t.Fatalf("new key = %+v", last)
	}
	if next.UpstreamAPIKeys[2] != "sk-new-key-123456" {
		t.Fatalf("plain key list out of sync: %v", next.UpstreamAPIKeys)
	}
	if len(changed) != 1 || changed[0] != "keys" {
		t.Fatalf("changed = %v", changed)
	}
}

func TestApplyConfigPatchRejectsDuplicateKeyIDs(t *testing.T) {
	_, _, err := applyConfigPatch(testConfig(), patchJSON(t, `{"keys":[{"id":0,"priority":1},{"id":0,"priority":2}]}`))
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestApplyConfigPatchRejectsEmptyKeyList(t *testing.T) {
	next, _, err := applyConfigPatch(testConfig(), patchJSON(t, `{"keys":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConfig(next); err == nil {
		t.Fatal("expected validation failure for empty key list")
	}
}

func TestEnvLockViolation(t *testing.T) {
	req := patchJSON(t, `{"settings":{"routing_strategy":"balanced"}}`)
	if got := envLockViolation(req, []string{"routing_strategy"}); got != "routing_strategy" {
		t.Fatalf("got %q", got)
	}
	if got := envLockViolation(req, []string{"session_ttl"}); got != "" {
		t.Fatalf("got %q", got)
	}
	keys := patchJSON(t, `{"keys":[{"key":"sk-x-1234567890"}]}`)
	if got := envLockViolation(keys, []string{"keys"}); got != "keys" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./ -run 'TestApplyConfigPatch|TestEnvLockViolation' -v`
Expected: FAIL to build with `undefined: configPatchRequest`.

- [ ] **Step 3: Write the implementation**

Append to `config_admin.go`. Add `"fmt"`, `"sort"`, `"time"` to its import block:

```go
// configPatchRequest is the PATCH /admin/config body.
type configPatchRequest struct {
	IfRevision   *string                    `json:"if_revision"`
	Settings     map[string]json.RawMessage `json:"settings"`
	ModelAliases map[string]*string         `json:"model_aliases"`
	Keys         *[]configKeyPatch          `json:"keys"`
}

type configKeyPatch struct {
	ID       *int    `json:"id"`
	Key      *string `json:"key"`
	Priority *int    `json:"priority"`
	Weight   *int    `json:"weight"`
}

func envLockViolation(req configPatchRequest, locked []string) string {
	lockedSet := make(map[string]bool, len(locked))
	for _, name := range locked {
		lockedSet[name] = true
	}
	if req.Keys != nil && lockedSet["keys"] {
		return "keys"
	}
	if req.ModelAliases != nil && lockedSet["model_aliases"] {
		return "model_aliases"
	}
	for name := range req.Settings {
		if lockedSet[name] {
			return name
		}
	}
	return ""
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func decodeStringSetting(raw json.RawMessage, name, fallback string) (string, error) {
	if isJSONNull(raw) {
		return fallback, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", name)
	}
	return strings.TrimSpace(s), nil
}

func decodeDurationSetting(raw json.RawMessage, name string, fallback time.Duration) (time.Duration, error) {
	if isJSONNull(raw) {
		return fallback, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, fmt.Errorf("%s must be a duration string", name)
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s must be a valid duration >= 0", name)
	}
	return d, nil
}

func decodeBoolSetting(raw json.RawMessage, name string, fallback bool) (bool, error) {
	if isJSONNull(raw) {
		return fallback, nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return b, nil
}

func decodeNumberSetting(raw json.RawMessage, name string, fallback float64) (float64, error) {
	if isJSONNull(raw) {
		return fallback, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, fmt.Errorf("%s must be a number", name)
	}
	return f, nil
}

// applyConfigPatch merges the request into a copy of current. It returns the
// merged config and the JSON names of the touched fields. The caller validates
// the result and persists it.
func applyConfigPatch(current Config, req configPatchRequest) (Config, []string, error) {
	next := current
	changed := []string{}
	defaults := defaultConfig()

	names := make([]string, 0, len(req.Settings))
	for name := range req.Settings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := req.Settings[name]
		switch name {
		case "routing_strategy":
			v, err := decodeStringSetting(raw, name, defaults.RoutingStrategy)
			if err != nil {
				return Config{}, nil, err
			}
			next.RoutingStrategy = v
		case "session_ttl":
			v, err := decodeDurationSetting(raw, name, defaults.SessionTTL)
			if err != nil {
				return Config{}, nil, err
			}
			next.SessionTTL = v
		case "balanced_idle_timeout":
			v, err := decodeDurationSetting(raw, name, defaults.BalancedIdleTimeout)
			if err != nil {
				return Config{}, nil, err
			}
			next.BalancedIdleTimeout = v
		case "usage_check_interval":
			v, err := decodeDurationSetting(raw, name, defaults.UsageCheckInterval)
			if err != nil {
				return Config{}, nil, err
			}
			next.UsageCheckInterval = v
		case "retry_exhausted_after":
			v, err := decodeDurationSetting(raw, name, defaults.RetryExhaustedAfter)
			if err != nil {
				return Config{}, nil, err
			}
			next.RetryExhaustedAfter = v
		case "proactive_switch_threshold":
			v, err := decodeNumberSetting(raw, name, defaults.ProactiveSwitchThreshold)
			if err != nil {
				return Config{}, nil, err
			}
			next.ProactiveSwitchThreshold = v
		case "disable_usage_polling":
			v, err := decodeBoolSetting(raw, name, defaults.DisableUsagePolling)
			if err != nil {
				return Config{}, nil, err
			}
			next.DisableUsagePolling = v
		case "sanitize_developer_role":
			v, err := decodeBoolSetting(raw, name, defaults.SanitizeDeveloperRole)
			if err != nil {
				return Config{}, nil, err
			}
			next.SanitizeDeveloperRole = v
		default:
			return Config{}, nil, fmt.Errorf("unknown setting %q", name)
		}
		changed = append(changed, name)
	}

	if req.ModelAliases != nil {
		aliases := make(map[string]string, len(current.ModelAliases))
		for k, v := range current.ModelAliases {
			aliases[k] = v
		}
		for k, v := range req.ModelAliases {
			name := strings.TrimSpace(k)
			if name == "" {
				return Config{}, nil, fmt.Errorf("model_aliases contains an empty alias name")
			}
			if v == nil {
				delete(aliases, name)
				continue
			}
			target := strings.TrimSpace(*v)
			if target == "" {
				return Config{}, nil, fmt.Errorf("model alias %q needs a non-empty target", name)
			}
			aliases[name] = target
		}
		next.ModelAliases = aliases
		changed = append(changed, "model_aliases")
	}

	if req.Keys != nil {
		currentConfigs := effectiveKeyConfigs(current)
		patched := make([]UpstreamKeyConfig, 0, len(*req.Keys))
		seenIDs := map[int]bool{}
		seenKeys := map[string]bool{}
		for _, kp := range *req.Keys {
			var kc UpstreamKeyConfig
			if kp.ID != nil {
				id := *kp.ID
				if id < 0 || id >= len(currentConfigs) {
					return Config{}, nil, fmt.Errorf("key id %d is out of range", id)
				}
				if seenIDs[id] {
					return Config{}, nil, fmt.Errorf("duplicate key id %d", id)
				}
				seenIDs[id] = true
				kc = normalizedKey(currentConfigs[id])
				if kp.Key != nil {
					if strings.TrimSpace(*kp.Key) == "" {
						return Config{}, nil, fmt.Errorf("key id %d rotation value must not be empty", id)
					}
					kc.Key = strings.TrimSpace(*kp.Key)
				}
			} else {
				if kp.Key == nil || strings.TrimSpace(*kp.Key) == "" {
					return Config{}, nil, fmt.Errorf("new key entries must include a non-empty key")
				}
				kc = UpstreamKeyConfig{Key: strings.TrimSpace(*kp.Key), Priority: 1, Weight: 1}
			}
			if kp.Priority != nil {
				if *kp.Priority < 1 {
					return Config{}, nil, fmt.Errorf("key priority must be >= 1")
				}
				kc.Priority = *kp.Priority
			}
			if kp.Weight != nil {
				if *kp.Weight < 1 {
					return Config{}, nil, fmt.Errorf("key weight must be >= 1")
				}
				kc.Weight = *kp.Weight
			}
			if seenKeys[kc.Key] {
				return Config{}, nil, fmt.Errorf("duplicate key value")
			}
			seenKeys[kc.Key] = true
			patched = append(patched, kc)
		}
		next.UpstreamKeyConfigs = patched
		next.UpstreamAPIKeys = keysFromConfigs(patched)
		changed = append(changed, "keys")
	}

	return next, changed, nil
}

func keysFromConfigs(configs []UpstreamKeyConfig) []string {
	out := make([]string, len(configs))
	for i, c := range configs {
		out[i] = c.Key
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./ -run 'TestApplyConfigPatch|TestEnvLockViolation' -v`
Expected: PASS.

- [ ] **Step 5: Run gofmt and the full suite**

Run: `gofmt -l config_admin.go config_admin_test.go`
Expected: no output.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add config_admin.go config_admin_test.go
git commit -s -m "feat(config): merge dashboard config patches"
```

---

### Task 3: YAML persistence

**Files:**
- Modify: `config_admin.go`
- Test: `config_admin_test.go`

**Interfaces:**
- Consumes: `Config`, `effectiveKeyConfigs`, `normalizedKey`, `defaultString`.
- Produces:
  - `func persistConfigChanges(path string, cfg Config, changed []string) ([]byte, error)`
  - `func atomicWriteFile(path string, data []byte, mode os.FileMode) error`
  - unexported helpers `rootMapping`, `ensureMapping`, `mappingSet`, `mappingDelete`, `mappingValue`, `encodeYAMLValue`, `yamlSettingValue`, `applyYAMLUpdates`

- [ ] **Step 1: Write the failing tests**

Append to `config_admin_test.go` and add `"os"` and `"path/filepath"` to its import block (`strings` is already there):

```go
func TestPersistConfigChangesPreservesCommentsAndUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := `# top comment
server:
  proxy_api_key: "p" # inline
custom:
  keep_me: true
upstream:
  api_keys:
    - key: "sk-1"
      priority: 1
      weight: 1
  routing_strategy: "session_sticky"
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	cfg.ConfigSourcePath = path
	cfg.RoutingStrategy = "balanced"
	cfg.UpstreamKeyConfigs = []UpstreamKeyConfig{
		{Key: "sk-1", Priority: 1, Weight: 1},
		{Key: "sk-2", Priority: 2, Weight: 1},
	}
	cfg.UpstreamAPIKeys = []string{"sk-1", "sk-2"}

	previous, err := persistConfigChanges(path, cfg, []string{"routing_strategy", "keys"})
	if err != nil {
		t.Fatal(err)
	}
	if previous == nil {
		t.Fatal("expected previous file contents")
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# top comment", "keep_me", "balanced", "sk-2"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}

	loaded, err := loadYAMLConfig(path)
	if err != nil {
		t.Fatalf("reload: %v\n%s", err, out)
	}
	if loaded.RoutingStrategy != "balanced" {
		t.Fatalf("strategy = %q", loaded.RoutingStrategy)
	}
	if len(loaded.UpstreamKeyConfigs) != 2 || loaded.UpstreamKeyConfigs[1].Key != "sk-2" {
		t.Fatalf("keys = %+v", loaded.UpstreamKeyConfigs)
	}
}

func TestPersistConfigChangesCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	cfg := testConfig()
	cfg.RoutingStrategy = "fill_first"
	if _, err := persistConfigChanges(path, cfg, []string{"routing_strategy"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	loaded, err := loadYAMLConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RoutingStrategy != "fill_first" {
		t.Fatalf("strategy = %q", loaded.RoutingStrategy)
	}
}

func TestPersistConfigChangesRemovesEmptyAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := "models:\n  aliases:\n    a: b\nupstream:\n  api_keys: [\"sk-1\"]\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.ModelAliases = map[string]string{}
	if _, err := persistConfigChanges(path, cfg, []string{"model_aliases"}); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "aliases") {
		t.Fatalf("aliases not removed:\n%s", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./ -run 'TestPersistConfigChanges' -v`
Expected: FAIL to build with `undefined: persistConfigChanges`.

- [ ] **Step 3: Write the implementation**

Append to `config_admin.go`. Add `"path/filepath"` (already imported in Task 1) and `yaml "gopkg.in/yaml.v3"` to the import block:

```go
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mappingSet replaces the value in place when the key exists, so surrounding
// comments and key order survive, or appends a new pair.
func mappingSet(m *yaml.Node, key string, value *yaml.Node) {
	if v := mappingValue(m, key); v != nil {
		value.HeadComment = v.HeadComment
		value.LineComment = v.LineComment
		value.FootComment = v.FootComment
		*v = *value
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

func mappingDelete(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

func ensureMapping(m *yaml.Node, key string) (*yaml.Node, error) {
	if v := mappingValue(m, key); v != nil {
		if v.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s must be a mapping in the config file", key)
		}
		return v, nil
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mappingSet(m, key, child)
	return child, nil
}

func rootMapping(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind == 0 || len(doc.Content) == 0 {
		root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{root}
		return root, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config file root must be a mapping")
	}
	return root, nil
}

func encodeYAMLValue(v any) (*yaml.Node, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	return &n, nil
}

func yamlSettingValue(cfg Config, name string) any {
	switch name {
	case "routing_strategy":
		return defaultString(cfg.RoutingStrategy, "session_sticky")
	case "session_ttl":
		return cfg.SessionTTL.String()
	case "balanced_idle_timeout":
		return cfg.BalancedIdleTimeout.String()
	case "proactive_switch_threshold":
		return cfg.ProactiveSwitchThreshold
	case "retry_exhausted_after":
		return cfg.RetryExhaustedAfter.String()
	case "usage_check_interval":
		return cfg.UsageCheckInterval.String()
	case "disable_usage_polling":
		return cfg.DisableUsagePolling
	case "sanitize_developer_role":
		return cfg.SanitizeDeveloperRole
	}
	return nil
}

func applyYAMLUpdates(doc *yaml.Node, cfg Config, changed []string) error {
	root, err := rootMapping(doc)
	if err != nil {
		return err
	}
	for _, name := range changed {
		switch name {
		case "routing_strategy", "session_ttl", "balanced_idle_timeout",
			"proactive_switch_threshold", "retry_exhausted_after",
			"usage_check_interval", "disable_usage_polling":
			upstream, err := ensureMapping(root, "upstream")
			if err != nil {
				return err
			}
			value, err := encodeYAMLValue(yamlSettingValue(cfg, name))
			if err != nil {
				return err
			}
			mappingSet(upstream, name, value)
		case "sanitize_developer_role":
			transforms, err := ensureMapping(root, "transformations")
			if err != nil {
				return err
			}
			value, err := encodeYAMLValue(cfg.SanitizeDeveloperRole)
			if err != nil {
				return err
			}
			mappingSet(transforms, name, value)
		case "model_aliases":
			models, err := ensureMapping(root, "models")
			if err != nil {
				return err
			}
			if len(cfg.ModelAliases) == 0 {
				mappingDelete(models, "aliases")
				if len(models.Content) == 0 {
					mappingDelete(root, "models")
				}
				continue
			}
			aliases := make(map[string]string, len(cfg.ModelAliases))
			for k, v := range cfg.ModelAliases {
				aliases[k] = v
			}
			value, err := encodeYAMLValue(aliases)
			if err != nil {
				return err
			}
			mappingSet(models, "aliases", value)
		case "keys":
			upstream, err := ensureMapping(root, "upstream")
			if err != nil {
				return err
			}
			type keyEntry struct {
				Key      string `yaml:"key"`
				Priority int    `yaml:"priority"`
				Weight   int    `yaml:"weight"`
			}
			configs := effectiveKeyConfigs(cfg)
			list := make([]keyEntry, len(configs))
			for i, kc := range configs {
				kc = normalizedKey(kc)
				list[i] = keyEntry{Key: kc.Key, Priority: kc.Priority, Weight: kc.Weight}
			}
			value, err := encodeYAMLValue(list)
			if err != nil {
				return err
			}
			mappingSet(upstream, "api_keys", value)
		}
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// persistConfigChanges rewrites the config file with the changed editable
// values. It returns the previous file contents for rollback, or nil when the
// file did not exist.
func persistConfigChanges(path string, cfg Config, changed []string) ([]byte, error) {
	if len(changed) == 0 {
		return nil, nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create config directory: %w", err)
		}
	}
	mode := os.FileMode(0o600)
	var previous []byte
	var doc yaml.Node
	if existing, err := os.ReadFile(path); err == nil {
		previous = existing
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode().Perm()
		}
		if len(existing) > 0 {
			if err := yaml.Unmarshal(existing, &doc); err != nil {
				return nil, fmt.Errorf("parse config file: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := applyYAMLUpdates(&doc, cfg, changed); err != nil {
		return nil, err
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}
	if err := atomicWriteFile(path, out, mode); err != nil {
		return nil, err
	}
	return previous, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./ -run 'TestPersistConfigChanges' -v`
Expected: PASS.

If `TestPersistConfigChangesPreservesCommentsAndUnknownKeys` fails because comments were dropped, marshal the root mapping node instead of the document node (`yaml.Marshal(doc.Content[0])`) and re-run. Do not delete the assertion.

- [ ] **Step 5: Run gofmt and the full suite**

Run: `gofmt -l config_admin.go config_admin_test.go`
Expected: no output.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add config_admin.go config_admin_test.go
git commit -s -m "feat(config): persist dashboard edits to the yaml config file"
```

---

### Task 4: Immutable runtime state and shared apply path

**Files:**
- Modify: `main.go`
- Modify: `workspace.go`
- Modify: `dashboard.go`
- Test: `config_admin_test.go`, `main_test.go`, `dashboard_test.go`

**Interfaces:**
- Consumes: `Config`, `UpstreamKeyConfig`.
- Produces:
  - `App.config atomic.Pointer[Config]`, `App.sender atomic.Pointer[AlertNotifier]`, `App.workspace atomic.Pointer[WorkspaceUsageClient]`
  - `func (a *App) cfg() *Config`
  - `func (a *App) notifier() *AlertNotifier`
  - `func (a *App) workspaceClient() *WorkspaceUsageClient`
  - `func (a *App) applyConfig(newCfg Config)`
  - `func (a *App) restartUsagePoller()`
  - `func (m *KeyManager) KeyPriority(i int) int`
  - App fields `usageMu sync.Mutex`, `usageBaseCtx context.Context`, `usageCancel context.CancelFunc`, `configApplyMu sync.Mutex`

- [ ] **Step 1: Write the failing tests**

Append to `config_admin_test.go` and add `"context"`, `"net/http"`, `"net/http/httptest"`, `"time"` to its import block:

```go
func TestApplyConfigUpdatesKeyManagerAndPoller(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{}}`))
	}))
	defer upstream.Close()

	cfg := defaultConfig()
	cfg.ProxyAPIKey = "p"
	cfg.UpstreamBaseURL = upstream.URL
	cfg.UpstreamAPIKeys = []string{"sk-1"}
	cfg.UpstreamKeyConfigs = []UpstreamKeyConfig{{Key: "sk-1", Priority: 1, Weight: 1}}
	cfg.DisableUsagePolling = true
	app := newApp(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.startUsagePoller(ctx)
	if app.usageCancel != nil {
		t.Fatal("poller should be off when disabled")
	}

	next := cfg
	next.RoutingStrategy = "balanced"
	next.DisableUsagePolling = false
	next.UsageCheckInterval = 50 * time.Millisecond
	next.UpstreamKeyConfigs = []UpstreamKeyConfig{
		{Key: "sk-1", Priority: 1, Weight: 1},
		{Key: "sk-2", Priority: 2, Weight: 1},
	}
	next.UpstreamAPIKeys = []string{"sk-1", "sk-2"}
	app.applyConfig(next)

	if app.cfg().RoutingStrategy != "balanced" {
		t.Fatal("config not published")
	}
	if app.keys.routingStrategy != "balanced" {
		t.Fatal("key manager not updated")
	}
	if len(app.keys.keys) != 2 {
		t.Fatalf("keys not updated: %v", app.keys.keys)
	}
	if app.usageCancel == nil {
		t.Fatal("poller not restarted")
	}
}

func TestKeyPriorityAccessor(t *testing.T) {
	km := NewKeyManagerWithKeyConfigs(
		[]UpstreamKeyConfig{{Key: "a", Priority: 3, Weight: 1}},
		time.Hour, "session_sticky", time.Hour, time.Hour, 95,
	)
	if got := km.KeyPriority(0); got != 3 {
		t.Fatalf("got %d", got)
	}
	if got := km.KeyPriority(99); got != 1 {
		t.Fatalf("got %d", got)
	}
}
```

Update `main_test.go` line around 269 in `TestAdminResetAllKeys`:

```go
	for i := range app.cfg().UpstreamAPIKeys {
		app.keys.MarkExhausted(i)
	}
```

Update `dashboard_test.go` lines 233-234 in `TestDashboardAutoKeyEmbedding`:

```go
			cfg := *app.cfg()
			cfg.ListenAddr = tc.listen
			cfg.DashboardAutoKey = tc.setting
			app.config.Store(&cfg)
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./ -run 'TestApplyConfigUpdatesKeyManagerAndPoller|TestKeyPriorityAccessor' -v`
Expected: FAIL to build with `app.usageCancel undefined` and `app.cfg undefined`.

- [ ] **Step 3: Change the App struct, accessors, and constructor**

In `main.go`, add `"sync/atomic"` to the import block. Replace the `App` struct with:

```go
type App struct {
	config    atomic.Pointer[Config]
	keys      *KeyManager
	client    *http.Client
	sender    atomic.Pointer[AlertNotifier]
	metrics   *MetricsRegistry
	workspace atomic.Pointer[WorkspaceUsageClient]

	usageMu      sync.Mutex
	usageBaseCtx context.Context
	usageCancel  context.CancelFunc

	configApplyMu sync.Mutex
}

func (a *App) cfg() *Config { return a.config.Load() }

func (a *App) notifier() *AlertNotifier {
	if n := a.sender.Load(); n != nil {
		return n
	}
	return NewAlertNotifier(SMTPConfig{}, AlertConfig{})
}

func (a *App) workspaceClient() *WorkspaceUsageClient {
	if w := a.workspace.Load(); w != nil {
		return w
	}
	return &WorkspaceUsageClient{}
}
```

In `newApp`, replace the `app := &App{...}` block and the trailing workspace assignment with:

```go
	app := &App{
		keys: km,
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 2 * time.Second,
			},
		},
		metrics: NewMetricsRegistry(),
	}
	app.config.Store(&cfg)
	app.sender.Store(NewAlertNotifier(cfg.SMTP, cfg.Alerts))
	app.workspace.Store(NewWorkspaceUsageClient(
		"",
		cfg.WorkspaceUsage.SessionCookie,
		cfg.WorkspaceUsage.WorkspaceIDs,
	))
	return app
```

- [ ] **Step 4: Replace every config read with a snapshot**

Run: `grep -n "a\.config" *.go`

Replace each read with `a.cfg()`. The exact call sites:

- `dashboard.go`: `dashboardAutoKeyEnabled` (2 reads), `serveDashboardIndex` (1), `handleDashboardMetricsJSON` (1)
- `main.go`: `authOK` (1), `handleAdmin` workspace encode (use `a.workspaceClient().Snapshot()`), `handleReload` response (3), `handleResetKey` (1), `handleResetAllKeys` (1), `handleReadyz` (1), `checkUpstreamReady` (1), `fetchUpstreamUsage` (1), `pollAllKeysUsage` (1), `handleValidateKeys` (3), `proxyV1` (4), `transformRequestBody` (3), `augmentModelsResponse` (1), `doUpstream` (1), `validateConfigAndPrint` (2), `startUsagePoller` (3, replaced wholesale in the next step)
- `workspace.go`: `startWorkspaceUsagePoller` (2 reads plus `a.workspace`)

Replace notifier uses with `a.notifier()`:

- `main.go` `proxyV1`: `a.sender.NotifySwitch(...)`, `a.sender.NotifyAllExhausted(...)`

Replace the workspace write loop in `workspace.go` `startWorkspaceUsagePoller` with:

```go
func (a *App) startWorkspaceUsagePoller(ctx context.Context) {
	ws := a.workspace.Load()
	interval := a.cfg().WorkspaceUsage.Interval
	if ws == nil || interval <= 0 {
		return
	}
	go ws.Refresh(ctx)
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ws.Refresh(ctx)
			}
		}
	}()
}
```

In `main.go` `proxyV1`, replace the unlocked priority read:

```go
		priority := a.keys.KeyPriority(idx)
```

- [ ] **Step 5: Add the shared apply path and restartable poller**

Add the `KeyPriority` accessor to `main.go` next to the other `KeyManager` methods:

```go
func (m *KeyManager) KeyPriority(i int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.priorities) {
		return 1
	}
	return m.priorities[i]
}
```

Replace `ReloadConfig` with the shared apply path:

```go
// applyConfig publishes newCfg to the running process.
func (a *App) applyConfig(newCfg Config) {
	prev := a.cfg()
	a.config.Store(&newCfg)
	a.keys.ReloadKeys(
		effectiveKeyConfigs(newCfg),
		newCfg.RetryExhaustedAfter,
		newCfg.RoutingStrategy,
		newCfg.SessionTTL,
		newCfg.BalancedIdleTimeout,
		newCfg.ProactiveSwitchThreshold,
	)
	if prev == nil || !reflect.DeepEqual(prev.SMTP, newCfg.SMTP) || !reflect.DeepEqual(prev.Alerts, newCfg.Alerts) {
		a.sender.Store(NewAlertNotifier(newCfg.SMTP, newCfg.Alerts))
	}
	if prev == nil || !reflect.DeepEqual(prev.WorkspaceUsage, newCfg.WorkspaceUsage) {
		a.workspace.Store(NewWorkspaceUsageClient(
			"",
			newCfg.WorkspaceUsage.SessionCookie,
			newCfg.WorkspaceUsage.WorkspaceIDs,
		))
	}
	if prev == nil || prev.UsageCheckInterval != newCfg.UsageCheckInterval || prev.DisableUsagePolling != newCfg.DisableUsagePolling {
		a.restartUsagePoller()
	}
}

func (a *App) ReloadConfig() error {
	a.configApplyMu.Lock()
	defer a.configApplyMu.Unlock()
	newCfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("reload config failed: %w", err)
	}
	a.applyConfig(newCfg)
	log.Printf("config reloaded: %s", safeConfigSummary(newCfg))
	return nil
}
```

Add `"reflect"` to the `main.go` import block.

Replace `startUsagePoller` with a restartable pair:

```go
func (a *App) startUsagePoller(ctx context.Context) {
	a.usageMu.Lock()
	a.usageBaseCtx = ctx
	a.usageMu.Unlock()
	a.restartUsagePoller()
}

func (a *App) restartUsagePoller() {
	a.usageMu.Lock()
	defer a.usageMu.Unlock()
	if a.usageCancel != nil {
		a.usageCancel()
		a.usageCancel = nil
	}
	cfg := a.cfg()
	if a.usageBaseCtx == nil || cfg.DisableUsagePolling || cfg.UsageCheckInterval <= 0 {
		return
	}
	pollCtx, cancel := context.WithCancel(a.usageBaseCtx)
	a.usageCancel = cancel
	go a.pollAllKeysUsage(pollCtx)
	ticker := time.NewTicker(cfg.UsageCheckInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				if a.keys.sessionMgr != nil {
					a.keys.sessionMgr.CleanupExpired()
				}
				a.pollAllKeysUsage(pollCtx)
			}
		}
	}()
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `gofmt -l *.go`
Expected: no output.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Run the race detector**

Run: `go test -race ./...`
Expected: PASS with no race reports.

- [ ] **Step 8: Commit**

```bash
git add main.go workspace.go dashboard.go main_test.go dashboard_test.go config_admin_test.go
git commit -s -m "refactor(app): publish runtime state through atomic pointers"
```

---

### Task 5: Admin config endpoints

**Files:**
- Modify: `config_admin.go`
- Modify: `main.go`
- Test: `config_admin_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1 through 4 plus `writeAPIError`, `writeJSON`, `apiStyleForRequest`, `io`, `log`.
- Produces:
  - `func (a *App) handleConfigGet(w http.ResponseWriter, r *http.Request)`
  - `func (a *App) handleConfigPatch(w http.ResponseWriter, r *http.Request)`
  - `var loadConfigAfterPersist = loadConfig`
  - routes `GET /admin/config` and `PATCH /admin/config`

- [ ] **Step 1: Write the failing tests**

Append to `config_admin_test.go` and add `"errors"` to its import block (`os` and `path/filepath` are already there from Task 3):

```go
const configTestYAML = `server:
  proxy_api_key: "p"
upstream:
  base_url: "https://example.invalid/v1"
  api_keys:
    - key: "sk-abcdefghijklmnop"
      priority: 1
      weight: 1
  routing_strategy: "session_sticky"
`

func newConfigTestApp(t *testing.T, contents string) (*App, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DisableUsagePolling = true
	return newApp(cfg), path
}

func serveConfig(t *testing.T, app *App, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/admin/config", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer p")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func TestAdminConfigGetMasksKeys(t *testing.T) {
	app, _ := newConfigTestApp(t, configTestYAML)
	rec := serveConfig(t, app, http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp configResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Keys) != 1 || resp.Keys[0].KeyHint == "sk-abcdefghijklmnop" {
		t.Fatalf("keys = %+v", resp.Keys)
	}
	if resp.Revision == "" || !resp.Editable {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestAdminConfigPatchAppliesAndPersists(t *testing.T) {
	app, path := newConfigTestApp(t, configTestYAML)
	rec := serveConfig(t, app, http.MethodPatch, `{"settings":{"routing_strategy":"balanced","proactive_switch_threshold":80}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	if app.cfg().RoutingStrategy != "balanced" {
		t.Fatal("config not applied")
	}
	if app.keys.routingStrategy != "balanced" {
		t.Fatal("key manager not updated")
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "balanced") {
		t.Fatalf("file not updated:\n%s", out)
	}
}

func TestAdminConfigPatchRejectsEnvLocked(t *testing.T) {
	app, _ := newConfigTestApp(t, configTestYAML)
	t.Setenv("ROUTING_STRATEGY", "balanced")
	rec := serveConfig(t, app, http.MethodPatch, `{"settings":{"routing_strategy":"round_robin"}}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminConfigPatchRejectsStaleRevision(t *testing.T) {
	app, _ := newConfigTestApp(t, configTestYAML)
	rec := serveConfig(t, app, http.MethodPatch, `{"if_revision":"deadbeef","settings":{"routing_strategy":"balanced"}}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminConfigPatchRejectsUnknownSetting(t *testing.T) {
	app, _ := newConfigTestApp(t, configTestYAML)
	rec := serveConfig(t, app, http.MethodPatch, `{"settings":{"nope":1}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminConfigPatchRollsBackWhenReloadFails(t *testing.T) {
	app, path := newConfigTestApp(t, configTestYAML)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original := loadConfigAfterPersist
	loadConfigAfterPersist = func() (Config, error) { return Config{}, errors.New("boom") }
	defer func() { loadConfigAfterPersist = original }()

	rec := serveConfig(t, app, http.MethodPatch, `{"settings":{"routing_strategy":"balanced"}}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("file not restored:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestAdminConfigPatchRejectsWhenNotEditable(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("SWITCHBOARD_GO_CONFIG", "")
	cfg := defaultConfig()
	cfg.ProxyAPIKey = "p"
	cfg.UpstreamAPIKeys = []string{"sk-1"}
	cfg.UpstreamKeyConfigs = []UpstreamKeyConfig{{Key: "sk-1", Priority: 1, Weight: 1}}
	app := newApp(cfg)
	rec := serveConfig(t, app, http.MethodPatch, `{"settings":{"routing_strategy":"balanced"}}`)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./ -run 'TestAdminConfig' -v`
Expected: FAIL with `--- FAIL: TestAdminConfigGetMasksKeys` returning 404 or a build error for `loadConfigAfterPersist`.

- [ ] **Step 3: Write the handlers**

Append to `config_admin.go`. Add `"io"`, `"log"` to its import block:

```go
var loadConfigAfterPersist = loadConfig

func (a *App) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg := a.cfg()
	editable := resolveWritableConfigPath(*cfg) != ""
	writeJSON(w, http.StatusOK, buildConfigResponse(*cfg, editable))
}

func (a *App) handleConfigPatch(w http.ResponseWriter, r *http.Request) {
	style := apiStyleForRequest(r)

	a.configApplyMu.Lock()
	defer a.configApplyMu.Unlock()

	current := *a.cfg()
	var req configPatchRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeAPIError(w, style, http.StatusBadRequest, "invalid_config", "invalid JSON body: "+err.Error())
		return
	}
	if locked := envLockViolation(req, envLockedFields()); locked != "" {
		writeAPIError(w, style, http.StatusConflict, "config_env_locked", locked+" is set by an environment variable and cannot be changed here")
		return
	}
	if req.IfRevision != nil && *req.IfRevision != configRevision(current) {
		writeAPIError(w, style, http.StatusConflict, "config_revision_conflict", "configuration changed since it was loaded; reload and try again")
		return
	}
	next, changed, err := applyConfigPatch(current, req)
	if err != nil {
		writeAPIError(w, style, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	if err := validateConfig(next); err != nil {
		writeAPIError(w, style, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	path := resolveWritableConfigPath(current)
	if path == "" {
		writeAPIError(w, style, http.StatusPreconditionFailed, "config_not_editable", "no writable config file; set SWITCHBOARD_GO_CONFIG or create a config file")
		return
	}
	previous, err := persistConfigChanges(path, next, changed)
	if err != nil {
		writeAPIError(w, style, http.StatusInternalServerError, "config_apply_failed", "persist config: "+err.Error())
		return
	}
	applied, err := loadConfigAfterPersist()
	if err != nil {
		if previous != nil {
			_ = atomicWriteFile(path, previous, 0o600)
		} else if len(changed) > 0 {
			_ = os.Remove(path)
		}
		writeAPIError(w, style, http.StatusInternalServerError, "config_apply_failed", "reload after write failed: "+err.Error())
		return
	}
	a.applyConfig(applied)
	log.Printf("config updated via API: fields=%s source=%s", strings.Join(changed, ","), defaultString(applied.ConfigSourcePath, "none"))
	writeJSON(w, http.StatusOK, buildConfigResponse(applied, true))
}
```

- [ ] **Step 4: Add the routes**

In `main.go` `handleAdmin`, add these cases before the `default:` case:

```go
	case r.URL.Path == "/admin/config" && r.Method == http.MethodGet:
		a.handleConfigGet(w, r)
	case r.URL.Path == "/admin/config" && r.Method == http.MethodPatch:
		a.handleConfigPatch(w, r)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./ -run 'TestAdminConfig' -v`
Expected: PASS.

- [ ] **Step 6: Run gofmt, the full suite, and the race detector**

Run: `gofmt -l *.go`
Expected: no output.

Run: `go test ./... && go test -race ./...`
Expected: PASS, no race reports.

- [ ] **Step 7: Commit**

```bash
git add config_admin.go main.go config_admin_test.go
git commit -s -m "feat(admin): add dashboard config get and patch endpoints"
```

---

### Task 6: Dashboard config types and API client

**Files:**
- Modify: `web/dashboard/src/types.ts`
- Modify: `web/dashboard/src/api.ts`

**Interfaces:**
- Consumes: existing `getJSON`, `authHeaders`, `normalizeBase`, `ApiError`.
- Produces:
  - `ProxyConfigSettings`, `ProxyConfigKey`, `ProxyConfigResponse`, `ProxyConfigKeyPatch`, `ProxyConfigPatch`
  - `fetchProxyConfig(base, apiKey): Promise<ProxyConfigResponse>`
  - `patchProxyConfig(base, apiKey, patch): Promise<ProxyConfigResponse>`

- [ ] **Step 1: Install dashboard dependencies**

Run from `web/dashboard`: `npm ci`
Expected: install completes with no errors.

- [ ] **Step 2: Add the types**

Append to `web/dashboard/src/types.ts`:

```ts
export interface ProxyConfigSettings {
  routing_strategy: string;
  session_ttl: string;
  balanced_idle_timeout: string;
  proactive_switch_threshold: number;
  retry_exhausted_after: string;
  usage_check_interval: string;
  disable_usage_polling: boolean;
  sanitize_developer_role: boolean;
}

export interface ProxyConfigKey {
  id: number;
  key_hint: string;
  priority: number;
  weight: number;
}

export interface ProxyConfigResponse {
  config_source: string;
  editable: boolean;
  revision: string;
  env_locked: string[];
  settings: ProxyConfigSettings;
  model_aliases: Record<string, string>;
  keys: ProxyConfigKey[];
}

export interface ProxyConfigKeyPatch {
  id?: number;
  key?: string;
  priority?: number;
  weight?: number;
}

export interface ProxyConfigPatch {
  if_revision: string;
  settings?: Record<string, string | number | boolean | null>;
  model_aliases?: Record<string, string | null>;
  keys?: ProxyConfigKeyPatch[];
}
```

- [ ] **Step 3: Add the API functions**

Append to `web/dashboard/src/api.ts` and add `ProxyConfigPatch`, `ProxyConfigResponse` to its type import list:

```ts
export async function fetchProxyConfig(
  base: string,
  apiKey: string,
): Promise<ProxyConfigResponse> {
  return getJSON<ProxyConfigResponse>(base, '/admin/config', apiKey);
}

export async function patchProxyConfig(
  base: string,
  apiKey: string,
  patch: ProxyConfigPatch,
): Promise<ProxyConfigResponse> {
  const res = await fetch(`${normalizeBase(base)}/admin/config`, {
    method: 'PATCH',
    headers: { ...authHeaders(apiKey), 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
  if (res.status === 401) {
    throw new ApiError(401, 'Invalid or missing proxy API key');
  }
  if (!res.ok) {
    let message = `Request failed with status ${res.status}`;
    try {
      const body = (await res.json()) as { error?: { message?: string } };
      if (body.error?.message) {
        message = body.error.message;
      }
    } catch {
      // Keep the default message when the body is not JSON.
    }
    throw new ApiError(res.status, message);
  }
  return (await res.json()) as ProxyConfigResponse;
}
```

- [ ] **Step 4: Verify the build**

Run from `web/dashboard`: `npm run build`
Expected: PASS (`tsc --noEmit` plus vite build).

- [ ] **Step 5: Commit**

```bash
git add web/dashboard/src/types.ts web/dashboard/src/api.ts
git commit -s -m "feat(dashboard): add proxy config api client"
```

---

### Task 7: Proxy config dialog component

**Files:**
- Create: `web/dashboard/src/components/ProxyConfig.ts`
- Modify: `web/dashboard/src/style.css`

**Interfaces:**
- Consumes: `ProxyConfigResponse`, `ProxyConfigKey`, `ProxyConfigKeyPatch`, `ProxyConfigPatch`, `ProxyConfigSettings`, `esc`.
- Produces:
  - `renderProxyConfigForm(cfg: ProxyConfigResponse): string`
  - `collectProxyConfigPatch(baseline: ProxyConfigResponse, root: HTMLElement): { patch?: ProxyConfigPatch; error?: string; changed: boolean }`
  - `renderNewKeyRow(): string`
  - `renderNewAliasRow(): string`

- [ ] **Step 1: Write the component**

Create `web/dashboard/src/components/ProxyConfig.ts`:

```ts
import type {
  ProxyConfigKeyPatch,
  ProxyConfigPatch,
  ProxyConfigResponse,
  ProxyConfigSettings,
} from '../types';
import { esc } from '../utils';

const DURATION_RE = /^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)$/;

const DURATION_FIELDS: Array<{ name: keyof ProxyConfigSettings; id: string; label: string }> = [
  { name: 'session_ttl', id: 'cfg-session-ttl', label: 'Session TTL' },
  { name: 'balanced_idle_timeout', id: 'cfg-balanced-idle-timeout', label: 'Balanced idle timeout' },
  { name: 'retry_exhausted_after', id: 'cfg-retry-exhausted-after', label: 'Retry exhausted after' },
];

const STRATEGIES = ['session_sticky', 'balanced', 'round_robin', 'fill_first'];

function envBadge(locked: Set<string>, name: string): string {
  return locked.has(name)
    ? '<span class="env-badge" title="Overridden by an environment variable">env</span>'
    : '';
}

function disabled(locked: Set<string>, name: string): string {
  return locked.has(name) ? ' disabled' : '';
}

function keyRow(row: { key_hint?: string; id?: number; priority?: number; weight?: number } | null): string {
  const isNew = row === null;
  const id = isNew ? '' : String(row.id);
  const hint = isNew ? 'new key' : row.key_hint || '…';
  const secret = isNew
    ? '<input type="password" class="input" data-key-value placeholder="sk-…" autocomplete="new-password" />'
    : '<input type="password" class="input" data-key-rotate placeholder="rotate (optional)" autocomplete="new-password" />';
  return `
    <div class="cfg-key-row" data-key-row>
      <input type="hidden" data-key-id value="${esc(id)}" />
      <span class="num cfg-key-hint">${esc(hint)}</span>
      <label class="num cfg-inline">pri
        <input type="number" class="input" min="1" data-key-priority value="${row?.priority ?? 1}" />
      </label>
      <label class="num cfg-inline">w
        <input type="number" class="input" min="1" data-key-weight value="${row?.weight ?? 1}" />
      </label>
      ${secret}
      <button type="button" class="btn btn-secondary btn-xs" data-remove-key>Remove</button>
    </div>
  `;
}

function aliasRow(from: string, to: string, original: string): string {
  return `
    <div class="cfg-alias-row" data-alias-row data-alias-original="${esc(original)}">
      <input type="text" class="input" data-alias-from placeholder="alias" value="${esc(from)}" />
      <input type="text" class="input" data-alias-to placeholder="target model" value="${esc(to)}" />
      <button type="button" class="btn btn-secondary btn-xs" data-remove-alias>Remove</button>
    </div>
  `;
}

export function renderNewKeyRow(): string {
  return keyRow(null);
}

export function renderNewAliasRow(): string {
  return aliasRow('', '', '');
}

export function renderProxyConfigForm(cfg: ProxyConfigResponse): string {
  const locked = new Set(cfg.env_locked);
  const s = cfg.settings;
  const strategyOptions = STRATEGIES.map(
    (v) => `<option value="${v}"${s.routing_strategy === v ? ' selected' : ''}>${v}</option>`,
  ).join('');

  const durationFields = DURATION_FIELDS.map(
    ({ name, id, label }) => `
      <div class="field">
        <label for="${id}">${label}${envBadge(locked, name)}</label>
        <input type="text" id="${id}" class="input" value="${esc(String(s[name]))}"${disabled(locked, name)} />
      </div>`,
  ).join('');

  const keyRows = locked.has('keys')
    ? '<p class="text-muted cfg-note">Keys are managed by OPENCODE_GO_API_KEYS.</p>'
    : cfg.keys.map((k) => keyRow(k)).join('');

  const aliasRows = locked.has('model_aliases')
    ? ''
    : Object.entries(cfg.model_aliases)
        .map(([from, to]) => aliasRow(from, to, from))
        .join('');

  return `
    <div class="cfg-section">
      <div class="kicker">Routing</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-routing-strategy">Strategy${envBadge(locked, 'routing_strategy')}</label>
          <select id="cfg-routing-strategy" class="input"${disabled(locked, 'routing_strategy')}>${strategyOptions}</select>
        </div>
        <div class="field">
          <label for="cfg-proactive-threshold">Proactive threshold %${envBadge(locked, 'proactive_switch_threshold')}</label>
          <input type="number" id="cfg-proactive-threshold" class="input" min="0" max="100" step="0.1" value="${s.proactive_switch_threshold}"${disabled(locked, 'proactive_switch_threshold')} />
        </div>
        ${durationFields}
      </div>
    </div>
    <div class="cfg-section">
      <div class="kicker">Polling</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-usage-check-interval">Usage check interval${envBadge(locked, 'usage_check_interval')}</label>
          <input type="text" id="cfg-usage-check-interval" class="input" value="${esc(s.usage_check_interval)}"${disabled(locked, 'usage_check_interval')} />
        </div>
        <div class="field">
          <label for="cfg-disable-polling">Disable usage polling${envBadge(locked, 'disable_usage_polling')}</label>
          <input type="checkbox" id="cfg-disable-polling"${s.disable_usage_polling ? ' checked' : ''}${disabled(locked, 'disable_usage_polling')} />
        </div>
      </div>
    </div>
    <div class="cfg-section">
      <div class="kicker">Models</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-sanitize-role">Sanitize developer role${envBadge(locked, 'sanitize_developer_role')}</label>
          <input type="checkbox" id="cfg-sanitize-role"${s.sanitize_developer_role ? ' checked' : ''}${disabled(locked, 'sanitize_developer_role')} />
        </div>
      </div>
      <div class="kicker" style="margin-top:var(--space-4)">Model aliases${envBadge(locked, 'model_aliases')}</div>
      <div data-alias-list>${aliasRows}</div>
      <button type="button" class="btn btn-secondary" data-add-alias${locked.has('model_aliases') ? ' disabled' : ''}>Add alias</button>
    </div>
    <div class="cfg-section">
      <div class="kicker">Upstream keys${envBadge(locked, 'keys')}</div>
      <div data-key-list>${keyRows}</div>
      <button type="button" class="btn btn-secondary" data-add-key${locked.has('keys') ? ' disabled' : ''}>Add key</button>
      <p class="text-muted cfg-note">Existing key values never leave the server. Paste a new value to rotate a key.</p>
    </div>
  `;
}

function readInput(root: HTMLElement, selector: string): HTMLInputElement | null {
  return root.querySelector<HTMLInputElement>(selector);
}

export function collectProxyConfigPatch(
  baseline: ProxyConfigResponse,
  root: HTMLElement,
): { patch?: ProxyConfigPatch; error?: string; changed: boolean } {
  const locked = new Set(baseline.env_locked);
  const b = baseline.settings;
  const patch: ProxyConfigPatch = { if_revision: baseline.revision };
  const settings: Record<string, string | number | boolean | null> = {};

  const strategy = root.querySelector<HTMLSelectElement>('#cfg-routing-strategy');
  if (strategy && !locked.has('routing_strategy') && strategy.value !== b.routing_strategy) {
    settings.routing_strategy = strategy.value;
  }

  const threshold = readInput(root, '#cfg-proactive-threshold');
  if (threshold && !locked.has('proactive_switch_threshold')) {
    const value = Number(threshold.value);
    if (!Number.isFinite(value) || value < 0 || value > 100) {
      return { error: 'proactive_switch_threshold must be between 0 and 100', changed: false };
    }
    if (value !== b.proactive_switch_threshold) {
      settings.proactive_switch_threshold = value;
    }
  }

  for (const { name, id } of DURATION_FIELDS) {
    if (locked.has(name)) continue;
    const input = readInput(root, `#${id}`);
    if (!input) continue;
    const value = input.value.trim();
    if (!DURATION_RE.test(value)) {
      return { error: `${name} must be a duration like 30s, 5m, or 2h`, changed: false };
    }
    if (value !== b[name]) {
      settings[name] = value;
    }
  }

  const usageInterval = readInput(root, '#cfg-usage-check-interval');
  if (usageInterval && !locked.has('usage_check_interval')) {
    const value = usageInterval.value.trim();
    if (!DURATION_RE.test(value)) {
      return { error: 'usage_check_interval must be a duration like 30s, 5m, or 2h', changed: false };
    }
    if (value !== b.usage_check_interval) {
      settings.usage_check_interval = value;
    }
  }

  const disablePolling = readInput(root, '#cfg-disable-polling');
  if (disablePolling && !locked.has('disable_usage_polling') && disablePolling.checked !== b.disable_usage_polling) {
    settings.disable_usage_polling = disablePolling.checked;
  }

  const sanitize = readInput(root, '#cfg-sanitize-role');
  if (sanitize && !locked.has('sanitize_developer_role') && sanitize.checked !== b.sanitize_developer_role) {
    settings.sanitize_developer_role = sanitize.checked;
  }

  if (Object.keys(settings).length > 0) {
    patch.settings = settings;
  }

  if (!locked.has('model_aliases')) {
    const aliases: Record<string, string | null> = {};
    const seen = new Set<string>();
    for (const row of root.querySelectorAll<HTMLElement>('[data-alias-row]')) {
      const from = row.querySelector<HTMLInputElement>('[data-alias-from]')?.value.trim() ?? '';
      const to = row.querySelector<HTMLInputElement>('[data-alias-to]')?.value.trim() ?? '';
      const original = row.dataset.aliasOriginal ?? '';
      if (!from) {
        if (original) aliases[original] = null;
        continue;
      }
      if (seen.has(from)) {
        return { error: `duplicate alias ${from}`, changed: false };
      }
      seen.add(from);
      if (original && original !== from) {
        aliases[original] = null;
      }
      if (!to) {
        if (baseline.model_aliases[from] !== undefined) {
          aliases[from] = null;
        }
        continue;
      }
      if (baseline.model_aliases[from] !== to) {
        aliases[from] = to;
      }
    }
    if (Object.keys(aliases).length > 0) {
      patch.model_aliases = aliases;
    }
  }

  if (!locked.has('keys')) {
    const keys: ProxyConfigKeyPatch[] = [];
    for (const row of root.querySelectorAll<HTMLElement>('[data-key-row]')) {
      const idRaw = row.querySelector<HTMLInputElement>('[data-key-id]')?.value ?? '';
      const priority = Number(row.querySelector<HTMLInputElement>('[data-key-priority]')?.value);
      const weight = Number(row.querySelector<HTMLInputElement>('[data-key-weight]')?.value);
      if (!Number.isInteger(priority) || priority < 1) {
        return { error: 'key priority must be >= 1', changed: false };
      }
      if (!Number.isInteger(weight) || weight < 1) {
        return { error: 'key weight must be >= 1', changed: false };
      }
      if (idRaw === '') {
        const value = row.querySelector<HTMLInputElement>('[data-key-value]')?.value.trim() ?? '';
        if (!value) {
          return { error: 'new keys need a value', changed: false };
        }
        keys.push({ key: value, priority, weight });
      } else {
        const entry: ProxyConfigKeyPatch = { id: Number(idRaw), priority, weight };
        const rotate = row.querySelector<HTMLInputElement>('[data-key-rotate]')?.value.trim();
        if (rotate) {
          entry.key = rotate;
        }
        keys.push(entry);
      }
    }
    if (keys.length === 0) {
      return { error: 'at least one upstream key is required', changed: false };
    }
    patch.keys = keys;
  }

  const changed =
    patch.settings !== undefined || patch.model_aliases !== undefined || patch.keys !== undefined;
  if (!changed) {
    return { changed: false };
  }
  return { patch, changed: true };
}
```

- [ ] **Step 2: Add the styles**

Append to `web/dashboard/src/style.css`:

```css
/* Proxy configuration dialog */
.dialog-body-scroll {
  max-height: 60vh;
  overflow-y: auto;
  padding-right: var(--space-2);
  margin-bottom: var(--space-4);
}

.cfg-section {
  margin-bottom: var(--space-6);
}

.cfg-section + .cfg-section {
  border-top: 1px solid rgba(243, 242, 242, 0.08);
  padding-top: var(--space-4);
}

.cfg-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-4);
  margin-top: var(--space-3);
}

.cfg-key-row {
  display: grid;
  grid-template-columns: 120px 84px 84px minmax(140px, 1fr) auto;
  gap: var(--space-3);
  align-items: center;
  padding: var(--space-2) 0;
  border-bottom: 1px solid rgba(243, 242, 242, 0.08);
}

.cfg-key-hint {
  font-size: 12px;
  color: var(--color-neutral-400);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cfg-inline {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  font-size: 11px;
  color: var(--color-neutral-600);
}

.cfg-inline .input {
  width: 56px;
}

.cfg-alias-row {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: var(--space-3);
  align-items: center;
  margin-bottom: var(--space-2);
}

.cfg-note {
  font-size: 11.5px;
  margin-top: var(--space-2);
}

.btn-xs {
  padding: 4px 10px;
  font-size: 11px;
}

.env-badge {
  font-size: 9.5px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  border: 1px solid var(--color-neutral-600);
  color: var(--color-neutral-600);
  padding: 0 4px;
  border-radius: 2px;
  margin-left: 6px;
}

@media (max-width: 760px) {
  .cfg-grid {
    grid-template-columns: 1fr;
  }

  .cfg-key-row {
    grid-template-columns: 1fr 1fr;
    row-gap: var(--space-2);
  }
}
```

- [ ] **Step 3: Verify the build**

Run from `web/dashboard`: `npm run build`
Expected: PASS. `ProxyConfig.ts` is not imported yet, but `tsc` still type checks it because `include` covers `src`.

- [ ] **Step 4: Commit**

```bash
git add web/dashboard/src/components/ProxyConfig.ts web/dashboard/src/style.css
git commit -s -m "feat(dashboard): add proxy config form component"
```

---

### Task 8: Wire the dialog into the dashboard

**Files:**
- Modify: `web/dashboard/index.html`
- Modify: `web/dashboard/src/main.ts`

**Interfaces:**
- Consumes: everything from Tasks 6 and 7.
- Produces: a `Proxy configuration` menu action and a working save flow.

- [ ] **Step 1: Add the menu item and dialog markup**

In `web/dashboard/index.html`, add this line to `#dropdown-menu` after the `reload` item:

```html
                <div class="menuitem" data-action="proxy-config">Proxy configuration</div>
```

Add this dialog after the `settings-dialog` block:

```html
    <!-- Proxy Configuration Dialog -->
    <dialog id="proxy-config-dialog" class="dialog-modal">
      <div class="dialog-backdrop" id="proxy-config-backdrop">
        <div class="dialog dialog-lg" role="dialog" aria-labelledby="proxy-config-title">
          <div class="dialog-header">
            <h2 id="proxy-config-title" class="dialog-title">Proxy Configuration</h2>
            <button type="button" id="proxy-config-close-btn" class="iconbtn sm" aria-label="Close">&times;</button>
          </div>
          <form id="proxy-config-form">
            <div id="proxy-config-body" class="dialog-body-scroll"></div>
            <p class="dialog-body">
              Writes to <span id="proxy-config-source" class="num">—</span>. Values set by environment variables are read-only.
            </p>
            <div class="dialog-actions">
              <button type="button" id="proxy-config-refresh-btn" class="btn btn-secondary">Reload from disk</button>
              <button type="submit" id="proxy-config-save" class="btn btn-primary">Apply changes</button>
            </div>
          </form>
        </div>
      </div>
    </dialog>
```

- [ ] **Step 2: Add the state and functions to main.ts**

Add to the imports:

```ts
import { patchProxyConfig, fetchProxyConfig } from './api';
import {
  collectProxyConfigPatch,
  renderNewAliasRow,
  renderNewKeyRow,
  renderProxyConfigForm,
} from './components/ProxyConfig';
import type { ProxyConfigResponse } from './types';
```

Add the state next to `lastWorkspace`:

```ts
let proxyConfig: ProxyConfigResponse | null = null;
```

Add these functions near `openSettingsDialog`:

```ts
async function loadProxyConfigDialog(): Promise<void> {
  const body = $('#proxy-config-body');
  body.innerHTML = '<p class="text-muted">Loading proxy configuration…</p>';
  try {
    proxyConfig = await fetchProxyConfig(settings.baseUrl, settings.apiKey);
    body.innerHTML = renderProxyConfigForm(proxyConfig);
    $('#proxy-config-source').textContent =
      proxyConfig.config_source === 'none' ? 'no config file' : proxyConfig.config_source;
    $<HTMLButtonElement>('#proxy-config-save').disabled = !proxyConfig.editable;
    if (!proxyConfig.editable) {
      for (const el of body.querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLButtonElement>('input, select, button')) {
        el.disabled = true;
      }
      body.insertAdjacentHTML(
        'afterbegin',
        '<p class="text-muted cfg-note">No writable config file found. Set SWITCHBOARD_GO_CONFIG or create a config file, then reload.</p>',
      );
    }
  } catch (err) {
    body.innerHTML = `<p class="text-muted">Could not load proxy configuration: ${esc(String(err))}</p>`;
    $<HTMLButtonElement>('#proxy-config-save').disabled = true;
  }
}

function openProxyConfigDialog(): void {
  const dlg = $<HTMLDialogElement>('#proxy-config-dialog');
  if (!dlg.open) {
    if (typeof dlg.showModal === 'function') {
      dlg.showModal();
    } else {
      dlg.setAttribute('open', '');
    }
  }
  void loadProxyConfigDialog();
}

function closeProxyConfigDialog(): void {
  const dlg = $<HTMLDialogElement>('#proxy-config-dialog');
  if (typeof dlg.close === 'function') {
    dlg.close();
  } else {
    dlg.removeAttribute('open');
  }
}

async function saveProxyConfig(): Promise<void> {
  if (!proxyConfig) return;
  const root = $('#proxy-config-body');
  const result = collectProxyConfigPatch(proxyConfig, root);
  if (result.error) {
    banner(result.error);
    return;
  }
  if (!result.changed || !result.patch) {
    toast('No configuration changes to apply');
    return;
  }
  try {
    const updated = await patchProxyConfig(settings.baseUrl, settings.apiKey, result.patch);
    proxyConfig = updated;
    root.innerHTML = renderProxyConfigForm(updated);
    $('#proxy-config-source').textContent =
      updated.config_source === 'none' ? 'no config file' : updated.config_source;
    banner(null);
    toast('Proxy configuration applied');
    await poll(false);
  } catch (err) {
    if (err instanceof ApiError && err.status === 409) {
      toast('Configuration changed elsewhere — reloading');
      await loadProxyConfigDialog();
      return;
    }
    if (err instanceof ApiError) {
      banner(`Configuration update failed: ${err.message}`);
    } else {
      banner(`Configuration update failed: ${String(err)}`);
    }
  }
}
```

- [ ] **Step 3: Add the menu action**

In `handleAction`, add this case before `case 'settings':`:

```ts
      case 'proxy-config': {
        openProxyConfigDialog();
        return;
      }
```

- [ ] **Step 4: Bind the dialog events**

In `bindEvents`, after the settings dialog handlers, add:

```ts
  $('#proxy-config-form').addEventListener('submit', (e) => {
    e.preventDefault();
    void saveProxyConfig();
  });

  $('#proxy-config-close-btn').addEventListener('click', closeProxyConfigDialog);
  $('#proxy-config-refresh-btn').addEventListener('click', () => {
    void loadProxyConfigDialog();
  });
  $('#proxy-config-backdrop').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) {
      closeProxyConfigDialog();
    }
  });
```

In the delegated `document.addEventListener('click', ...)` handler, before the topbar menu dismissal block, add:

```ts
    if (target.closest('[data-add-key]')) {
      $('#proxy-config-body [data-key-list]')?.insertAdjacentHTML('beforeend', renderNewKeyRow());
      return;
    }

    if (target.closest('[data-remove-key]')) {
      target.closest('[data-key-row]')?.remove();
      return;
    }

    if (target.closest('[data-add-alias]')) {
      $('#proxy-config-body [data-alias-list]')?.insertAdjacentHTML('beforeend', renderNewAliasRow());
      return;
    }

    if (target.closest('[data-remove-alias]')) {
      target.closest('[data-alias-row]')?.remove();
      return;
    }
```

- [ ] **Step 5: Verify the build and copy the built assets**

Run from `web/dashboard`: `npm run build`
Expected: PASS and `dist/` is regenerated.

- [ ] **Step 6: Commit**

```bash
git add web/dashboard/index.html web/dashboard/src/main.ts web/dashboard/dist
git commit -s -m "feat(dashboard): add proxy configuration dialog"
```

---

### Task 9: Documentation

**Files:**
- Modify: `docs/admin-api.md`
- Modify: `docs/configuration.md`

**Interfaces:**
- Consumes: the endpoints from Task 5.
- Produces: operator documentation for dashboard editing.

- [ ] **Step 1: Document the endpoints**

In `docs/admin-api.md`, add this section before `## Reset key manually`:

````markdown
## Configuration

`GET /admin/config` returns the editable settings, model aliases, and masked
upstream keys. `PATCH /admin/config` merges changes into the running config,
writes them to the config file, and applies them without a restart. Both
endpoints require the proxy API key.

```json
{
  "config_source": "/home/user/.config/switchboard-go/config.yaml",
  "editable": true,
  "revision": "6d1f…",
  "env_locked": ["routing_strategy"],
  "settings": {
    "routing_strategy": "session_sticky",
    "session_ttl": "2h",
    "balanced_idle_timeout": "1h",
    "proactive_switch_threshold": 95,
    "retry_exhausted_after": "5m",
    "usage_check_interval": "30s",
    "disable_usage_polling": false,
    "sanitize_developer_role": true
  },
  "model_aliases": { "gpt-4o": "glm-5.1" },
  "keys": [
    { "id": 0, "key_hint": "sk-abcd…1234", "priority": 1, "weight": 3 }
  ]
}
```

PATCH accepts only the fields being changed:

```json
{
  "if_revision": "6d1f…",
  "settings": { "routing_strategy": "balanced", "session_ttl": null },
  "model_aliases": { "gpt-4o": null, "claude-sonnet": "glm-5.1" },
  "keys": [
    { "id": 0, "priority": 2 },
    { "id": 1, "key": "sk-rotated-value" },
    { "key": "sk-new-key", "priority": 1, "weight": 2 }
  ]
}
```

- A `null` in `settings` resets that field to its default. A `null` in
  `model_aliases` deletes the alias.
- `keys`, when present, is the complete desired list. An entry with `id` edits
  that key. Omit `key` to keep the secret, or include it to rotate. An entry
  without `id` adds a key. Existing ids you leave out are removed.
- `if_revision` is the `revision` from GET. A stale value returns 409.
- Settings pinned by an env var (`env_locked`) return 409 when changed.
- Status codes: 400 invalid values, 409 stale revision or env lock, 412 no
  writable config file, 500 write or apply failure.

Values are written to `config_source`, preserving comments and keys the
dashboard does not manage. Environment variables still win over the file at
the next load.
````

- [ ] **Step 2: Document dashboard editing**

In `docs/configuration.md`, add this section after the "Configuration
precedence" list:

````markdown
## Editing configuration from the dashboard

The dashboard's Proxy configuration dialog edits the core proxy settings and
upstream key list at runtime. It writes to the config file the process loaded,
or to `~/.config/switchboard-go/config.yaml` when the process started from
environment variables only.

- Writes preserve comments, key order, and keys the dashboard does not manage.
- New files get `0600` permissions; new directories get `0700`.
- Values pinned by environment variables show as read-only, because env
  overrides the file again at the next load.
- Upstream key values never leave the server. The dashboard shows masked hints
  and lets you add, remove, rotate, and re-prioritize keys.
- Editable fields: `routing_strategy`, `session_ttl`,
  `balanced_idle_timeout`, `proactive_switch_threshold`,
  `retry_exhausted_after`, `usage_check_interval`,
  `disable_usage_polling`, `sanitize_developer_role`, `models.aliases`, and
  `upstream.api_keys`.
- Not editable: listen address, proxy API key, upstream base URL, request body
  limit, alerts, SMTP, and workspace usage settings.
````

- [ ] **Step 3: Commit**

```bash
git add docs/admin-api.md docs/configuration.md
git commit -s -m "docs: describe dashboard runtime configuration"
```

---

### Task 10: End-to-end verification

**Files:**
- No new files.

**Interfaces:**
- Consumes: every previous task.
- Produces: a verified branch ready for review.

- [ ] **Step 1: Run the Go quality gates**

Run: `gofmt -l *.go`
Expected: no output.

Run: `go vet ./...`
Expected: no output.

Run: `go test ./... && go test -race ./...`
Expected: PASS, no race reports.

- [ ] **Step 2: Run the dashboard build**

Run from `web/dashboard`: `npm run build`
Expected: PASS.

- [ ] **Step 3: Exercise the flow in a browser**

Create a temp config and start the server:

```bash
mkdir -p /tmp/swb-e2e
cat > /tmp/swb-e2e/config.yaml <<'EOF'
server:
  listen_addr: "127.0.0.1:8495"
  proxy_api_key: "e2e-key"
upstream:
  base_url: "https://opencode.ai/zen/go/v1"
  api_keys:
    - key: "sk-e2e-abcdefghijklmnop"
      priority: 1
      weight: 1
  routing_strategy: "session_sticky"
  disable_usage_polling: true
EOF
SWITCHBOARD_GO_CONFIG=/tmp/swb-e2e/config.yaml go run . &
```

Then use the browser tool at `http://127.0.0.1:8495/dashboard/`:

1. Open the actions menu, choose Proxy configuration. Confirm keys render masked.
2. Change strategy to `balanced`, raise the proactive threshold, save. Confirm the toast and that `/tmp/swb-e2e/config.yaml` contains `balanced` while comments survive.
3. Add a key, set priority 2 and weight 3, save. Confirm the keys table on the main dashboard shows the new count.
4. Edit an existing key's weight only, save, then reload the page and confirm the weight stuck.
5. Rename an alias and delete another, save, confirm the footer alias list updates.
6. Set `ROUTING_STRATEGY=balanced` in the server environment before restart, reopen the dialog, confirm the strategy field is disabled with the `env` badge and a patch attempt shows the conflict toast.
7. Stop the server and confirm no stray `go run` process remains.

- [ ] **Step 4: Confirm the branch state**

Run: `git status --short --branch`
Expected: clean tree on `dashboard-runtime-config`.

Run: `git log --oneline main..HEAD`
Expected: the spec commit plus one commit per task.

- [ ] **Step 5: Commit any verification fixes**

If a step required a code change, commit it:

```bash
git add -A
git commit -s -m "fix: address issues found in end-to-end verification"
```
