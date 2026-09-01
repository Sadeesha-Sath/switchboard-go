package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyManagerNoCurrentWhenExhausted(t *testing.T) {
	km := NewKeyManager([]string{"a"}, time.Hour)
	km.MarkExhausted(0)
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected no current key")
	}
}

func TestBearerTokenCaseInsensitive(t *testing.T) {
	if got := bearerToken("bearer abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := bearerToken("BEARER abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestIsQuota429(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))}
	if !isQuota429(resp) {
		t.Fatal("expected quota 429")
	}
}

func TestIsQuota429NotGenericRateLimit(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"try again","code":"rate_limit_exceeded"}}`))}
	if isQuota429(resp) {
		t.Fatal("expected generic rate_limit_exceeded not to count as quota")
	}
}

func TestIsQuota429RestoresBody(t *testing.T) {
	const body = `{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
	_ = isQuota429(resp)
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Fatalf("body not restored: %q", string(got))
	}
}

func TestIsQuota429AnthropicUsageLimit(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"rate_limit_error","message":"credit balance is too low"}}`))}
	if !isQuota429(resp) {
		t.Fatal("expected anthropic credit balance 429 to count as quota")
	}
}

func TestIsQuota429AnthropicGenericRateLimit(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"rate_limit_error","message":"rate limit reached"}}`))}
	if isQuota429(resp) {
		t.Fatal("expected generic anthropic rate limit not to count as quota")
	}
}

func TestRequestTooLargeReturns413(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, MaxRequestBodyBytes: 4})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("12345")))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRootOpenAIPathProxies(t *testing.T) {
	var gotPath, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"glm-5.1","messages":[]}`))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/chat/completions" || gotAuth != "Bearer u" {
		t.Fatalf("unexpected upstream path=%q auth=%q", gotPath, gotAuth)
	}
}

func TestDoUpstreamSetsDefaultUserAgent(t *testing.T) {
	var gotUA string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: upstream.URL})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	resp, err := app.doUpstream(req.Context(), req, nil, "u", APIStyleOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotUA != "OpenAI/Python 1.0.0" {
		t.Fatalf("got user-agent %q", gotUA)
	}
}

func TestDoUpstreamAnthropicSetsHeaders(t *testing.T) {
	var gotPath, gotKey, gotAuth, gotVersion, gotUA string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: upstream.URL})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"minimax-m3","messages":[]}`))
	req.Header.Set("x-api-key", "p")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := app.doUpstream(req.Context(), req, []byte(`{"model":"minimax-m3","messages":[]}`), "u", APIStyleAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotPath != "/messages" || gotKey != "u" || gotAuth != "" || gotVersion != "2023-06-01" || gotUA != "anthropic-sdk-go/1.0.0" {
		t.Fatalf("unexpected upstream request path=%q key=%q auth=%q version=%q ua=%q", gotPath, gotKey, gotAuth, gotVersion, gotUA)
	}
}

func TestProxyAnthropicMessagesCyclesKeys(t *testing.T) {
	var gotKeys []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			http.NotFound(w, r)
			return
		}
		gotKeys = append(gotKeys, r.Header.Get("x-api-key"))
		if r.Header.Get("x-api-key") == "bad" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"usage limit exhausted"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","content":[]}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"bad", "good"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"minimax-m3","messages":[]}`))
	req.Header.Set("x-api-key", "p")
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Join(gotKeys, ",") != "bad,good" {
		t.Fatalf("unexpected keys: %v", gotKeys)
	}
}

func TestAuthFailureReturnsOpenAIJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("ct %q", ct)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] == nil {
		t.Fatal("missing error object")
	}
}

func TestAuthOKConstantTimeCompare(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 for correct bearer, got %d", rec.Code)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req2.Header.Set("Authorization", "Bearer wrong")
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong bearer, got %d", rec2.Code)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req3.Header.Set("x-api-key", "p")
	rec3 := httptest.NewRecorder()
	app.ServeHTTP(rec3, req3)
	if rec3.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 for correct x-api-key, got %d", rec3.Code)
	}
}

func TestAdminResetKeyUnExhausts(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"a", "b"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	app.keys.MarkExhausted(0)
	app.keys.MarkExhausted(1)
	body := bytes.NewReader([]byte(`{"index":0}`))
	req := httptest.NewRequest(http.MethodPost, "/admin/reset-key", body)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body %s", rec.Code, rec.Body.String())
	}
	var st StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Keys[0].State != string(KeyAvailable) {
		t.Fatalf("key 0 not available: %+v", st.Keys[0])
	}
	if st.Keys[1].State != string(KeyExhausted) {
		t.Fatalf("key 1 should still be exhausted: %+v", st.Keys[1])
	}
}

func TestAdminResetKeyRejectsBadIndex(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"a"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	body := bytes.NewReader([]byte(`{"index":99}`))
	req := httptest.NewRequest(http.MethodPost, "/admin/reset-key", body)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestAdminResetAllKeys(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"a", "b", "c"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	for i := range app.config.UpstreamAPIKeys {
		app.keys.MarkExhausted(i)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/reset-all-keys", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var st StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	for i, k := range st.Keys {
		if k.State != string(KeyAvailable) {
			t.Fatalf("key %d not available: %+v", i, k)
		}
	}
}
func TestAuthFailureReturnsAnthropicJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	errObj, ok := payload["error"].(map[string]any)
	if payload["type"] != "error" || !ok || errObj["type"] != "authentication_error" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRootAnthropicAuthFailureReturnsAnthropicJSON(t *testing.T) {
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://example.com", MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/messages", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	errObj, ok := payload["error"].(map[string]any)
	if payload["type"] != "error" || !ok || errObj["type"] != "authentication_error" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestValidateConfigHelper(t *testing.T) {
	if err := validateConfig(Config{}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateConfig(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"u"}, UpstreamBaseURL: "http://x", MaxRequestBodyBytes: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateKeysEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") != "OpenAI/Python 1.0.0" {
			t.Fatalf("unexpected user-agent %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Authorization") == "Bearer good" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
	}))
	defer upstream.Close()
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"good", "bad"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodPost, "/admin/validate-keys", nil)
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var out ValidateKeysResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("got %d results", len(out.Results))
	}
	if out.Results[0].State != string(KeyAvailable) || out.Results[1].State != string(KeyExhausted) {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
}

func TestReadyzUnauthenticated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"good"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestLoadConfigFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`server:
  listen_addr: "127.0.0.1:9090"
  proxy_api_key: "yaml-proxy"
upstream:
  base_url: "https://example.com/v1"
  api_keys: ["k1", "k2"]
smtp:
  host: "smtp.example.com"
  port: 587
  tls: false
  starttls: true
limits:
  max_request_body_bytes: 1234
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	t.Setenv("PROXY_API_KEY", "env-proxy")
	t.Setenv("OPENCODE_GO_API_KEYS", "env1,env2")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:9090" || cfg.ProxyAPIKey != "env-proxy" || cfg.UpstreamBaseURL != "https://example.com/v1" || cfg.MaxRequestBodyBytes != 1234 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoadConfigExplicitInvalidPathErrors(t *testing.T) {
	t.Setenv("SWITCHBOARD_GO_CONFIG", "/does/not/exist.yaml")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigExplicitInvalidYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`server: {listen_addr: "127.0.0.1:1", proxy_api_key: "yaml"}
upstream: {base_url: "https://yaml", api_keys: ["yaml1"]}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	t.Setenv("PROXY_API_KEY", "env")
	t.Setenv("OPENCODE_GO_API_KEYS", "e1,e2")
	t.Setenv("LISTEN_ADDR", "0.0.0.0:9999")
	t.Setenv("MAX_REQUEST_BODY_BYTES", "99")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "0.0.0.0:9999" || cfg.ProxyAPIKey != "env" || len(cfg.UpstreamAPIKeys) != 2 || cfg.MaxRequestBodyBytes != 99 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestKeyManagerRetryEligibleAfterCooldown(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	km := NewKeyManager([]string{"a"}, time.Minute)
	km.now = func() time.Time { return now }
	km.MarkExhausted(0)
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected no eligible key immediately after exhaustion")
	}
	now = now.Add(59 * time.Second)
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected key still in cooldown at 59s")
	}
	now = now.Add(2 * time.Second) // 61s since exhaustion
	idx, key, ok := km.Current()
	if !ok || idx != 0 || key != "a" {
		t.Fatalf("expected key eligible after cooldown, got idx=%d key=%q ok=%v", idx, key, ok)
	}
}

func TestKeyManagerZeroCooldownImmediatelyEligible(t *testing.T) {
	km := NewKeyManager([]string{"a"}, 0)
	km.MarkExhausted(0)
	idx, key, ok := km.Current()
	if !ok || idx != 0 || key != "a" {
		t.Fatalf("expected exhausted key immediately eligible with zero cooldown, got ok=%v", ok)
	}
}

func TestKeyManagerRetryAfterSeconds(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	km := NewKeyManager([]string{"a", "b"}, 2*time.Minute)
	km.now = func() time.Time { return now }
	km.MarkExhausted(0) // eligible at t=2120
	now = now.Add(30 * time.Second)
	km.MarkExhausted(1) // eligible at t=2150
	secs, ok := km.RetryAfterSeconds()
	if !ok || secs != 90 { // soonest 2120 - now 2030
		t.Fatalf("expected retry-after 90s, got %d ok=%v", secs, ok)
	}

	km2 := NewKeyManager([]string{"a"}, 0)
	km2.MarkExhausted(0)
	if _, ok := km2.RetryAfterSeconds(); ok {
		t.Fatal("expected no retry-after hint when cooldown is disabled")
	}
}

func TestKeyManagerReArmsNotificationsOnRecovery(t *testing.T) {
	km := NewKeyManager([]string{"a", "b"}, time.Minute)
	km.MarkExhausted(0)
	km.MarkExhausted(1)
	if !km.ShouldNotifyAllExhausted() {
		t.Fatal("expected first all-exhausted notification")
	}
	if km.ShouldNotifyAllExhausted() {
		t.Fatal("expected all-exhausted notification suppressed the second time")
	}
	if !km.ShouldNotifySwitch(0) {
		t.Fatal("expected switch notification for key 0")
	}
	// Recovery of a key must re-arm both the switch flag and the all-exhausted flag.
	km.MarkAvailable(0)
	if !km.ShouldNotifySwitch(0) {
		t.Fatal("expected switch notification re-armed after recovery")
	}
	km.MarkExhausted(0)
	if !km.ShouldNotifyAllExhausted() {
		t.Fatal("expected all-exhausted notification re-armed after a recovery round")
	}
}

func doProxyReq(app *App) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.1","messages":[]}`))
	req.Header.Set("Authorization", "Bearer p")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func TestProxyRetriesExhaustedKeyAfterCooldown(t *testing.T) {
	var replenished atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if replenished.Load() {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"k1"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024, RetryExhaustedAfter: time.Minute})
	now := time.Unix(5000, 0).UTC()
	app.keys.now = func() time.Time { return now }

	// First request: the only key quota-fails, so the proxy returns a local 429
	// carrying a Retry-After hint pointing at the next probe window.
	rec := doProxyReq(app)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body %s", rec.Code, rec.Body.String())
	}
	if ra := rec.Header().Get("Retry-After"); ra != "60" {
		t.Fatalf("expected Retry-After 60, got %q", ra)
	}

	// Within the cooldown the proxy fast-fails without probing upstream again.
	rec = doProxyReq(app)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 during cooldown, got %d", rec.Code)
	}

	// Cooldown elapses and the account is replenished: the next real request acts
	// as a probe and succeeds.
	now = now.Add(2 * time.Minute)
	replenished.Store(true)
	rec = doProxyReq(app)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after cooldown+replenish, got %d body %s", rec.Code, rec.Body.String())
	}
	st := app.keys.Status()
	if st.Keys[0].State != string(KeyAvailable) || !st.Keys[0].Eligible {
		t.Fatalf("expected key available and eligible after recovery, got %+v", st.Keys[0])
	}
	if st.RetryExhaustedAfterSeconds != 60 {
		t.Fatalf("expected retry_exhausted_after_seconds 60, got %d", st.RetryExhaustedAfterSeconds)
	}
}

func TestProxyZeroCooldownNoRetryAfterHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","code":"insufficient_quota"}}`))
	}))
	defer upstream.Close()

	app := newApp(Config{ProxyAPIKey: "p", UpstreamAPIKeys: []string{"k1"}, UpstreamBaseURL: upstream.URL, MaxRequestBodyBytes: 1024, RetryExhaustedAfter: 0})
	rec := doProxyReq(app)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "" {
		t.Fatalf("expected no Retry-After header with cooldown disabled, got %q", ra)
	}
}

func TestLoadConfigRetryExhaustedAfterParsedAndOverridden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"], retry_exhausted_after: \"90s\"}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetryExhaustedAfter != 90*time.Second {
		t.Fatalf("expected 90s from yaml, got %s", cfg.RetryExhaustedAfter)
	}
	t.Setenv("RETRY_EXHAUSTED_AFTER", "5m")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetryExhaustedAfter != 5*time.Minute {
		t.Fatalf("expected env override to 5m, got %s", cfg.RetryExhaustedAfter)
	}
}

func TestLoadConfigRetryExhaustedAfterDefaultAndExplicitZero(t *testing.T) {
	dir := t.TempDir()
	omitted := filepath.Join(dir, "omitted.yaml")
	if err := os.WriteFile(omitted, []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", omitted)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetryExhaustedAfter != 5*time.Minute {
		t.Fatalf("expected default 5m when omitted, got %s", cfg.RetryExhaustedAfter)
	}

	zero := filepath.Join(dir, "zero.yaml")
	if err := os.WriteFile(zero, []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"], retry_exhausted_after: \"0\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", zero)
	cfg, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetryExhaustedAfter != 0 {
		t.Fatalf("expected explicit 0 to disable cooldown, got %s", cfg.RetryExhaustedAfter)
	}
}

func TestLoadConfigRejectsInvalidRetryExhaustedAfterEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("server: {proxy_api_key: \"p\"}\nupstream: {base_url: \"https://x\", api_keys: [\"k\"]}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWITCHBOARD_GO_CONFIG", path)
	t.Setenv("RETRY_EXHAUSTED_AFTER", "not-a-duration")
	if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "RETRY_EXHAUSTED_AFTER") {
		t.Fatalf("expected RETRY_EXHAUSTED_AFTER validation error, got %v", err)
	}
}

func TestParseUpstreamUsage(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	// Format 1: Nested under "usage" with ISO8601 string
	raw1 := []byte(`{
		"usage": {
			"rolling": {"status": "ok", "percent": 45.5, "resetsAt": "2026-09-01T15:00:00Z"},
			"weekly": {"status": "ok", "percent": 60, "resetsAt": "2026-09-07T00:00:00Z"},
			"monthly": {"status": "ok", "percent": 25.0, "resetsAt": "2026-10-01T00:00:00Z"}
		}
	}`)
	u1, err := parseUpstreamUsage(raw1, now)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if u1.Rolling.Percent != 45.5 || u1.Rolling.ResetsAt.Hour() != 15 {
		t.Fatalf("unexpected rolling: %+v", u1.Rolling)
	}
	if u1.Weekly.Percent != 60 || u1.Weekly.ResetsAt.Day() != 7 {
		t.Fatalf("unexpected weekly: %+v", u1.Weekly)
	}

	// Format 2: Flat with resetsInSeconds and integer percent
	raw2 := []byte(`{
		"rolling": {"status": "warning", "percent": 96, "resetsInSeconds": 3600},
		"weekly": {"status": "ok", "percent": 40},
		"monthly": {"status": "ok", "percent": "20.5"}
	}`)
	u2, err := parseUpstreamUsage(raw2, now)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if u2.Rolling.Percent != 96 || u2.Rolling.Status != "warning" {
		t.Fatalf("unexpected rolling: %+v", u2.Rolling)
	}
	expectedReset := now.Add(1 * time.Hour)
	if !u2.Rolling.ResetsAt.Equal(expectedReset) {
		t.Fatalf("expected reset %v, got %v", expectedReset, u2.Rolling.ResetsAt)
	}
	if u2.Monthly.Percent != 20.5 {
		t.Fatalf("expected monthly 20.5, got %v", u2.Monthly.Percent)
	}
}

func TestSessionManager(t *testing.T) {
	fakeTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	sm := NewSessionManager(2 * time.Hour)
	sm.now = func() time.Time { return fakeTime }

	if _, ok := sm.GetKey("s1"); ok {
		t.Fatalf("expected non-existent session to return false")
	}

	sm.SetKey("s1", 2)
	k, ok := sm.GetKey("s1")
	if !ok || k != 2 {
		t.Fatalf("expected key 2, got %d (ok=%t)", k, ok)
	}
	if sm.ActiveCount() != 1 {
		t.Fatalf("expected active count 1, got %d", sm.ActiveCount())
	}

	// Advance time within TTL (1 hour)
	fakeTime = fakeTime.Add(1 * time.Hour)
	k, ok = sm.GetKey("s1")
	if !ok || k != 2 {
		t.Fatalf("expected session to remain active")
	}

	// Advance past 2h idle TTL without touch
	fakeTime = fakeTime.Add(2*time.Hour + 1*time.Minute)
	if _, ok := sm.GetKey("s1"); ok {
		t.Fatalf("expected session to expire after idle TTL")
	}
	if sm.ActiveCount() != 0 {
		t.Fatalf("expected active count 0, got %d", sm.ActiveCount())
	}

	// Test InvalidateKey
	sm.SetKey("s2", 1)
	sm.SetKey("s3", 1)
	sm.SetKey("s4", 0)
	sm.InvalidateKey(1)
	if _, ok := sm.GetKey("s2"); ok {
		t.Fatalf("expected s2 to be invalidated")
	}
	if _, ok := sm.GetKey("s3"); ok {
		t.Fatalf("expected s3 to be invalidated")
	}
	if k, ok := sm.GetKey("s4"); !ok || k != 0 {
		t.Fatalf("expected s4 to remain valid")
	}
}

func TestExtractSessionID(t *testing.T) {
	// From Header
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("x-session-id", "sess-123")
	if sid := extractSessionID(req, nil); sid != "sess-123" {
		t.Fatalf("expected sess-123, got %s", sid)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req2.Header.Set("x-conversation-id", "conv-456")
	if sid := extractSessionID(req2, nil); sid != "conv-456" {
		t.Fatalf("expected conv-456, got %s", sid)
	}

	// From JSON body user field
	req3 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	body3 := []byte(`{"model":"minimax-m3","user":"agent-user-789","messages":[{"role":"user","content":"hello"}]}`)
	if sid := extractSessionID(req3, body3); sid != "agent-user-789" {
		t.Fatalf("expected agent-user-789, got %s", sid)
	}

	// From JSON body prompt hash
	req4 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	body4 := []byte(`{"model":"minimax-m3","messages":[{"role":"user","content":"repeat prompt for hashing test"}]}`)
	sid4 := extractSessionID(req4, body4)
	if !strings.HasPrefix(sid4, "conv_") {
		t.Fatalf("expected conv_ prefix from hashed messages, got %s", sid4)
	}
	// Verify deterministic hashing
	sid4b := extractSessionID(req4, body4)
	if sid4 != sid4b {
		t.Fatalf("expected identical hash for identical message content")
	}
}

func TestRoutingStrategySessionSticky(t *testing.T) {
	km := NewKeyManagerWithConfig([]string{"key0", "key1", "key2"}, 5*time.Minute, "session_sticky", 2*time.Hour, 5*time.Minute, 95.0)

	// Set usage: key0 has 40% weekly, key1 has 10% weekly, key2 has 70% weekly
	km.UpdateUsage(0, &KeyUsage{Weekly: UsageWindow{Percent: 40}, Rolling: UsageWindow{Percent: 20}}, "")
	km.UpdateUsage(1, &KeyUsage{Weekly: UsageWindow{Percent: 10}, Rolling: UsageWindow{Percent: 20}}, "")
	km.UpdateUsage(2, &KeyUsage{Weekly: UsageWindow{Percent: 70}, Rolling: UsageWindow{Percent: 20}}, "")

	// New session should be assigned to lowest weekly usage (key1)
	idx, key, ok := km.KeyForRequest("sess-A")
	if !ok || idx != 1 || key != "key1" {
		t.Fatalf("expected sess-A assigned to key1 (lowest weekly), got index %d key %s", idx, key)
	}

	// Subsequent requests from same session MUST stick to key1
	idx, key, ok = km.KeyForRequest("sess-A")
	if !ok || idx != 1 || key != "key1" {
		t.Fatalf("expected sess-A to stick to key1, got index %d key %s", idx, key)
	}

	// Now key1 usage increases to 96% rolling (proactively saturated)
	km.UpdateUsage(1, &KeyUsage{Weekly: UsageWindow{Percent: 10}, Rolling: UsageWindow{Percent: 96}}, "")

	// Request from sess-A should now proactively switch away to the next best key (key0)
	idx, key, ok = km.KeyForRequest("sess-A")
	if !ok || idx != 0 || key != "key0" {
		t.Fatalf("expected sess-A to proactively switch to key0 (40%% weekly), got index %d key %s", idx, key)
	}
}

func TestRoutingStrategyBalanced(t *testing.T) {
	fakeTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	km := NewKeyManagerWithConfig([]string{"key0", "key1"}, 5*time.Minute, "balanced", 2*time.Hour, 1*time.Hour, 95.0)
	km.now = func() time.Time { return fakeTime }

	km.UpdateUsage(0, &KeyUsage{Weekly: UsageWindow{Percent: 20}, Rolling: UsageWindow{Percent: 10}}, "")
	km.UpdateUsage(1, &KeyUsage{Weekly: UsageWindow{Percent: 50}, Rolling: UsageWindow{Percent: 10}}, "")

	// Initial request picks key0 (lowest usage)
	idx, key, ok := km.KeyForRequest("")
	if !ok || idx != 0 || key != "key0" {
		t.Fatalf("expected key0, got %d %s", idx, key)
	}

	// Usage updates in background so key1 becomes lowest (5% weekly) while key0 is 80%
	km.UpdateUsage(0, &KeyUsage{Weekly: UsageWindow{Percent: 80}, Rolling: UsageWindow{Percent: 10}}, "")
	km.UpdateUsage(1, &KeyUsage{Weekly: UsageWindow{Percent: 5}, Rolling: UsageWindow{Percent: 10}}, "")

	// Request 30 minutes later (within balanced_idle_timeout of 1h) -> sticks to key0 to preserve upstream prompt cache
	fakeTime = fakeTime.Add(30 * time.Minute)
	idx, key, ok = km.KeyForRequest("")
	if !ok || idx != 0 || key != "key0" {
		t.Fatalf("expected to stick to key0 within idle window, got %d %s", idx, key)
	}

	// Request 65 minutes later (exceeding 1h balanced_idle_timeout) -> re-evaluates and switches to key1 (lowest usage)
	fakeTime = fakeTime.Add(65 * time.Minute)
	idx, key, ok = km.KeyForRequest("")
	if !ok || idx != 1 || key != "key1" {
		t.Fatalf("expected to switch to key1 after idle timeout, got %d %s", idx, key)
	}
}

func TestRoutingStrategyRoundRobin(t *testing.T) {
	km := NewKeyManagerWithConfig([]string{"key0", "key1", "key2"}, 5*time.Minute, "round_robin", 2*time.Hour, 5*time.Minute, 95.0)

	idx0, _, _ := km.KeyForRequest("")
	idx1, _, _ := km.KeyForRequest("")
	idx2, _, _ := km.KeyForRequest("")
	idx3, _, _ := km.KeyForRequest("")

	if idx0 != 0 || idx1 != 1 || idx2 != 2 || idx3 != 0 {
		t.Fatalf("expected sequential 0,1,2,0 round robin, got %d,%d,%d,%d", idx0, idx1, idx2, idx3)
	}
}

func TestRoutingStrategyFillFirst(t *testing.T) {
	km := NewKeyManagerWithConfig([]string{"key0", "key1"}, 5*time.Minute, "fill_first", 2*time.Hour, 5*time.Minute, 95.0)

	km.UpdateUsage(0, &KeyUsage{Weekly: UsageWindow{Percent: 50}, Rolling: UsageWindow{Percent: 50}}, "")
	km.UpdateUsage(1, &KeyUsage{Weekly: UsageWindow{Percent: 10}, Rolling: UsageWindow{Percent: 10}}, "")

	// Even though key1 has lower weekly usage, fill_first fills key0 first until saturated
	idx, _, _ := km.KeyForRequest("")
	if idx != 0 {
		t.Fatalf("expected fill_first to use key0, got %d", idx)
	}

	// When key0 hits >= 95% rolling, fill_first moves to key1
	km.UpdateUsage(0, &KeyUsage{Weekly: UsageWindow{Percent: 50}, Rolling: UsageWindow{Percent: 96}}, "")
	idx, _, _ = km.KeyForRequest("")
	if idx != 1 {
		t.Fatalf("expected fill_first to move to key1 when key0 saturated, got %d", idx)
	}
}

func TestDynamicResetsAtCooldownAndRecovery(t *testing.T) {
	fakeTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	km := NewKeyManagerWithConfig([]string{"key0", "key1"}, 5*time.Minute, "session_sticky", 2*time.Hour, 5*time.Minute, 95.0)
	km.now = func() time.Time { return fakeTime }

	resetTime := fakeTime.Add(30 * time.Minute)
	km.UpdateUsage(0, &KeyUsage{Rolling: UsageWindow{Percent: 99, ResetsAt: resetTime}}, "")
	km.MarkExhausted(0)

	// Key0 is exhausted and resetTime is in 30 minutes
	st := km.Status()
	if st.Keys[0].Eligible {
		t.Fatalf("expected key0 to not be eligible")
	}
	if st.Keys[0].RetryAfterSeconds != 1800 {
		t.Fatalf("expected 1800 seconds (30m) retry after, got %d", st.Keys[0].RetryAfterSeconds)
	}

	// Advance time past reset time (31 minutes)
	fakeTime = fakeTime.Add(31 * time.Minute)

	// Status should now show key0 is eligible for probe
	st = km.Status()
	if !st.Keys[0].Eligible {
		t.Fatalf("expected key0 to be eligible after resetTime reached")
	}

	// New usage check reports rolling usage has cleared to 10%
	km.UpdateUsage(0, &KeyUsage{Rolling: UsageWindow{Percent: 10, ResetsAt: fakeTime.Add(5 * time.Hour)}}, "")
	st = km.Status()
	if st.Keys[0].State != string(KeyAvailable) {
		t.Fatalf("expected key0 state to automatically recover to available, got %s", st.Keys[0].State)
	}
}

func TestAggregatedUsageEndpoint(t *testing.T) {
	reset0 := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	reset1 := time.Date(2026, 9, 1, 16, 0, 0, 0, time.UTC)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/usage" {
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Authorization") == "Bearer key0" {
				_, _ = w.Write([]byte(`{"rolling":{"status":"ok","percent":40.0,"resetsAt":"2026-09-01T14:00:00Z"},"weekly":{"status":"ok","percent":20.0},"monthly":{"status":"ok","percent":10.0}}`))
			} else {
				_, _ = w.Write([]byte(`{"rolling":{"status":"ok","percent":60.0,"resetsAt":"2026-09-01T16:00:00Z"},"weekly":{"status":"ok","percent":30.0},"monthly":{"status":"ok","percent":15.0}}`))
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	cfg := Config{
		ProxyAPIKey:              "test-proxy-key",
		UpstreamAPIKeys:          []string{"key0", "key1"},
		UpstreamBaseURL:          upstream.URL,
		MaxRequestBodyBytes:      1024,
		DisableUsagePolling:      true,
		RoutingStrategy:          "session_sticky",
		ProactiveSwitchThreshold: 95.0,
	}
	app := newApp(cfg)

	// 1. Unauthenticated request should fail with 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/usage", nil)
	recUnauth := httptest.NewRecorder()
	app.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauth, got %d", recUnauth.Code)
	}

	// 2. Authenticated request with refresh=true
	req := httptest.NewRequest(http.MethodGet, "/v1/usage?refresh=true", nil)
	req.Header.Set("Authorization", "Bearer test-proxy-key")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d body %s", rec.Code, rec.Body.String())
	}

	var resp AggregatedUsageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify standard OpenCode top-level pooled metrics
	if resp.Rolling.Percent != 50.0 { // (40 + 60) / 2
		t.Fatalf("expected pooled rolling 50.0%%, got %f", resp.Rolling.Percent)
	}
	if resp.Weekly.Percent != 25.0 { // (20 + 30) / 2
		t.Fatalf("expected pooled weekly 25.0%%, got %f", resp.Weekly.Percent)
	}
	if resp.Monthly.Percent != 12.5 { // (10 + 15) / 2
		t.Fatalf("expected pooled monthly 12.5%%, got %f", resp.Monthly.Percent)
	}
	if !resp.Rolling.ResetsAt.Equal(reset0) {
		t.Fatalf("expected earliest reset time %v, got %v", reset0, resp.Rolling.ResetsAt)
	}
	if !resp.Keys[1].Rolling.ResetsAt.Equal(reset1) {
		t.Fatalf("expected key 1 reset time %v, got %v", reset1, resp.Keys[1].Rolling.ResetsAt)
	}

	// Verify multi-subscription summary & telemetry
	if resp.Summary.TotalKeys != 2 || resp.Summary.AvailableKeys != 2 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if len(resp.Keys) != 2 {
		t.Fatalf("expected 2 keys telemetry, got %d", len(resp.Keys))
	}
	if resp.Keys[0].Rolling.Percent != 40.0 || resp.Keys[1].Rolling.Percent != 60.0 {
		t.Fatalf("unexpected per key rolling usage: %+v", resp.Keys)
	}
	if resp.Summary.PoolUsage.Rolling.TotalRemainingPercent != 100.0 { // 200 total - 100 used = 100 remaining
		t.Fatalf("expected 100%% total remaining rolling quota, got %f", resp.Summary.PoolUsage.Rolling.TotalRemainingPercent)
	}
}

func TestUsagePoller(t *testing.T) {
	var pollCount int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/usage" {
			atomic.AddInt32(&pollCount, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rolling":{"status":"ok","percent":25.0}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	cfg := Config{
		ProxyAPIKey:         "p",
		UpstreamAPIKeys:     []string{"k1"},
		UpstreamBaseURL:     upstream.URL,
		MaxRequestBodyBytes: 1024,
		UsageCheckInterval:  20 * time.Millisecond,
		DisableUsagePolling: false,
	}
	app := newApp(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	app.startUsagePoller(ctx)

	// Wait briefly for at least 2 poll ticks
	time.Sleep(70 * time.Millisecond)
	cancel()

	cnt := atomic.LoadInt32(&pollCount)
	if cnt < 2 {
		t.Fatalf("expected at least 2 polls, got %d", cnt)
	}

	st := app.keys.GetAggregatedUsage()
	if st.Rolling.Percent != 25.0 {
		t.Fatalf("expected rolling percent 25.0 updated by poller, got %f", st.Rolling.Percent)
	}
}

func TestConcurrentLoadAndRace(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/usage" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rolling":{"status":"ok","percent":30.0}}`))
			return
		}
		if r.URL.Path == "/chat/completions" || r.URL.Path == "/messages" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"ok"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	cfg := Config{
		ProxyAPIKey:              "test-key",
		UpstreamAPIKeys:          []string{"k1", "k2", "k3", "k4"},
		UpstreamBaseURL:          upstream.URL,
		MaxRequestBodyBytes:      1024 * 1024,
		RoutingStrategy:          "session_sticky",
		UsageCheckInterval:       10 * time.Millisecond,
		ProactiveSwitchThreshold: 95.0,
	}
	app := newApp(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.startUsagePoller(ctx)

	var wg sync.WaitGroup
	workers := 25
	requestsPerWorker := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				// Mix of chat completions, usage queries, and status queries
				if j%3 == 0 {
					req := httptest.NewRequest(http.MethodGet, "/usage", nil)
					req.Header.Set("Authorization", "Bearer test-key")
					rec := httptest.NewRecorder()
					app.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Errorf("worker %d: /usage failed with %d", workerID, rec.Code)
					}
				} else {
					body := []byte(`{"model":"gpt-4o","user":"user-` + string(rune('A'+workerID)) + `","messages":[{"role":"user","content":"test"}]}`)
					req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
					req.Header.Set("Authorization", "Bearer test-key")
					rec := httptest.NewRecorder()
					app.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Errorf("worker %d: /chat/completions failed with %d", workerID, rec.Code)
					}
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestSSEFlushing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: chunk1\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("data: chunk2\n\n"))
	}))
	defer upstream.Close()

	cfg := Config{
		ProxyAPIKey:         "test-key",
		UpstreamAPIKeys:     []string{"k1"},
		UpstreamBaseURL:     upstream.URL,
		MaxRequestBodyBytes: 1024,
		DisableUsagePolling: true,
	}
	app := newApp(cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "data: chunk1") || !strings.Contains(rec.Body.String(), "data: chunk2") {
		t.Fatalf("expected chunks in response, got %s", rec.Body.String())
	}
}

func TestKeyPriorityTierFallback(t *testing.T) {
	configs := []UpstreamKeyConfig{
		{Key: "primary-1", Priority: 1, Weight: 1},
		{Key: "primary-2", Priority: 1, Weight: 1},
		{Key: "backup-1", Priority: 2, Weight: 1},
	}
	km := NewKeyManagerWithKeyConfigs(configs, 5*time.Minute, "session_sticky", 2*time.Hour, 1*time.Hour, 95.0)

	// Set usage: primary-1 has 30% weekly, primary-2 has 40% weekly, backup-1 has 5% weekly
	km.UpdateUsage(0, &KeyUsage{Weekly: UsageWindow{Percent: 30}, Rolling: UsageWindow{Percent: 10}}, "")
	km.UpdateUsage(1, &KeyUsage{Weekly: UsageWindow{Percent: 40}, Rolling: UsageWindow{Percent: 10}}, "")
	km.UpdateUsage(2, &KeyUsage{Weekly: UsageWindow{Percent: 5}, Rolling: UsageWindow{Percent: 10}}, "")

	// Even though backup-1 has lower weekly usage (5%), primary tier (priority 1) MUST be preferred
	idx, key, ok := km.KeyForRequest("session-1")
	if !ok || idx != 0 || key != "primary-1" {
		t.Fatalf("expected primary-1 to be selected from priority 1 tier, got %d %s", idx, key)
	}

	// Saturated primary-1 (>= 95% rolling) -> falls back to primary-2 (still priority 1)
	km.UpdateUsage(0, &KeyUsage{Weekly: UsageWindow{Percent: 30}, Rolling: UsageWindow{Percent: 96}}, "")
	idx, key, ok = km.KeyForRequest("session-2")
	if !ok || idx != 1 || key != "primary-2" {
		t.Fatalf("expected primary-2 from priority 1 tier, got %d %s", idx, key)
	}

	// Saturated primary-2 as well -> now all priority 1 keys are saturated, so fallback to backup-1 (priority 2)
	km.UpdateUsage(1, &KeyUsage{Weekly: UsageWindow{Percent: 40}, Rolling: UsageWindow{Percent: 96}}, "")
	idx, key, ok = km.KeyForRequest("session-3")
	if !ok || idx != 2 || key != "backup-1" {
		t.Fatalf("expected fallback to backup-1 (priority 2 tier), got %d %s", idx, key)
	}
}

func TestWeightedTrafficDistribution(t *testing.T) {
	configs := []UpstreamKeyConfig{
		{Key: "key-w3", Priority: 1, Weight: 3},
		{Key: "key-w1", Priority: 1, Weight: 1},
	}
	km := NewKeyManagerWithKeyConfigs(configs, 5*time.Minute, "round_robin", 2*time.Hour, 1*time.Hour, 95.0)

	counts := make(map[int]int)
	// Make 4 requests
	for i := 0; i < 4; i++ {
		idx, _, ok := km.KeyForRequest("")
		if !ok {
			t.Fatalf("expected request %d to succeed", i)
		}
		counts[idx]++
	}

	if counts[0] != 3 || counts[1] != 1 {
		t.Fatalf("expected 3 requests for key-w3 and 1 for key-w1, got %+v", counts)
	}
}

func TestLoadConfigWithKeyConfigsYAML(t *testing.T) {
	yamlContent := `
server:
  listen_addr: "127.0.0.1:9090"
  proxy_api_key: "test-proxy-key"

upstream:
  base_url: "https://opencode.ai/zen/go/v1"
  api_keys:
    - key: "sk-pri"
      priority: 1
      weight: 3
    - key: "sk-sec"
      priority: 2
      weight: 1
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write tmp config: %v", err)
	}

	t.Setenv("SWITCHBOARD_GO_CONFIG", tmpFile)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.UpstreamKeyConfigs) != 2 {
		t.Fatalf("expected 2 key configs, got %d", len(cfg.UpstreamKeyConfigs))
	}
	if cfg.UpstreamKeyConfigs[0].Key != "sk-pri" || cfg.UpstreamKeyConfigs[0].Priority != 1 || cfg.UpstreamKeyConfigs[0].Weight != 3 {
		t.Fatalf("unexpected key config 0: %+v", cfg.UpstreamKeyConfigs[0])
	}
	if cfg.UpstreamKeyConfigs[1].Key != "sk-sec" || cfg.UpstreamKeyConfigs[1].Priority != 2 || cfg.UpstreamKeyConfigs[1].Weight != 1 {
		t.Fatalf("unexpected key config 1: %+v", cfg.UpstreamKeyConfigs[1])
	}
}
