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
