package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	yaml "gopkg.in/yaml.v3"
)

type UpstreamKeyConfig struct {
	Key      string `yaml:"key"`
	Priority int    `yaml:"priority"` // 1 = primary (default), 2+ = backup tiers
	Weight   int    `yaml:"weight"`   // >= 1 (default: 1)
}

type rawKeyEntry struct {
	Key      string `yaml:"key"`
	Priority int    `yaml:"priority"`
	Weight   int    `yaml:"weight"`
}

func (r *rawKeyEntry) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		r.Key = strings.TrimSpace(value.Value)
		r.Priority = 1
		r.Weight = 1
		return nil
	}
	type plain rawKeyEntry
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	*r = rawKeyEntry(p)
	if r.Priority <= 0 {
		r.Priority = 1
	}
	if r.Weight <= 0 {
		r.Weight = 1
	}
	return nil
}

type Config struct {
	ListenAddr          string
	UpstreamBaseURL     string
	ProxyAPIKey         string
	UpstreamAPIKeys     []string
	UpstreamKeyConfigs  []UpstreamKeyConfig
	MaxRequestBodyBytes int64
	// RetryExhaustedAfter is how long a key stays exhausted before it becomes
	// eligible for an automatic retry probe if upstream reset time is unknown.
	RetryExhaustedAfter      time.Duration
	RoutingStrategy          string
	SessionTTL               time.Duration
	BalancedIdleTimeout      time.Duration
	UsageCheckInterval       time.Duration
	ProactiveSwitchThreshold float64
	DisableUsagePolling      bool
	ConfigSourcePath         string

	ModelAliases          map[string]string
	SanitizeDeveloperRole bool

	SMTP SMTPConfig
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       string
	TLS      bool
	StartTLS bool
}

type APIStyle int

const (
	APIStyleOpenAI APIStyle = iota
	APIStyleAnthropic
)

func loadConfig() (Config, error) {
	cfg := defaultConfig()
	if path, ok, err := resolveConfigPath(); err != nil {
		return Config{}, err
	} else if ok {
		fileCfg, err := loadYAMLConfig(path)
		if err != nil {
			return Config{}, err
		}
		mergeConfig(&cfg, fileCfg)
		cfg.ConfigSourcePath = path
	}
	applyEnvOverrides(&cfg)
	return cfg, validateConfig(cfg)
}

func defaultConfig() Config {
	return Config{
		ListenAddr:               ":8080",
		UpstreamBaseURL:          "https://opencode.ai/zen/go/v1",
		MaxRequestBodyBytes:      20 << 20,
		RetryExhaustedAfter:      5 * time.Minute,
		RoutingStrategy:          "session_sticky",
		SessionTTL:               2 * time.Hour,
		BalancedIdleTimeout:      1 * time.Hour,
		UsageCheckInterval:       30 * time.Second,
		ProactiveSwitchThreshold: 95.0,
		DisableUsagePolling:      false,
		ModelAliases:             make(map[string]string),
		SanitizeDeveloperRole:    true,
		SMTP:                     SMTPConfig{Port: 25},
	}
}

func resolveConfigPath() (string, bool, error) {
	if explicit := strings.TrimSpace(os.Getenv("SWITCHBOARD_GO_CONFIG")); explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", false, fmt.Errorf("read SWITCHBOARD_GO_CONFIG: %w", err)
		}
		return explicit, true, nil
	}
	home, _ := os.UserHomeDir()
	paths := []string{}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".config", "switchboard-go", "config.yaml"))
	}
	paths = append(paths, "/etc/switchboard-go/config.yaml")
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, true, nil
		}
	}
	return "", false, nil
}

type yamlConfig struct {
	Server struct {
		ListenAddr  string `yaml:"listen_addr"`
		ProxyAPIKey string `yaml:"proxy_api_key"`
	} `yaml:"server"`
	Upstream struct {
		BaseURL                  string        `yaml:"base_url"`
		APIKeys                  []rawKeyEntry `yaml:"api_keys"`
		RetryExhaustedAfter      string        `yaml:"retry_exhausted_after"`
		RoutingStrategy          string        `yaml:"routing_strategy"`
		SessionTTL               string        `yaml:"session_ttl"`
		BalancedIdleTimeout      string        `yaml:"balanced_idle_timeout"`
		UsageCheckInterval       string        `yaml:"usage_check_interval"`
		ProactiveSwitchThreshold *float64      `yaml:"proactive_switch_threshold"`
		DisableUsagePolling      *bool         `yaml:"disable_usage_polling"`
	} `yaml:"upstream"`
	Models struct {
		Aliases map[string]string `yaml:"aliases"`
	} `yaml:"models"`
	Transformations struct {
		SanitizeDeveloperRole *bool `yaml:"sanitize_developer_role"`
	} `yaml:"transformations"`
	SMTP struct {
		Host     string `yaml:"host"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		From     string `yaml:"from"`
		To       string `yaml:"to"`
		Port     int    `yaml:"port"`
		TLS      bool   `yaml:"tls"`
		StartTLS bool   `yaml:"starttls"`
	} `yaml:"smtp"`
	Limits struct {
		MaxRequestBodyBytes int64 `yaml:"max_request_body_bytes"`
	} `yaml:"limits"`
}

func loadYAMLConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}
	var yc yamlConfig
	if err := yaml.Unmarshal(b, &yc); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	retry := time.Duration(-1)
	if s := strings.TrimSpace(yc.Upstream.RetryExhaustedAfter); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return Config{}, fmt.Errorf("parse retry_exhausted_after: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("retry_exhausted_after must be >= 0")
		}
		retry = d
	}

	sessionTTL := time.Duration(-1)
	if s := strings.TrimSpace(yc.Upstream.SessionTTL); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return Config{}, fmt.Errorf("parse session_ttl: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("session_ttl must be >= 0")
		}
		sessionTTL = d
	}

	balancedIdle := time.Duration(-1)
	if s := strings.TrimSpace(yc.Upstream.BalancedIdleTimeout); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return Config{}, fmt.Errorf("parse balanced_idle_timeout: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("balanced_idle_timeout must be >= 0")
		}
		balancedIdle = d
	}

	usageInterval := time.Duration(-1)
	if s := strings.TrimSpace(yc.Upstream.UsageCheckInterval); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return Config{}, fmt.Errorf("parse usage_check_interval: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("usage_check_interval must be >= 0")
		}
		usageInterval = d
	}

	proactiveThreshold := -1.0
	if yc.Upstream.ProactiveSwitchThreshold != nil {
		proactiveThreshold = *yc.Upstream.ProactiveSwitchThreshold
	}

	disablePolling := false
	if yc.Upstream.DisableUsagePolling != nil {
		disablePolling = *yc.Upstream.DisableUsagePolling
	}

	keyConfigs := make([]UpstreamKeyConfig, 0, len(yc.Upstream.APIKeys))
	keys := make([]string, 0, len(yc.Upstream.APIKeys))
	for _, entry := range yc.Upstream.APIKeys {
		if k := strings.TrimSpace(entry.Key); k != "" {
			p := entry.Priority
			if p <= 0 {
				p = 1
			}
			w := entry.Weight
			if w <= 0 {
				w = 1
			}
			keyConfigs = append(keyConfigs, UpstreamKeyConfig{Key: k, Priority: p, Weight: w})
			keys = append(keys, k)
		}
	}

	sanitizeRole := true
	if yc.Transformations.SanitizeDeveloperRole != nil {
		sanitizeRole = *yc.Transformations.SanitizeDeveloperRole
	}
	modelAliases := make(map[string]string)
	for k, v := range yc.Models.Aliases {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			modelAliases[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}

	return Config{
		ListenAddr:               yc.Server.ListenAddr,
		UpstreamBaseURL:          yc.Upstream.BaseURL,
		ProxyAPIKey:              yc.Server.ProxyAPIKey,
		UpstreamAPIKeys:          keys,
		UpstreamKeyConfigs:       keyConfigs,
		MaxRequestBodyBytes:      yc.Limits.MaxRequestBodyBytes,
		RetryExhaustedAfter:      retry,
		RoutingStrategy:          yc.Upstream.RoutingStrategy,
		SessionTTL:               sessionTTL,
		BalancedIdleTimeout:      balancedIdle,
		UsageCheckInterval:       usageInterval,
		ProactiveSwitchThreshold: proactiveThreshold,
		DisableUsagePolling:      disablePolling,
		ModelAliases:             modelAliases,
		SanitizeDeveloperRole:    sanitizeRole,
		SMTP: SMTPConfig{
			Host:     yc.SMTP.Host,
			Port:     yc.SMTP.Port,
			Username: yc.SMTP.Username,
			Password: yc.SMTP.Password,
			From:     yc.SMTP.From,
			To:       yc.SMTP.To,
			TLS:      yc.SMTP.TLS,
			StartTLS: yc.SMTP.StartTLS,
		},
	}, nil
}

func mergeConfig(dst *Config, src Config) {
	if src.ListenAddr != "" {
		dst.ListenAddr = src.ListenAddr
	}
	if src.UpstreamBaseURL != "" {
		dst.UpstreamBaseURL = src.UpstreamBaseURL
	}
	if src.ProxyAPIKey != "" {
		dst.ProxyAPIKey = src.ProxyAPIKey
	}
	if len(src.UpstreamKeyConfigs) > 0 {
		dst.UpstreamKeyConfigs = append([]UpstreamKeyConfig(nil), src.UpstreamKeyConfigs...)
		dst.UpstreamAPIKeys = append([]string(nil), src.UpstreamAPIKeys...)
	} else if len(src.UpstreamAPIKeys) > 0 {
		dst.UpstreamAPIKeys = append([]string(nil), src.UpstreamAPIKeys...)
		dst.UpstreamKeyConfigs = make([]UpstreamKeyConfig, len(src.UpstreamAPIKeys))
		for i, k := range src.UpstreamAPIKeys {
			dst.UpstreamKeyConfigs[i] = UpstreamKeyConfig{Key: k, Priority: 1, Weight: 1}
		}
	}
	if src.MaxRequestBodyBytes > 0 {
		dst.MaxRequestBodyBytes = src.MaxRequestBodyBytes
	}
	if src.RetryExhaustedAfter >= 0 {
		dst.RetryExhaustedAfter = src.RetryExhaustedAfter
	}
	if src.RoutingStrategy != "" {
		dst.RoutingStrategy = src.RoutingStrategy
	}
	if src.SessionTTL >= 0 {
		dst.SessionTTL = src.SessionTTL
	}
	if src.BalancedIdleTimeout >= 0 {
		dst.BalancedIdleTimeout = src.BalancedIdleTimeout
	}
	if src.UsageCheckInterval >= 0 {
		dst.UsageCheckInterval = src.UsageCheckInterval
	}
	if src.ProactiveSwitchThreshold >= 0 {
		dst.ProactiveSwitchThreshold = src.ProactiveSwitchThreshold
	}
	dst.DisableUsagePolling = src.DisableUsagePolling || dst.DisableUsagePolling
	if len(src.ModelAliases) > 0 {
		if dst.ModelAliases == nil {
			dst.ModelAliases = make(map[string]string)
		}
		for k, v := range src.ModelAliases {
			dst.ModelAliases[k] = v
		}
	}
	dst.SanitizeDeveloperRole = src.SanitizeDeveloperRole
	if src.SMTP.Host != "" {
		dst.SMTP.Host = src.SMTP.Host
	}
	if src.SMTP.Port != 0 {
		dst.SMTP.Port = src.SMTP.Port
	}
	if src.SMTP.Username != "" {
		dst.SMTP.Username = src.SMTP.Username
	}
	if src.SMTP.Password != "" {
		dst.SMTP.Password = src.SMTP.Password
	}
	if src.SMTP.From != "" {
		dst.SMTP.From = src.SMTP.From
	}
	if src.SMTP.To != "" {
		dst.SMTP.To = src.SMTP.To
	}
	dst.SMTP.TLS = src.SMTP.TLS || dst.SMTP.TLS
	dst.SMTP.StartTLS = src.SMTP.StartTLS || dst.SMTP.StartTLS
}

func parseCommaInts(s string) []int {
	var res []int
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				res = append(res, n)
			}
		}
	}
	return res
}

func applyEnvOverrides(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("LISTEN_ADDR")); v != "" {
		cfg.ListenAddr = v
	}
	if v := strings.TrimSpace(os.Getenv("UPSTREAM_BASE_URL")); v != "" {
		cfg.UpstreamBaseURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("PROXY_API_KEY")); v != "" {
		cfg.ProxyAPIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENCODE_GO_API_KEYS")); v != "" {
		var keys []string
		for _, k := range strings.Split(v, ",") {
			if s := strings.TrimSpace(k); s != "" {
				keys = append(keys, s)
			}
		}
		if len(keys) > 0 {
			cfg.UpstreamAPIKeys = keys
			priorities := parseCommaInts(os.Getenv("OPENCODE_GO_API_KEY_PRIORITIES"))
			weights := parseCommaInts(os.Getenv("OPENCODE_GO_API_KEY_WEIGHTS"))
			cfg.UpstreamKeyConfigs = make([]UpstreamKeyConfig, len(keys))
			for i, k := range keys {
				p := 1
				if i < len(priorities) && priorities[i] > 0 {
					p = priorities[i]
				}
				w := 1
				if i < len(weights) && weights[i] > 0 {
					w = weights[i]
				}
				cfg.UpstreamKeyConfigs[i] = UpstreamKeyConfig{Key: k, Priority: p, Weight: w}
			}
		}
	}
	if len(cfg.UpstreamKeyConfigs) == 0 && len(cfg.UpstreamAPIKeys) > 0 {
		cfg.UpstreamKeyConfigs = make([]UpstreamKeyConfig, len(cfg.UpstreamAPIKeys))
		for i, k := range cfg.UpstreamAPIKeys {
			cfg.UpstreamKeyConfigs[i] = UpstreamKeyConfig{Key: k, Priority: 1, Weight: 1}
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_REQUEST_BODY_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.MaxRequestBodyBytes = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RETRY_EXHAUSTED_AFTER")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			cfg.RetryExhaustedAfter = d
		} else {
			cfg.RetryExhaustedAfter = -1
		}
	}
	if v := strings.TrimSpace(os.Getenv("ROUTING_STRATEGY")); v != "" {
		cfg.RoutingStrategy = v
	}
	if v := strings.TrimSpace(os.Getenv("SESSION_TTL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			cfg.SessionTTL = d
		} else {
			cfg.SessionTTL = -1
		}
	}
	if v := strings.TrimSpace(os.Getenv("BALANCED_IDLE_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			cfg.BalancedIdleTimeout = d
		} else {
			cfg.BalancedIdleTimeout = -1
		}
	}
	if v := strings.TrimSpace(os.Getenv("USAGE_CHECK_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			cfg.UsageCheckInterval = d
		} else {
			cfg.UsageCheckInterval = -1
		}
	}
	if v := strings.TrimSpace(os.Getenv("PROACTIVE_SWITCH_THRESHOLD")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 100 {
			cfg.ProactiveSwitchThreshold = f
		} else {
			cfg.ProactiveSwitchThreshold = -1
		}
	}
	if v := os.Getenv("DISABLE_USAGE_POLLING"); v != "" {
		cfg.DisableUsagePolling = parseBool(v)
	}
	if v := os.Getenv("SANITIZE_DEVELOPER_ROLE"); v != "" {
		cfg.SanitizeDeveloperRole = parseBool(v)
	}
	if v := strings.TrimSpace(os.Getenv("MODEL_ALIASES")); v != "" {
		if cfg.ModelAliases == nil {
			cfg.ModelAliases = make(map[string]string)
		}
		for _, pair := range strings.Split(v, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				if k != "" && val != "" {
					cfg.ModelAliases[k] = val
				}
			}
		}
	}
	if v := os.Getenv("SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.Port = n
		}
	}
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		cfg.SMTP.Username = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		cfg.SMTP.Password = v
	}
	if v := os.Getenv("SMTP_FROM"); v != "" {
		cfg.SMTP.From = v
	}
	if v := os.Getenv("SMTP_TO"); v != "" {
		cfg.SMTP.To = v
	}
	if v := os.Getenv("SMTP_TLS"); v != "" {
		cfg.SMTP.TLS = parseBool(v)
	}
	if v := os.Getenv("SMTP_STARTTLS"); v != "" {
		cfg.SMTP.StartTLS = parseBool(v)
	}
}

func defaultString(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.ProxyAPIKey) == "" {
		return errors.New("PROXY_API_KEY is required")
	}
	if len(cfg.UpstreamAPIKeys) == 0 {
		return errors.New("OPENCODE_GO_API_KEYS is required")
	}
	if strings.TrimSpace(cfg.UpstreamBaseURL) == "" {
		return errors.New("UPSTREAM_BASE_URL is required")
	}
	if cfg.MaxRequestBodyBytes <= 0 {
		return errors.New("MAX_REQUEST_BODY_BYTES must be > 0")
	}
	if cfg.RetryExhaustedAfter < 0 {
		return errors.New("RETRY_EXHAUSTED_AFTER must be >= 0")
	}
	if cfg.SessionTTL < 0 {
		return errors.New("SESSION_TTL must be >= 0")
	}
	if cfg.BalancedIdleTimeout < 0 {
		return errors.New("BALANCED_IDLE_TIMEOUT must be >= 0")
	}
	if cfg.UsageCheckInterval < 0 {
		return errors.New("USAGE_CHECK_INTERVAL must be >= 0")
	}
	if cfg.ProactiveSwitchThreshold < 0 || cfg.ProactiveSwitchThreshold > 100 {
		return errors.New("PROACTIVE_SWITCH_THRESHOLD must be between 0 and 100")
	}
	strategy := strings.ToLower(strings.TrimSpace(cfg.RoutingStrategy))
	if strategy != "" && strategy != "session_sticky" && strategy != "balanced" && strategy != "round_robin" && strategy != "fill_first" {
		return fmt.Errorf("invalid ROUTING_STRATEGY: %q (must be one of: session_sticky, balanced, round_robin, fill_first)", cfg.RoutingStrategy)
	}
	return nil
}

func safeConfigSummary(cfg Config) string {
	strategy := defaultString(cfg.RoutingStrategy, "session_sticky")
	return fmt.Sprintf("listen=%s upstream=%s upstream_keys=%d strategy=%s session_ttl=%s balanced_idle_timeout=%s usage_check_interval=%s proactive_threshold=%.1f%% polling_disabled=%t smtp_configured=%t config_source=%s max_request_body_bytes=%d retry_exhausted_after=%s",
		cfg.ListenAddr, cfg.UpstreamBaseURL, len(cfg.UpstreamAPIKeys), strategy, cfg.SessionTTL, cfg.BalancedIdleTimeout, cfg.UsageCheckInterval, cfg.ProactiveSwitchThreshold, cfg.DisableUsagePolling, cfg.SMTP.Host != "" && cfg.SMTP.From != "" && cfg.SMTP.To != "", defaultString(cfg.ConfigSourcePath, "none"), cfg.MaxRequestBodyBytes, cfg.RetryExhaustedAfter)
}

func parseBool(v string) bool { b, _ := strconv.ParseBool(strings.TrimSpace(v)); return b }

type KeyState string

const (
	KeyUnknown   KeyState = "unknown"
	KeyAvailable KeyState = "available"
	KeyExhausted KeyState = "exhausted"
)

// UsageWindow represents a usage quota bucket (rolling, weekly, monthly).
type UsageWindow struct {
	Status   string    `json:"status"`
	Percent  float64   `json:"percent"`
	ResetsAt time.Time `json:"resetsAt"`
}

// KeyUsage represents complete multi-window quota usage for an API key.
type KeyUsage struct {
	Rolling UsageWindow `json:"rolling"`
	Weekly  UsageWindow `json:"weekly"`
	Monthly UsageWindow `json:"monthly"`
}

func parseUpstreamUsage(body []byte, now time.Time) (KeyUsage, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return KeyUsage{}, err
	}
	if u, ok := raw["usage"].(map[string]any); ok {
		raw = u
	}
	return KeyUsage{
		Rolling: parseUsageWindow(raw["rolling"], now),
		Weekly:  parseUsageWindow(raw["weekly"], now),
		Monthly: parseUsageWindow(raw["monthly"], now),
	}, nil
}

func parseUsageWindow(v any, now time.Time) UsageWindow {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return UsageWindow{Status: "ok", Percent: 0}
	}
	w := UsageWindow{Status: "ok"}
	if st, ok := m["status"].(string); ok && st != "" {
		w.Status = st
	}
	switch p := m["percent"].(type) {
	case float64:
		w.Percent = p
	case int:
		w.Percent = float64(p)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil {
			w.Percent = f
		}
	}
	w.ResetsAt = parseResetTime(m, now)
	return w
}

func parseResetTime(m map[string]any, now time.Time) time.Time {
	for _, key := range []string{"resetsAt", "resets_at", "reset_at", "resetAt"} {
		if val, ok := m[key]; ok && val != nil {
			switch v := val.(type) {
			case string:
				v = strings.TrimSpace(v)
				for _, layout := range []string{
					time.RFC3339Nano,
					time.RFC3339,
					"2006-01-02T15:04:05Z07:00",
					"2006-01-02T15:04:05.000Z",
					"2006-01-02T15:04:05",
					"2006-01-02 15:04:05",
				} {
					if t, err := time.Parse(layout, v); err == nil {
						return t.UTC()
					}
				}
			case float64:
				if v > 1e11 {
					return time.UnixMilli(int64(v)).UTC()
				} else if v > 1e8 {
					return time.Unix(int64(v), 0).UTC()
				}
			case int:
				if v > 1e8 {
					return time.Unix(int64(v), 0).UTC()
				}
			}
		}
	}
	for _, key := range []string{"resetsInSeconds", "resets_in_seconds", "reset_in_seconds"} {
		if val, ok := m[key]; ok && val != nil {
			switch v := val.(type) {
			case float64:
				if v > 0 {
					return now.Add(time.Duration(v) * time.Second).UTC()
				}
			case int:
				if v > 0 {
					return now.Add(time.Duration(v) * time.Second).UTC()
				}
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 {
					return now.Add(time.Duration(f) * time.Second).UTC()
				}
			}
		}
	}
	return time.Time{}
}

type sessionEntry struct {
	keyIndex   int
	lastActive time.Time
}

type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry
	ttl      time.Duration
	now      func() time.Time
}

func NewSessionManager(ttl time.Duration) *SessionManager {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	return &SessionManager{
		sessions: make(map[string]sessionEntry),
		ttl:      ttl,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (sm *SessionManager) GetKey(sessionID string) (int, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sessionID == "" {
		return 0, false
	}
	entry, ok := sm.sessions[sessionID]
	if !ok {
		return 0, false
	}
	if sm.now().Sub(entry.lastActive) > sm.ttl {
		delete(sm.sessions, sessionID)
		return 0, false
	}
	entry.lastActive = sm.now()
	sm.sessions[sessionID] = entry
	return entry.keyIndex, true
}

func (sm *SessionManager) SetKey(sessionID string, keyIndex int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sessionID == "" {
		return
	}
	sm.sessions[sessionID] = sessionEntry{
		keyIndex:   keyIndex,
		lastActive: sm.now(),
	}
}

func (sm *SessionManager) InvalidateKey(keyIndex int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for sid, entry := range sm.sessions {
		if entry.keyIndex == keyIndex {
			delete(sm.sessions, sid)
		}
	}
}

func (sm *SessionManager) ActiveCount() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	count := 0
	now := sm.now()
	for sid, entry := range sm.sessions {
		if now.Sub(entry.lastActive) <= sm.ttl {
			count++
		} else {
			delete(sm.sessions, sid)
		}
	}
	return count
}

func (sm *SessionManager) CleanupExpired() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := sm.now()
	for sid, entry := range sm.sessions {
		if now.Sub(entry.lastActive) > sm.ttl {
			delete(sm.sessions, sid)
		}
	}
}

func extractSessionID(r *http.Request, body []byte) string {
	for _, h := range []string{
		"x-session-id",
		"session-id",
		"x-conversation-id",
		"conversation-id",
		"x-agent-session",
	} {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			return v
		}
	}

	if len(body) > 0 {
		var raw map[string]any
		if json.Unmarshal(body, &raw) == nil {
			for _, f := range []string{"user", "prompt_cache_key", "session_id", "conversation_id"} {
				if s, ok := raw[f].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
			if msgs, ok := raw["messages"].([]any); ok && len(msgs) > 0 {
				if first, ok := msgs[0].(map[string]any); ok {
					var text string
					if content, ok := first["content"].(string); ok {
						text = content
					} else if contentArr, ok := first["content"].([]any); ok && len(contentArr) > 0 {
						if contentObj, ok := contentArr[0].(map[string]any); ok {
							if txt, ok := contentObj["text"].(string); ok {
								text = txt
							}
						}
					}
					if len(text) > 0 {
						if len(text) > 512 {
							text = text[:512]
						}
						h := sha256.Sum256([]byte(text))
						return "conv_" + hex.EncodeToString(h[:8])
					}
				}
			} else if prompt, ok := raw["prompt"].(string); ok && len(prompt) > 0 {
				if len(prompt) > 512 {
					prompt = prompt[:512]
				}
				h := sha256.Sum256([]byte(prompt))
				return "prompt_" + hex.EncodeToString(h[:8])
			} else if system, ok := raw["system"].(string); ok && len(system) > 0 {
				if len(system) > 512 {
					system = system[:512]
				}
				h := sha256.Sum256([]byte(system))
				return "system_" + hex.EncodeToString(h[:8])
			}
		}
	}
	return ""
}

type KeyManager struct {
	mu                  sync.Mutex
	keys                []string
	priorities          []int
	weights             []int
	currentWeights      []int
	states              []KeyState
	last429             map[int]time.Time
	resetTimes          map[int]time.Time
	usageData           map[int]*KeyUsage
	lastUsageCheck      map[int]time.Time
	usageErrors         map[int]string
	current             int
	roundRobinIndex     int
	allNotified         bool
	notifiedSwitch      map[int]bool
	cooldown            time.Duration
	routingStrategy     string
	proactiveThreshold  float64
	balancedIdleTimeout time.Duration
	lastGlobalRequest   time.Time
	sessionMgr          *SessionManager
	now                 func() time.Time
}

func NewKeyManager(keys []string, cooldown time.Duration) *KeyManager {
	configs := make([]UpstreamKeyConfig, len(keys))
	for i, k := range keys {
		configs[i] = UpstreamKeyConfig{Key: k, Priority: 1, Weight: 1}
	}
	return NewKeyManagerWithKeyConfigs(configs, cooldown, "session_sticky", 2*time.Hour, 1*time.Hour, 95.0)
}

func NewKeyManagerWithConfig(keys []string, cooldown time.Duration, strategy string, sessionTTL, balancedIdle time.Duration, proactiveThreshold float64) *KeyManager {
	configs := make([]UpstreamKeyConfig, len(keys))
	for i, k := range keys {
		configs[i] = UpstreamKeyConfig{Key: k, Priority: 1, Weight: 1}
	}
	return NewKeyManagerWithKeyConfigs(configs, cooldown, strategy, sessionTTL, balancedIdle, proactiveThreshold)
}

func NewKeyManagerWithKeyConfigs(configs []UpstreamKeyConfig, cooldown time.Duration, strategy string, sessionTTL, balancedIdle time.Duration, proactiveThreshold float64) *KeyManager {
	n := len(configs)
	keys := make([]string, n)
	priorities := make([]int, n)
	weights := make([]int, n)
	currentWeights := make([]int, n)
	states := make([]KeyState, n)

	for i, c := range configs {
		keys[i] = c.Key
		p := c.Priority
		if p <= 0 {
			p = 1
		}
		priorities[i] = p
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		weights[i] = w
		states[i] = KeyUnknown
	}

	if strategy == "" {
		strategy = "session_sticky"
	}
	if proactiveThreshold <= 0 {
		proactiveThreshold = 95.0
	}
	if balancedIdle <= 0 {
		balancedIdle = 1 * time.Hour
	}
	if sessionTTL <= 0 {
		sessionTTL = 2 * time.Hour
	}
	return &KeyManager{
		keys:                keys,
		priorities:          priorities,
		weights:             weights,
		currentWeights:      currentWeights,
		states:              states,
		last429:             map[int]time.Time{},
		resetTimes:          map[int]time.Time{},
		usageData:           map[int]*KeyUsage{},
		lastUsageCheck:      map[int]time.Time{},
		usageErrors:         map[int]string{},
		notifiedSwitch:      map[int]bool{},
		cooldown:            cooldown,
		routingStrategy:     strategy,
		proactiveThreshold:  proactiveThreshold,
		balancedIdleTimeout: balancedIdle,
		sessionMgr:          NewSessionManager(sessionTTL),
		now:                 func() time.Time { return time.Now().UTC() },
	}
}

func (m *KeyManager) eligibleLocked(i int) bool {
	if i < 0 || i >= len(m.keys) {
		return false
	}
	if m.states[i] != KeyExhausted {
		return true
	}
	if t, ok := m.resetTimes[i]; ok && !t.IsZero() {
		return !m.now().Before(t)
	}
	if t, ok := m.last429[i]; ok {
		return !m.now().Before(t.Add(m.cooldown))
	}
	return true
}

func (m *KeyManager) isProactivelySaturatedLocked(i int) bool {
	if u, ok := m.usageData[i]; ok && u != nil {
		return u.Rolling.Percent >= m.proactiveThreshold
	}
	return false
}

func (m *KeyManager) bestKeyByQuotaLocked(exclude map[int]bool) (int, bool) {
	if len(m.keys) == 0 {
		return 0, false
	}

	type candidate struct {
		index    int
		priority int
		weight   int
		weekly   float64
		monthly  float64
		rolling  float64
	}

	var healthy []candidate
	var saturated []candidate
	var exhausted []candidate

	for i := range m.keys {
		if exclude != nil && exclude[i] {
			continue
		}
		if !m.eligibleLocked(i) {
			continue
		}
		c := candidate{
			index:    i,
			priority: m.priorities[i],
			weight:   m.weights[i],
		}
		if u, ok := m.usageData[i]; ok && u != nil {
			c.weekly = u.Weekly.Percent
			c.monthly = u.Monthly.Percent
			c.rolling = u.Rolling.Percent
		}
		if m.states[i] == KeyExhausted {
			exhausted = append(exhausted, c)
		} else if m.isProactivelySaturatedLocked(i) {
			saturated = append(saturated, c)
		} else {
			healthy = append(healthy, c)
		}
	}

	filterBestTier := func(pool []candidate) []candidate {
		if len(pool) == 0 {
			return nil
		}
		minPriority := pool[0].priority
		for _, c := range pool[1:] {
			if c.priority < minPriority {
				minPriority = c.priority
			}
		}
		var tier []candidate
		for _, c := range pool {
			if c.priority == minPriority {
				tier = append(tier, c)
			}
		}
		return tier
	}

	pool := filterBestTier(healthy)
	if len(pool) == 0 {
		pool = filterBestTier(saturated)
	}
	if len(pool) == 0 {
		pool = filterBestTier(exhausted)
	}
	if len(pool) == 0 {
		return 0, false
	}

	best := pool[0]
	for _, c := range pool[1:] {
		if c.weekly < best.weekly {
			best = c
		} else if c.weekly == best.weekly {
			if c.monthly < best.monthly {
				best = c
			} else if c.monthly == best.monthly {
				if c.rolling < best.rolling {
					best = c
				} else if c.rolling == best.rolling {
					if c.weight > best.weight {
						best = c
					}
				}
			}
		}
	}
	return best.index, true
}

func (m *KeyManager) KeyForRequest(sessionID string, exclude ...map[int]bool) (int, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.keys) == 0 {
		return 0, "", false
	}

	var tried map[int]bool
	if len(exclude) > 0 && exclude[0] != nil {
		tried = exclude[0]
	}

	now := m.now()
	idleGap := now.Sub(m.lastGlobalRequest)
	m.lastGlobalRequest = now

	switch m.routingStrategy {
	case "session_sticky":
		if sessionID != "" && m.sessionMgr != nil {
			if k, ok := m.sessionMgr.GetKey(sessionID); ok {
				if (tried == nil || !tried[k]) && m.eligibleLocked(k) && !m.isProactivelySaturatedLocked(k) && m.states[k] != KeyExhausted {
					m.current = k
					return k, m.keys[k], true
				}
			}
		}
		best, ok := m.bestKeyByQuotaLocked(tried)
		if !ok {
			return 0, "", false
		}
		if sessionID != "" && m.sessionMgr != nil {
			m.sessionMgr.SetKey(sessionID, best)
		}
		m.current = best
		return best, m.keys[best], true

	case "balanced":
		if m.lastGlobalRequest.IsZero() || idleGap > m.balancedIdleTimeout || !m.eligibleLocked(m.current) || m.isProactivelySaturatedLocked(m.current) || m.states[m.current] == KeyExhausted || (tried != nil && tried[m.current]) {
			if best, ok := m.bestKeyByQuotaLocked(tried); ok {
				m.current = best
			}
		}
		if (tried == nil || !tried[m.current]) && m.eligibleLocked(m.current) {
			return m.current, m.keys[m.current], true
		}
		if best, ok := m.bestKeyByQuotaLocked(tried); ok {
			m.current = best
			return best, m.keys[best], true
		}
		return 0, "", false

	case "round_robin":
		var eligibleHealthy []int
		for i := range m.keys {
			if (tried == nil || !tried[i]) && m.eligibleLocked(i) && !m.isProactivelySaturatedLocked(i) && m.states[i] != KeyExhausted {
				eligibleHealthy = append(eligibleHealthy, i)
			}
		}
		var pool []int
		if len(eligibleHealthy) > 0 {
			minP := m.priorities[eligibleHealthy[0]]
			for _, idx := range eligibleHealthy[1:] {
				if m.priorities[idx] < minP {
					minP = m.priorities[idx]
				}
			}
			for _, idx := range eligibleHealthy {
				if m.priorities[idx] == minP {
					pool = append(pool, idx)
				}
			}
		} else {
			var allEligible []int
			for i := range m.keys {
				if (tried == nil || !tried[i]) && m.eligibleLocked(i) {
					allEligible = append(allEligible, i)
				}
			}
			if len(allEligible) > 0 {
				minP := m.priorities[allEligible[0]]
				for _, idx := range allEligible[1:] {
					if m.priorities[idx] < minP {
						minP = m.priorities[idx]
					}
				}
				for _, idx := range allEligible {
					if m.priorities[idx] == minP {
						pool = append(pool, idx)
					}
				}
			}
		}
		if len(pool) == 0 {
			return 0, "", false
		}

		totalWeight := 0
		bestIdx := pool[0]
		maxCurrentWeight := math.MinInt32

		for _, idx := range pool {
			w := m.weights[idx]
			if w <= 0 {
				w = 1
			}
			m.currentWeights[idx] += w
			totalWeight += w
			if m.currentWeights[idx] > maxCurrentWeight {
				maxCurrentWeight = m.currentWeights[idx]
				bestIdx = idx
			}
		}
		m.currentWeights[bestIdx] -= totalWeight
		m.current = bestIdx
		return bestIdx, m.keys[bestIdx], true

	case "fill_first":
		fallthrough
	default:
		type keyRef struct {
			idx      int
			priority int
		}
		ordered := make([]keyRef, len(m.keys))
		for i := range m.keys {
			ordered[i] = keyRef{idx: i, priority: m.priorities[i]}
		}
		for i := 0; i < len(ordered); i++ {
			for j := i + 1; j < len(ordered); j++ {
				if ordered[j].priority < ordered[i].priority {
					ordered[i], ordered[j] = ordered[j], ordered[i]
				}
			}
		}

		for _, ref := range ordered {
			i := ref.idx
			if (tried == nil || !tried[i]) && m.eligibleLocked(i) && !m.isProactivelySaturatedLocked(i) && m.states[i] != KeyExhausted {
				m.current = i
				return i, m.keys[i], true
			}
		}
		for _, ref := range ordered {
			i := ref.idx
			if (tried == nil || !tried[i]) && m.eligibleLocked(i) {
				m.current = i
				return i, m.keys[i], true
			}
		}
		return 0, "", false
	}
}

func (m *KeyManager) Current() (int, string, bool) {
	return m.KeyForRequest("")
}

func (m *KeyManager) MarkExhausted(i int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.keys) {
		return
	}
	m.states[i] = KeyExhausted
	m.last429[i] = m.now()
	if m.resetTimes[i].IsZero() || m.now().After(m.resetTimes[i]) {
		if m.cooldown > 0 {
			m.resetTimes[i] = m.now().Add(m.cooldown)
		}
	}
	if m.sessionMgr != nil {
		m.sessionMgr.InvalidateKey(i)
	}
	m.advanceLocked()
}

func (m *KeyManager) ShouldNotifySwitch(i int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.notifiedSwitch[i] {
		return false
	}
	m.notifiedSwitch[i] = true
	return true
}

func (m *KeyManager) ShouldNotifyAllExhausted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.allNotified {
		return false
	}
	if !m.allExhaustedLocked() {
		return false
	}
	m.allNotified = true
	return true
}

func (m *KeyManager) AdvanceOnExhaustion() (int, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.keys) == 0 {
		return 0, "", false
	}
	m.advanceLocked()
	return m.current, m.keys[m.current], true
}

func (m *KeyManager) advanceLocked() {
	if len(m.keys) == 0 {
		return
	}
	if best, ok := m.bestKeyByQuotaLocked(nil); ok {
		m.current = best
		return
	}
	start := m.current
	for step := 1; step <= len(m.keys); step++ {
		next := (start + step) % len(m.keys)
		if m.eligibleLocked(next) {
			m.current = next
			return
		}
	}
	m.current = start
}

func (m *KeyManager) RetryAfterSeconds() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var soonest time.Time
	found := false
	for i := range m.keys {
		if m.states[i] != KeyExhausted {
			return 0, false
		}
		var resetAt time.Time
		if t, ok := m.resetTimes[i]; ok && !t.IsZero() {
			resetAt = t
		} else if t, ok := m.last429[i]; ok && m.cooldown > 0 {
			resetAt = t.Add(m.cooldown)
		} else {
			return 0, false
		}
		if !found || resetAt.Before(soonest) {
			soonest = resetAt
			found = true
		}
	}
	if !found {
		return 0, false
	}
	secs := int(math.Ceil(soonest.Sub(m.now()).Seconds()))
	if secs < 1 {
		secs = 1
	}
	return secs, true
}

func (m *KeyManager) AllExhausted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.allExhaustedLocked()
}

func (m *KeyManager) allExhaustedLocked() bool {
	for _, st := range m.states {
		if st != KeyExhausted {
			return false
		}
	}
	return true
}

func (m *KeyManager) Status() StatusResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	states := make([]PerKeyStatus, len(m.keys))
	for i := range m.keys {
		state := m.states[i]
		display := state
		if i == m.current && state != KeyExhausted {
			display = KeyAvailable
		}
		eligible := m.eligibleLocked(i)
		ps := PerKeyStatus{
			Index:       i,
			State:       string(display),
			Priority:    m.priorities[i],
			Weight:      m.weights[i],
			Last429Time: m.last429String(i),
			Current:     i == m.current,
			Eligible:    eligible,
		}
		if state == KeyExhausted && !eligible {
			var resetAt time.Time
			if t, ok := m.resetTimes[i]; ok && !t.IsZero() {
				resetAt = t
			} else if t, ok := m.last429[i]; ok && m.cooldown > 0 {
				resetAt = t.Add(m.cooldown)
			}
			if !resetAt.IsZero() {
				secs := int(math.Ceil(resetAt.Sub(m.now()).Seconds()))
				if secs < 0 {
					secs = 0
				}
				ps.RetryAfterSeconds = secs
			}
		}
		states[i] = ps
	}
	return StatusResponse{
		CurrentKeyIndex:            m.current,
		Keys:                       states,
		RetryExhaustedAfterSeconds: int(m.cooldown / time.Second),
		Note:                       "unknown means the key has not yet been validated or used since startup; an exhausted key becomes eligible for an automatic retry once retry_exhausted_after_seconds has elapsed since last_429_time or resetsAt is reached.",
	}
}

func (m *KeyManager) SetState(i int, state KeyState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.states) {
		return
	}
	m.states[i] = state
	if state == KeyExhausted {
		m.last429[i] = m.now()
		if m.resetTimes[i].IsZero() || m.now().After(m.resetTimes[i]) {
			if m.cooldown > 0 {
				m.resetTimes[i] = m.now().Add(m.cooldown)
			}
		}
		if m.sessionMgr != nil {
			m.sessionMgr.InvalidateKey(i)
		}
		return
	}
	delete(m.last429, i)
	delete(m.resetTimes, i)
	m.notifiedSwitch[i] = false
	m.allNotified = false
}

func (m *KeyManager) MarkAvailable(i int) { m.SetState(i, KeyAvailable) }

func (m *KeyManager) UpdateUsage(i int, usage *KeyUsage, errStr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.keys) {
		return
	}
	m.lastUsageCheck[i] = m.now()
	m.usageErrors[i] = errStr
	if usage != nil {
		m.usageData[i] = usage
		if !usage.Rolling.ResetsAt.IsZero() {
			m.resetTimes[i] = usage.Rolling.ResetsAt
		}
		if m.states[i] == KeyExhausted && usage.Rolling.Percent < m.proactiveThreshold {
			m.states[i] = KeyAvailable
			delete(m.last429, i)
			m.notifiedSwitch[i] = false
			m.allNotified = false
		}
	}
}

func (m *KeyManager) last429String(i int) string {
	if t, ok := m.last429[i]; ok && !t.IsZero() {
		return t.Format(time.RFC3339)
	}
	return ""
}

type PerKeyStatus struct {
	Index             int    `json:"index"`
	State             string `json:"state"`
	Priority          int    `json:"priority"`
	Weight            int    `json:"weight"`
	Last429Time       string `json:"last_429_time,omitempty"`
	Current           bool   `json:"current"`
	Eligible          bool   `json:"eligible"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

type StatusResponse struct {
	CurrentKeyIndex            int            `json:"current_key_index"`
	Keys                       []PerKeyStatus `json:"keys"`
	RetryExhaustedAfterSeconds int            `json:"retry_exhausted_after_seconds"`
	Note                       string         `json:"note"`
}

type UsageSummaryPool struct {
	Rolling SummaryWindow `json:"rolling"`
	Weekly  SummaryWindow `json:"weekly"`
	Monthly SummaryWindow `json:"monthly"`
}

type SummaryWindow struct {
	AveragePercent        float64    `json:"average_percent"`
	TotalRemainingPercent float64    `json:"total_remaining_percent"`
	MinPercent            float64    `json:"min_percent"`
	MaxPercent            float64    `json:"max_percent"`
	EarliestResetAt       *time.Time `json:"earliest_reset_at,omitempty"`
}

type UsageSummary struct {
	TotalKeys                 int               `json:"total_keys"`
	AvailableKeys             int               `json:"available_keys"`
	ExhaustedKeys             int               `json:"exhausted_keys"`
	ActiveSessions            int               `json:"active_sessions"`
	RoutingStrategy           string            `json:"routing_strategy"`
	ProactiveThresholdPercent float64           `json:"proactive_threshold_percent"`
	PoolUsage                 *UsageSummaryPool `json:"pool_usage,omitempty"`
}

type PerKeyUsage struct {
	Index             int         `json:"index"`
	State             string      `json:"state"`
	Priority          int         `json:"priority"`
	Weight            int         `json:"weight"`
	Current           bool        `json:"current"`
	Eligible          bool        `json:"eligible"`
	RetryAfterSeconds int         `json:"retry_after_seconds,omitempty"`
	Rolling           UsageWindow `json:"rolling"`
	Weekly            UsageWindow `json:"weekly"`
	Monthly           UsageWindow `json:"monthly"`
	LastCheckedAt     string      `json:"last_checked_at,omitempty"`
	Error             string      `json:"error,omitempty"`
}

type AggregatedUsageResponse struct {
	Rolling UsageWindow   `json:"rolling"`
	Weekly  UsageWindow   `json:"weekly"`
	Monthly UsageWindow   `json:"monthly"`
	Summary UsageSummary  `json:"summary"`
	Keys    []PerKeyUsage `json:"keys"`
}

func (m *KeyManager) GetAggregatedUsage() AggregatedUsageResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := len(m.keys)
	keysList := make([]PerKeyUsage, n)

	var totalRolling, totalWeekly, totalMonthly float64
	minRolling, maxRolling := 100.0, 0.0
	minWeekly, maxWeekly := 100.0, 0.0
	minMonthly, maxMonthly := 100.0, 0.0
	var earliestReset *time.Time

	availableCount := 0
	exhaustedCount := 0

	var hasUsageData bool
	for i := range m.keys {
		state := m.states[i]
		display := state
		if i == m.current && state != KeyExhausted {
			display = KeyAvailable
		}
		eligible := m.eligibleLocked(i)
		if state == KeyExhausted {
			exhaustedCount++
		} else if eligible && !m.isProactivelySaturatedLocked(i) {
			availableCount++
		}

		pku := PerKeyUsage{
			Index:    i,
			State:    string(display),
			Priority: m.priorities[i],
			Weight:   m.weights[i],
			Current:  i == m.current,
			Eligible: eligible,
			Error:    m.usageErrors[i],
		}

		if t, ok := m.lastUsageCheck[i]; ok && !t.IsZero() {
			pku.LastCheckedAt = t.Format(time.RFC3339)
		}

		if state == KeyExhausted && !eligible {
			var resetAt time.Time
			if t, ok := m.resetTimes[i]; ok && !t.IsZero() {
				resetAt = t
			} else if t, ok := m.last429[i]; ok && m.cooldown > 0 {
				resetAt = t.Add(m.cooldown)
			}
			if !resetAt.IsZero() {
				secs := int(math.Ceil(resetAt.Sub(m.now()).Seconds()))
				if secs < 0 {
					secs = 0
				}
				pku.RetryAfterSeconds = secs
			}
		}

		if u, ok := m.usageData[i]; ok && u != nil {
			hasUsageData = true
			pku.Rolling = u.Rolling
			pku.Weekly = u.Weekly
			pku.Monthly = u.Monthly

			totalRolling += u.Rolling.Percent
			totalWeekly += u.Weekly.Percent
			totalMonthly += u.Monthly.Percent

			if u.Rolling.Percent < minRolling {
				minRolling = u.Rolling.Percent
			}
			if u.Rolling.Percent > maxRolling {
				maxRolling = u.Rolling.Percent
			}

			if u.Weekly.Percent < minWeekly {
				minWeekly = u.Weekly.Percent
			}
			if u.Weekly.Percent > maxWeekly {
				maxWeekly = u.Weekly.Percent
			}

			if u.Monthly.Percent < minMonthly {
				minMonthly = u.Monthly.Percent
			}
			if u.Monthly.Percent > maxMonthly {
				maxMonthly = u.Monthly.Percent
			}

			if !u.Rolling.ResetsAt.IsZero() {
				if earliestReset == nil || u.Rolling.ResetsAt.Before(*earliestReset) {
					t := u.Rolling.ResetsAt
					earliestReset = &t
				}
			}
		} else {
			pku.Rolling = UsageWindow{Status: string(state)}
			pku.Weekly = UsageWindow{Status: string(state)}
			pku.Monthly = UsageWindow{Status: string(state)}
		}

		keysList[i] = pku
	}

	var avgRolling, avgWeekly, avgMonthly float64
	if n > 0 && hasUsageData {
		avgRolling = totalRolling / float64(n)
		avgWeekly = totalWeekly / float64(n)
		avgMonthly = totalMonthly / float64(n)
	} else {
		minRolling, minWeekly, minMonthly = 0, 0, 0
	}

	poolCapacity := float64(n) * 100.0

	activeSessions := 0
	if m.sessionMgr != nil {
		activeSessions = m.sessionMgr.ActiveCount()
	}

	pool := &UsageSummaryPool{
		Rolling: SummaryWindow{
			AveragePercent:        avgRolling,
			TotalRemainingPercent: poolCapacity - totalRolling,
			MinPercent:            minRolling,
			MaxPercent:            maxRolling,
			EarliestResetAt:       earliestReset,
		},
		Weekly: SummaryWindow{
			AveragePercent:        avgWeekly,
			TotalRemainingPercent: poolCapacity - totalWeekly,
			MinPercent:            minWeekly,
			MaxPercent:            maxWeekly,
		},
		Monthly: SummaryWindow{
			AveragePercent:        avgMonthly,
			TotalRemainingPercent: poolCapacity - totalMonthly,
			MinPercent:            minMonthly,
			MaxPercent:            maxMonthly,
		},
	}

	return AggregatedUsageResponse{
		Rolling: UsageWindow{
			Status:  "ok",
			Percent: avgRolling,
			ResetsAt: func() time.Time {
				if earliestReset != nil {
					return *earliestReset
				}
				return time.Time{}
			}(),
		},
		Weekly: UsageWindow{
			Status:  "ok",
			Percent: avgWeekly,
		},
		Monthly: UsageWindow{
			Status:  "ok",
			Percent: avgMonthly,
		},
		Summary: UsageSummary{
			TotalKeys:                 n,
			AvailableKeys:             availableCount,
			ExhaustedKeys:             exhaustedCount,
			ActiveSessions:            activeSessions,
			RoutingStrategy:           m.routingStrategy,
			ProactiveThresholdPercent: m.proactiveThreshold,
			PoolUsage:                 pool,
		},
		Keys: keysList,
	}
}

type ValidateKeyResult struct {
	Index  int    `json:"index"`
	State  string `json:"state"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

type ValidateKeysResponse struct {
	Results []ValidateKeyResult `json:"results"`
}

type App struct {
	config Config
	keys   *KeyManager
	client *http.Client
	sender *SMTPNotifier
}

func newApp(cfg Config) *App {
	var km *KeyManager
	if len(cfg.UpstreamKeyConfigs) > 0 {
		km = NewKeyManagerWithKeyConfigs(
			cfg.UpstreamKeyConfigs,
			cfg.RetryExhaustedAfter,
			cfg.RoutingStrategy,
			cfg.SessionTTL,
			cfg.BalancedIdleTimeout,
			cfg.ProactiveSwitchThreshold,
		)
	} else {
		km = NewKeyManagerWithConfig(
			cfg.UpstreamAPIKeys,
			cfg.RetryExhaustedAfter,
			cfg.RoutingStrategy,
			cfg.SessionTTL,
			cfg.BalancedIdleTimeout,
			cfg.ProactiveSwitchThreshold,
		)
	}
	return &App{
		config: cfg,
		keys:   km,
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
		sender: NewSMTPNotifier(cfg.SMTP),
	}
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/healthz":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	case r.URL.Path == "/readyz":
		a.handleReadyz(w, r)
	case r.URL.Path == "/usage" || r.URL.Path == "/v1/usage" || r.URL.Path == "/admin/usage":
		if !a.authOK(r) {
			writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "Unauthorized")
			return
		}
		a.handleUsage(w, r)
	case strings.HasPrefix(r.URL.Path, "/admin/"):
		if !a.authOK(r) {
			writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "Unauthorized")
			return
		}
		a.handleAdmin(w, r)
	case isProxyPath(r.URL.Path):
		if !a.authOK(r) {
			writeAPIError(w, apiStyleForRequest(r), http.StatusUnauthorized, "invalid_api_key", "Unauthorized")
			return
		}
		a.proxyV1(w, r, apiStyleForRequest(r))
	default:
		http.NotFound(w, r)
	}
}

func (a *App) authOK(r *http.Request) bool {
	want := a.config.ProxyAPIKey
	if want == "" {
		return false
	}
	if tok := bearerToken(r.Header.Get("Authorization")); tok != "" {
		return subtle.ConstantTimeCompare([]byte(tok), []byte(want)) == 1
	}
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("x-api-key")), []byte(want)) == 1
}

func bearerToken(v string) string {
	parts := strings.Fields(v)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func isProxyPath(path string) bool {
	if strings.HasPrefix(path, "/v1/") {
		return true
	}
	switch path {
	case "/models", "/chat/completions", "/messages", "/complete", "/responses", "/embeddings":
		return true
	default:
		return false
	}
}

func (a *App) handleAdmin(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/admin/validate-keys" && r.Method == http.MethodPost:
		a.handleValidateKeys(w, r)
	case r.URL.Path == "/admin/reset-key" && r.Method == http.MethodPost:
		a.handleResetKey(w, r)
	case r.URL.Path == "/admin/reset-all-keys" && r.Method == http.MethodPost:
		a.handleResetAllKeys(w, r)
	case r.URL.Path == "/admin/status" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a.keys.Status())
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("refresh") == "true" {
		a.pollAllKeysUsage(r.Context())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.keys.GetAggregatedUsage())
}

func (a *App) handleResetKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Index int `json:"index"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if body.Index < 0 || body.Index >= len(a.config.UpstreamAPIKeys) {
		http.Error(w, "index out of range", http.StatusBadRequest)
		return
	}
	a.keys.MarkAvailable(body.Index)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.keys.Status())
}

func (a *App) handleResetAllKeys(w http.ResponseWriter, r *http.Request) {
	for i := range a.config.UpstreamAPIKeys {
		a.keys.MarkAvailable(i)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.keys.Status())
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"ready": false}
	if err := validateConfig(a.config); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	_, key, ok := a.keys.Current()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	if err := a.checkUpstreamReady(r.Context(), key); err != nil {
		resp["error"] = "upstream not ready"
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	resp["ready"] = true
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *App) checkUpstreamReady(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.config.UpstreamBaseURL, "/")+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("upstream status %d", resp.StatusCode)
}

func (a *App) fetchUpstreamUsage(ctx context.Context, key string) (KeyUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	u := strings.TrimRight(a.config.UpstreamBaseURL, "/") + "/usage"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return KeyUsage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")

	resp, err := a.client.Do(req)
	if err != nil {
		return KeyUsage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return KeyUsage{}, fmt.Errorf("upstream /usage returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return KeyUsage{}, err
	}
	return parseUpstreamUsage(body, time.Now().UTC())
}

func (a *App) pollAllKeysUsage(ctx context.Context) {
	for i, key := range a.config.UpstreamAPIKeys {
		usage, err := a.fetchUpstreamUsage(ctx, key)
		if err != nil {
			a.keys.UpdateUsage(i, nil, err.Error())
		} else {
			a.keys.UpdateUsage(i, &usage, "")
		}
	}
}

func (a *App) startUsagePoller(ctx context.Context) {
	if a.config.DisableUsagePolling || a.config.UsageCheckInterval <= 0 {
		return
	}
	go a.pollAllKeysUsage(ctx)

	ticker := time.NewTicker(a.config.UsageCheckInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if a.keys.sessionMgr != nil {
					a.keys.sessionMgr.CleanupExpired()
				}
				a.pollAllKeysUsage(ctx)
			}
		}
	}()
}

func (a *App) handleValidateKeys(w http.ResponseWriter, r *http.Request) {
	results := make([]ValidateKeyResult, 0, len(a.config.UpstreamAPIKeys))
	for i, key := range a.config.UpstreamAPIKeys {
		res := ValidateKeyResult{Index: i}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.config.UpstreamBaseURL, "/")+"/models", nil)
		if err != nil {
			cancel()
			res.State = string(KeyUnknown)
			res.Status = http.StatusBadGateway
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
		resp, err := a.client.Do(req)
		cancel()
		if err != nil {
			res.State = string(KeyUnknown)
			res.Status = http.StatusBadGateway
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.Status = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.keys.MarkAvailable(i)
			res.State = string(KeyAvailable)
		} else if resp.StatusCode == http.StatusTooManyRequests && isQuota429(resp) {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.keys.SetState(i, KeyExhausted)
			res.State = string(KeyExhausted)
			res.Error = "quota exhausted"
		} else {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.keys.SetState(i, KeyUnknown)
			res.State = string(KeyUnknown)
			res.Error = fmt.Sprintf("status %d", resp.StatusCode)
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, ValidateKeysResponse{Results: results})
}

func (a *App) proxyV1(w http.ResponseWriter, r *http.Request, style APIStyle) {
	r.Body = http.MaxBytesReader(w, r.Body, a.config.MaxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()
	orig := r.Context()

	sessionID := extractSessionID(r, body)
	transformedBody := a.transformRequestBody(body, style == APIStyleAnthropic)

	isModelsEndpoint := (r.URL.Path == "/models" || r.URL.Path == "/v1/models") && r.Method == http.MethodGet

	tried := make(map[int]bool)

	for attempts := 0; attempts < len(a.config.UpstreamAPIKeys); attempts++ {
		idx, key, ok := a.keys.KeyForRequest(sessionID, tried)
		if !ok {
			break
		}
		tried[idx] = true
		resp, reqErr := a.doUpstream(orig, r, transformedBody, key, style)
		if reqErr != nil {
			http.Error(w, reqErr.Error(), http.StatusBadGateway)
			return
		}
		if resp.StatusCode == http.StatusTooManyRequests && isQuota429(resp) {
			_ = resp.Body.Close()
			a.keys.MarkExhausted(idx)
			if a.keys.ShouldNotifySwitch(idx) {
				a.sender.NotifySwitch(idx, a.keys.Status())
			}
			continue
		}
		// The key served the request, so it is healthy: mark it available so a
		// recovered (previously exhausted) key un-sticks and the notification
		// flags re-arm for the next depletion round.
		a.keys.MarkAvailable(idx)

		if isModelsEndpoint && len(a.config.ModelAliases) > 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			respBody, rErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if rErr == nil {
				respBody = a.augmentModelsResponse(respBody)
			}
			for k, vv := range resp.Header {
				if strings.EqualFold(k, "Content-Length") {
					continue
				}
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(respBody)))
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(respBody)
			return
		}

		copyResponse(w, resp)
		return
	}
	// No eligible key remains. Fail fast locally instead of hammering upstream,
	// and hint well-behaved clients at the next probe window via Retry-After.
	if a.keys.ShouldNotifyAllExhausted() {
		a.sender.NotifyAllExhausted(a.keys.Status())
	}
	if secs, ok := a.keys.RetryAfterSeconds(); ok {
		w.Header().Set("Retry-After", strconv.Itoa(secs))
	}
	writeAPIError(w, style, http.StatusTooManyRequests, "rate_limit_exceeded", "all upstream keys exhausted")
}

func (a *App) transformRequestBody(body []byte, isAnthropic bool) []byte {
	if len(body) == 0 {
		return body
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}
	mutated := false

	if m, ok := raw["model"].(string); ok && len(a.config.ModelAliases) > 0 {
		if target, exists := a.config.ModelAliases[m]; exists && target != "" {
			raw["model"] = target
			mutated = true
		}
	}

	if !isAnthropic && a.config.SanitizeDeveloperRole {
		if msgs, ok := raw["messages"].([]any); ok {
			for _, item := range msgs {
				if msgObj, ok := item.(map[string]any); ok {
					if role, ok := msgObj["role"].(string); ok && strings.EqualFold(role, "developer") {
						msgObj["role"] = "system"
						mutated = true
					}
				}
			}
		}
	}

	if !mutated {
		return body
	}
	newBody, err := json.Marshal(raw)
	if err != nil {
		return body
	}
	return newBody
}

func (a *App) augmentModelsResponse(body []byte) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	dataList, ok := obj["data"].([]any)
	if !ok {
		return body
	}
	existing := make(map[string]bool)
	for _, item := range dataList {
		if m, ok := item.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				existing[id] = true
			}
		}
	}
	for alias := range a.config.ModelAliases {
		if !existing[alias] {
			dataList = append(dataList, map[string]any{
				"id":       alias,
				"object":   "model",
				"created":  time.Now().Unix(),
				"owned_by": "switchboard-alias",
			})
		}
	}
	obj["data"] = dataList
	if augmented, err := json.Marshal(obj); err == nil {
		return augmented
	}
	return body
}

func (a *App) doUpstream(ctx context.Context, r *http.Request, body []byte, key string, apiStyle APIStyle) (*http.Response, error) {
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/v1")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := a.config.UpstreamBaseURL + path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.ContentLength = int64(len(body))
	copyHeaders(req.Header, r.Header)
	req.Header.Del("Content-Length")
	if apiStyle == APIStyleAnthropic {
		req.Header.Set("x-api-key", key)
		if strings.TrimSpace(req.Header.Get("anthropic-version")) == "" {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
		if strings.TrimSpace(req.Header.Get("User-Agent")) == "" {
			req.Header.Set("User-Agent", "anthropic-sdk-go/1.0.0")
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
		if strings.TrimSpace(req.Header.Get("User-Agent")) == "" {
			req.Header.Set("User-Agent", "OpenAI/Python 1.0.0")
		}
	}
	stripHopByHopHeaders(req.Header)
	return a.client.Do(req)
}

func apiStyleForRequest(r *http.Request) APIStyle {
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	if strings.HasPrefix(path, "/messages") || strings.HasPrefix(path, "/complete") {
		return APIStyleAnthropic
	}
	for name := range r.Header {
		if strings.HasPrefix(strings.ToLower(name), "anthropic-") {
			return APIStyleAnthropic
		}
	}
	return APIStyleOpenAI
}

func (a *App) validateConfigAndPrint() error {
	if err := validateConfig(a.config); err != nil {
		return err
	}
	log.Println(safeConfigSummary(a.config))
	return nil
}

func copyHeaders(dst, src http.Header) {
	for k, v := range src {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "x-api-key") {
			continue
		}
		for _, s := range v {
			dst.Add(k, s)
		}
	}
}

func stripHopByHopHeaders(h http.Header) {
	hop := map[string]struct{}{
		"Connection":          {},
		"Proxy-Authorization": {},
		"Proxy-Authenticate":  {},
		"Keep-Alive":          {},
		"Te":                  {},
		"Trailer":             {},
		"Transfer-Encoding":   {},
		"Upgrade":             {},
	}
	for _, v := range h.Values("Connection") {
		for _, part := range strings.Split(v, ",") {
			if name := strings.TrimSpace(part); name != "" {
				hop[http.CanonicalHeaderKey(name)] = struct{}{}
			}
		}
	}
	for k := range hop {
		h.Del(k)
	}
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil && n > 0 {
		fw.f.Flush()
	}
	return n, err
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	defer resp.Body.Close()
	stripHopByHopHeaders(resp.Header)
	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}
	w.WriteHeader(resp.StatusCode)
	var writer io.Writer = w
	if f, ok := w.(http.Flusher); ok {
		writer = flushWriter{w: w, f: f}
	}
	_, _ = io.Copy(writer, resp.Body)
}

func isQuota429(resp *http.Response) bool {
	if resp.StatusCode != 429 {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		if e, ok := m["error"].(map[string]any); ok {
			if code, _ := e["code"].(string); code == "insufficient_quota" || code == "usage_not_included" {
				return true
			}
			typ, _ := e["type"].(string)
			msg, _ := e["message"].(string)
			lowType := strings.ToLower(typ)
			lowMsg := strings.ToLower(msg)
			if strings.Contains(lowType, "usage") || strings.Contains(lowType, "quota") || strings.Contains(lowType, "freeusagelimit") {
				return true
			}
			if strings.Contains(lowMsg, "quota") || strings.Contains(lowMsg, "exhausted") || strings.Contains(lowMsg, "usage limit") || strings.Contains(lowMsg, "credit balance") || strings.Contains(lowMsg, "billing limit") {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(resp.Header.Get("X-RateLimit-Reason")), "quota")
}

func writeAPIError(w http.ResponseWriter, style APIStyle, status int, code, message string) {
	if style == APIStyleAnthropic {
		writeAnthropicError(w, status, code, message)
		return
	}
	writeOpenAIError(w, status, code, message)
}

func writeOpenAIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    code,
		},
	})
}

func writeAnthropicError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    anthropicErrorType(status, code),
			"message": message,
		},
	})
}

func anthropicErrorType(status int, code string) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "authentication_error"
	case status == http.StatusTooManyRequests || code == "rate_limit_exceeded":
		return "rate_limit_error"
	case status >= 500:
		return "api_error"
	default:
		return "invalid_request_error"
	}
}

type SMTPNotifier struct{ cfg SMTPConfig }

func NewSMTPNotifier(cfg SMTPConfig) *SMTPNotifier { return &SMTPNotifier{cfg: cfg} }
func (n *SMTPNotifier) NotifySwitch(idx int, st StatusResponse) {
	go n.send("Switchboard Go switched upstream key", fmt.Sprintf("Switched away from key %d\n\n%+v", idx, st))
}
func (n *SMTPNotifier) NotifyAllExhausted(st StatusResponse) {
	go n.send("Switchboard Go exhausted all upstream keys", fmt.Sprintf("All keys exhausted\n\n%+v", st))
}
func (n *SMTPNotifier) send(subject, body string) {
	if n.cfg.Host == "" || n.cfg.To == "" || n.cfg.From == "" {
		return
	}
	addr := net.JoinHostPort(n.cfg.Host, strconv.Itoa(n.cfg.Port))
	auth := smtp.Auth(nil)
	if strings.TrimSpace(n.cfg.Username) != "" {
		auth = smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
	}
	msg := []byte("To: " + n.cfg.To + "\r\nSubject: " + subject + "\r\n\r\n" + body + "\r\n")
	if err := sendMail(addr, auth, n.cfg.From, []string{n.cfg.To}, msg, n.cfg.TLS, n.cfg.StartTLS); err != nil {
		log.Printf("smtp notification failed: %v", err)
	}
}

var sendMail = func(addr string, auth smtp.Auth, from string, to []string, msg []byte, useTLS, useStartTLS bool) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	host, _, _ := net.SplitHostPort(addr)
	if useTLS {
		conn = tls.Client(conn, &tls.Config{ServerName: host})
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Quit()
	if useStartTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return err
			}
		}
	}
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "validate-config" {
		cfg, err := loadConfig()
		if err != nil {
			log.Fatal(err)
		}
		if err := validateConfig(cfg); err != nil {
			log.Fatal(err)
		}
		log.Println(safeConfigSummary(cfg))
		return
	}
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	a := newApp(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a.startUsagePoller(ctx)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           a,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       65 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	log.Printf("startup %s", safeConfigSummary(cfg))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
