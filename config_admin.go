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

	yaml "gopkg.in/yaml.v3"
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
