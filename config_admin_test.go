package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
