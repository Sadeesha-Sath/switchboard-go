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
