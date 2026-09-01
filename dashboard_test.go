package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testKey0 = "sk-aaaaaaaaaaaaaaaa1234"
	testKey1 = "sk-bbbbbbbbbbbbbbbb5678"
)

func newDashboardTestApp(t *testing.T) *App {
	t.Helper()
	cfg := Config{
		ProxyAPIKey:         "test-proxy-key",
		UpstreamAPIKeys:     []string{testKey0, testKey1},
		UpstreamBaseURL:     "http://127.0.0.1:1",
		MaxRequestBodyBytes: 1024,
		DisableUsagePolling: true,
		ModelAliases:        map[string]string{"gpt-4o": "glm-5.1", "gpt-4o-mini": "minimax-m3"},
	}
	return newApp(cfg)
}

func TestMaskKeyHint(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"long key", "sk-5R6dJLpzrYZRG5SqJ1Qt0x7o1uyShjUocx2DHRQiHaAaDJILRMeCGFGaC93ez28B", "sk-5R6d…z28B"},
		{"long synthetic key", testKey0, "sk-aaaa…1234"},
		{"short key fully masked", "abc12", "…"},
		{"empty key", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskKeyHint(tc.in); got != tc.want {
				t.Fatalf("maskKeyHint(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStatusAndUsageIncludeMaskedKeyHint(t *testing.T) {
	app := newDashboardTestApp(t)

	st := app.keys.Status()
	if len(st.Keys) != 2 {
		t.Fatalf("expected 2 keys in status, got %d", len(st.Keys))
	}
	if st.Keys[0].KeyHint != "sk-aaaa…1234" {
		t.Fatalf("status key hint = %q, want %q", st.Keys[0].KeyHint, "sk-aaaa…1234")
	}

	usage := app.keys.GetAggregatedUsage()
	if usage.Keys[1].KeyHint != "sk-bbbb…5678" {
		t.Fatalf("usage key hint = %q, want %q", usage.Keys[1].KeyHint, "sk-bbbb…5678")
	}

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	req.Header.Set("Authorization", "Bearer test-proxy-key")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /usage, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "sk-aaaa…1234") {
		t.Fatalf("expected masked hint in /usage body, got %s", body)
	}
	if strings.Contains(body, testKey0) || strings.Contains(body, testKey1) {
		t.Fatalf("full upstream key leaked in /usage body")
	}
}

func TestDashboardRootRedirect(t *testing.T) {
	app := newDashboardTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 from /, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard/" {
		t.Fatalf("expected Location /dashboard/, got %q", loc)
	}
}

func TestDashboardShellServed(t *testing.T) {
	app := newDashboardTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/dashboard/" {
		t.Fatalf("expected 302 to /dashboard/, got %d %q", rec.Code, rec.Header().Get("Location"))
	}

	req2 := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for /dashboard/, got %d", rec2.Code)
	}
	if ct := rec2.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %q", ct)
	}
	if !strings.Contains(rec2.Body.String(), "Switchboard") {
		t.Fatalf("expected dashboard shell markup, got %s", rec2.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodGet, "/dashboard/favicon.svg", nil)
	rec3 := httptest.NewRecorder()
	app.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 for dashboard asset, got %d", rec3.Code)
	}
	if ct := rec3.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg") {
		t.Fatalf("expected image/svg content type, got %q", ct)
	}

	req4 := httptest.NewRequest(http.MethodGet, "/dashboard/assets/nope.js", nil)
	rec4 := httptest.NewRecorder()
	app.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset, got %d", rec4.Code)
	}
}

func TestDashboardMetricsJSON(t *testing.T) {
	app := newDashboardTestApp(t)

	app.metrics.RecordHTTPRequest("/v1/chat/completions", http.MethodPost, 200, 0.25)
	app.metrics.RecordUpstreamRequest(0, 1, 200, 0.5)
	app.metrics.RecordUpstreamRequest(1, 1, 429, 0.75)
	app.metrics.RecordKeyExhaustion(1)
	app.metrics.RecordKeySwitch(0, 1, "quota_429")

	req := httptest.NewRequest(http.MethodGet, "/dashboard/api/metrics.json", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for metrics.json, got %d body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var snap MetricsSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}

	var foundHTTP bool
	for _, h := range snap.HTTPRequests {
		if h.Endpoint == "/v1/chat/completions" && h.Method == http.MethodPost && h.Status == 200 && h.Count == 1 {
			foundHTTP = true
		}
	}
	if !foundHTTP {
		t.Fatalf("expected http request metric in snapshot, got %+v", snap.HTTPRequests)
	}

	if len(snap.UpstreamRequests) != 2 {
		t.Fatalf("expected 2 upstream metrics, got %+v", snap.UpstreamRequests)
	}
	byStatus := map[int]UpstreamRequestMetric{}
	for _, u := range snap.UpstreamRequests {
		byStatus[u.Status] = u
	}
	u200, ok := byStatus[200]
	if !ok || u200.KeyIndex != 0 || u200.Priority != 1 || u200.DurationSecondsCount != 1 {
		t.Fatalf("unexpected upstream 200 metric: %+v", snap.UpstreamRequests)
	}
	if u429, ok := byStatus[429]; !ok || u429.KeyIndex != 1 {
		t.Fatalf("unexpected upstream 429 metric: %+v", snap.UpstreamRequests)
	}

	var exhausted bool
	for _, e := range snap.KeyExhaustions {
		if e.KeyIndex == 1 && e.Count == 1 {
			exhausted = true
		}
	}
	if !exhausted {
		t.Fatalf("expected key exhaustion metric, got %+v", snap.KeyExhaustions)
	}

	var switched bool
	for _, s := range snap.KeySwitches {
		if s.FromKey == 0 && s.ToKey == 1 && s.Reason == "quota_429" && s.Count == 1 {
			switched = true
		}
	}
	if !switched {
		t.Fatalf("expected key switch metric, got %+v", snap.KeySwitches)
	}

	if snap.ActiveSessions != 0 {
		t.Fatalf("expected 0 active sessions, got %d", snap.ActiveSessions)
	}
	if snap.ModelAliases["gpt-4o"] != "glm-5.1" {
		t.Fatalf("expected model aliases in snapshot, got %+v", snap.ModelAliases)
	}

	body := rec.Body.String()
	if strings.Contains(body, testKey0) || strings.Contains(body, testKey1) {
		t.Fatalf("snapshot leaked upstream keys")
	}
}
