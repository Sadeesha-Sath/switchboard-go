package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
