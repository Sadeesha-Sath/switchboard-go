package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed all:web/dashboard/dist
var dashboardFS embed.FS

// maskKeyHint returns a short, non-secret identifier for an upstream key so
// operators can tell subscriptions apart in the dashboard. Long keys reveal
// the first 7 and last 4 characters; short keys are fully masked.
func maskKeyHint(key string) string {
	if key == "" {
		return ""
	}
	if len(key) >= 16 {
		return key[:7] + "…" + key[len(key)-4:]
	}
	return "…"
}

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/":
		http.Redirect(w, r, "/dashboard/", http.StatusFound)
		return
	case "/dashboard":
		http.Redirect(w, r, "/dashboard/", http.StatusFound)
		return
	case "/dashboard/api/metrics.json":
		a.handleDashboardMetricsJSON(w, r)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/dashboard/") {
		http.NotFound(w, r)
		return
	}
	sub, err := fs.Sub(dashboardFS, "web/dashboard/dist")
	if err != nil {
		http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
		return
	}
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/dashboard")
	http.FileServer(http.FS(sub)).ServeHTTP(w, r)
}

func (a *App) handleDashboardMetricsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap := a.metrics.Snapshot(a.keys)
	snap.ModelAliases = a.config.ModelAliases
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

type HTTPRequestMetric struct {
	Endpoint string `json:"endpoint"`
	Method   string `json:"method"`
	Status   int    `json:"status"`
	Count    uint64 `json:"count"`
}

type HTTPDurationMetric struct {
	Endpoint             string  `json:"endpoint"`
	Method               string  `json:"method"`
	DurationSecondsSum   float64 `json:"duration_seconds_sum"`
	DurationSecondsCount uint64  `json:"duration_seconds_count"`
}

type UpstreamRequestMetric struct {
	KeyIndex             int     `json:"key_index"`
	Priority             int     `json:"priority"`
	Status               int     `json:"status"`
	Count                uint64  `json:"count"`
	DurationSecondsSum   float64 `json:"duration_seconds_sum"`
	DurationSecondsCount uint64  `json:"duration_seconds_count"`
}

type KeyExhaustionMetric struct {
	KeyIndex int    `json:"key_index"`
	Count    uint64 `json:"count"`
}

type KeySwitchMetric struct {
	FromKey int    `json:"from_key"`
	ToKey   int    `json:"to_key"`
	Reason  string `json:"reason"`
	Count   uint64 `json:"count"`
}

type MetricsSnapshot struct {
	GeneratedAt      string                  `json:"generated_at"`
	HTTPRequests     []HTTPRequestMetric     `json:"http_requests"`
	HTTPDurations    []HTTPDurationMetric    `json:"http_durations"`
	UpstreamRequests []UpstreamRequestMetric `json:"upstream_requests"`
	KeyExhaustions   []KeyExhaustionMetric   `json:"key_exhaustions"`
	KeySwitches      []KeySwitchMetric       `json:"key_switches"`
	ActiveSessions   int                     `json:"active_sessions"`
	ModelAliases     map[string]string       `json:"model_aliases,omitempty"`
}

func parsePromLabels(s string) map[string]string {
	labels := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		idx := strings.Index(part, "=")
		if idx <= 0 {
			continue
		}
		labels[part[:idx]] = strings.Trim(part[idx+1:], `"`)
	}
	return labels
}

func atoiDefault(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// Snapshot returns the metrics registry contents as structured JSON so the
// dashboard does not need to parse Prometheus exposition text. It contains
// only counters, latencies, and session counts — never API keys.
func (m *MetricsRegistry) Snapshot(km *KeyManager) MetricsSnapshot {
	snap := MetricsSnapshot{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		HTTPRequests:     []HTTPRequestMetric{},
		HTTPDurations:    []HTTPDurationMetric{},
		UpstreamRequests: []UpstreamRequestMetric{},
		KeyExhaustions:   []KeyExhaustionMetric{},
		KeySwitches:      []KeySwitchMetric{},
	}
	if m == nil {
		return snap
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	for k, count := range m.httpRequestsTotal {
		l := parsePromLabels(k)
		snap.HTTPRequests = append(snap.HTTPRequests, HTTPRequestMetric{
			Endpoint: l["endpoint"],
			Method:   l["method"],
			Status:   atoiDefault(l["status"]),
			Count:    count,
		})
	}
	for k, sum := range m.httpRequestDurationSum {
		l := parsePromLabels(k)
		snap.HTTPDurations = append(snap.HTTPDurations, HTTPDurationMetric{
			Endpoint:             l["endpoint"],
			Method:               l["method"],
			DurationSecondsSum:   sum,
			DurationSecondsCount: m.httpRequestDurationCount[k],
		})
	}
	for k, count := range m.upstreamRequestsTotal {
		l := parsePromLabels(k)
		durKey := fmt.Sprintf(`key_index="%s",priority="%s",status="%s"`, l["key_index"], l["priority"], l["status"])
		snap.UpstreamRequests = append(snap.UpstreamRequests, UpstreamRequestMetric{
			KeyIndex:             atoiDefault(l["key_index"]),
			Priority:             atoiDefault(l["priority"]),
			Status:               atoiDefault(l["status"]),
			Count:                count,
			DurationSecondsSum:   m.upstreamDurationSum[durKey],
			DurationSecondsCount: m.upstreamDurationCount[durKey],
		})
	}
	for k, count := range m.keyExhaustionsTotal {
		snap.KeyExhaustions = append(snap.KeyExhaustions, KeyExhaustionMetric{KeyIndex: k, Count: count})
	}
	for k, count := range m.keySwitchesTotal {
		l := parsePromLabels(k)
		snap.KeySwitches = append(snap.KeySwitches, KeySwitchMetric{
			FromKey: atoiDefault(l["from_key"]),
			ToKey:   atoiDefault(l["to_key"]),
			Reason:  l["reason"],
			Count:   count,
		})
	}
	if km != nil {
		usage := km.GetAggregatedUsage()
		snap.ActiveSessions = usage.Summary.ActiveSessions
	}
	return snap
}
