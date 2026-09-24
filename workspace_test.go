package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newConsoleTestServer serves the two console endpoints the client uses.
// Requests must carry "Bearer test-key", "Accept: application/json", and for
// the usage endpoint "pageSize=100". An unknown `since` value fails the
// request, so a wrong window mapping surfaces as a test failure.
func newConsoleTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	resetsAt := time.Now().Add(90 * time.Minute).UTC().Format(time.RFC3339)
	endsAt := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	mux := http.NewServeMux()
	mux.HandleFunc("/console/api/go/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			http.Error(w, "expected Accept: application/json, got "+r.Header.Get("Accept"), http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"_tag":"Unauthorized"}`)
			return
		}
		fmt.Fprintf(w, `{"product":"go","access":{"startsAt":"2026-08-26T15:52:58.000Z","endsAt":%q,"meters":{
			"fiveHour":{"startsAt":"2026-09-24T04:59:21.277Z","resetsAt":%q,"limitMicroCents":"1200000000","usedMicroCents":"84289897"},
			"week":{"startsAt":"2026-09-21T00:00:00.000Z","resetsAt":%q,"limitMicroCents":"3000000000","usedMicroCents":"1013236711"},
			"month":{"limitMicroCents":"6000000000","usedMicroCents":"5084232386"}}}}`, endsAt, resetsAt, resetsAt)
	})
	mux.HandleFunc("/console/api/usage/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			http.Error(w, "expected Accept: application/json, got "+r.Header.Get("Accept"), http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("pageSize") != "100" {
			http.Error(w, "expected pageSize=100, got "+r.URL.Query().Get("pageSize"), http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("since") {
		case "2026-09-24T04:59:21.277Z":
			fmt.Fprint(w, `{"items":[{"model":"deepseek-v4.1-flash","totalCostMicroCents":"105967287"}],"pageInfo":{"page":1,"pageCount":1}}`)
		case "2026-09-21T00:00:00.000Z":
			fmt.Fprint(w, `{"items":[{"model":"deepseek-v4.1-flash","totalCostMicroCents":"579970673"},{"model":"muse-spark-1.3-contributor","totalCostMicroCents":"56639134"}],"pageInfo":{"page":1,"pageCount":1}}`)
		case "2026-08-26T15:52:58.000Z":
			fmt.Fprint(w, `{"items":[{"model":"deepseek-v4.1-flash","totalCostMicroCents":"546483151"}],"pageInfo":{"page":1,"pageCount":1}}`)
		default:
			http.Error(w, "unexpected since "+r.URL.Query().Get("since"), http.StatusBadRequest)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestWorkspaceClientRefreshAndSnapshot(t *testing.T) {
	srv := newConsoleTestServer(t)
	c := NewWorkspaceUsageClient(srv.URL, "test-key")
	if !c.Enabled() {
		t.Fatal("client should be enabled")
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	snap := c.Snapshot()
	if !snap.Enabled || snap.UpdatedAt == "" {
		t.Fatalf("snapshot not populated: %+v", snap)
	}
	if len(snap.Workspaces) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(snap.Workspaces))
	}
	ws := snap.Workspaces[0]
	if ws.ID != "opencode-go" || ws.Name != "OpenCode Go" || ws.Error != "" {
		t.Fatalf("workspace = %+v", ws)
	}
	roll, ok := ws.Windows["rolling"]
	if !ok {
		t.Fatal("rolling window missing")
	}
	if roll.Status != "ok" || roll.UsagePercent != 7.0 {
		t.Fatalf("rolling = %+v", roll)
	}
	if roll.UsageUSD != 84289897.0/1e8 || roll.LimitUSD != 12.0 {
		t.Fatalf("rolling USD = %v/%v", roll.UsageUSD, roll.LimitUSD)
	}
	if roll.ResetInSec < 85*60 || roll.ResetInSec > 90*60 {
		t.Fatalf("rolling reset = %v, want about 90m", roll.ResetInSec)
	}
	if len(roll.Rows) != 1 || roll.Rows[0].Model != "deepseek-v4.1-flash" {
		t.Fatalf("rolling rows = %+v", roll.Rows)
	}
	if roll.Rows[0].Cost != 105967287.0/1e8 || roll.Rows[0].QuotaCost != roll.Rows[0].Cost {
		t.Fatalf("row costs = %+v", roll.Rows[0])
	}
	if roll.Rows[0].Multiplier != nil || roll.Rows[0].Estimated {
		t.Fatalf("row flags = %+v", roll.Rows[0])
	}
	if roll.Rows[0].ContributionPercent != 100.0 {
		t.Fatalf("contribution = %v, want 100", roll.Rows[0].ContributionPercent)
	}
	week := ws.Windows["weekly"]
	if len(week.Rows) != 2 || week.Rows[0].Model != "deepseek-v4.1-flash" || week.Rows[1].Model != "muse-spark-1.3-contributor" {
		t.Fatalf("weekly rows = %+v", week.Rows)
	}
	if week.Rows[0].ContributionPercent != 91.1 || week.Rows[1].ContributionPercent != 8.9 {
		t.Fatalf("weekly contributions = %v/%v", week.Rows[0].ContributionPercent, week.Rows[1].ContributionPercent)
	}
	month := ws.Windows["monthly"]
	if month.ResetInSec <= 0 {
		t.Fatalf("monthly reset should come from access.endsAt: %v", month.ResetInSec)
	}
	if len(month.Rows) != 1 || month.Rows[0].Cost != 546483151.0/1e8 {
		t.Fatalf("monthly rows = %+v", month.Rows)
	}
}

func TestWorkspaceClientUnauthorized(t *testing.T) {
	const want = "service API key rejected (expired or revoked)"
	cases := []struct {
		name      string
		newServer func(*testing.T) *httptest.Server
	}{
		{name: "401", newServer: newConsoleTestServer},
		{
			name: "403",
			newServer: func(t *testing.T) *httptest.Server {
				t.Helper()
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}))
				t.Cleanup(srv.Close)
				return srv
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tc.newServer(t)
			c := NewWorkspaceUsageClient(srv.URL, "wrong-key")
			err := c.Refresh(context.Background())
			if err == nil || err.Error() != want {
				t.Fatalf("Refresh error = %v, want %q", err, want)
			}
			snap := c.Snapshot()
			if snap.Error != want {
				t.Fatalf("snapshot error = %q", snap.Error)
			}
			if !snap.Enabled {
				t.Fatalf("snapshot should stay enabled: %+v", snap)
			}
			if len(snap.Workspaces) != 1 {
				t.Fatalf("workspaces = %d, want 1", len(snap.Workspaces))
			}
			ws := snap.Workspaces[0]
			if ws.ID != "opencode-go" || ws.Name != "OpenCode Go" || ws.Error != want {
				t.Fatalf("workspace = %+v", ws)
			}
		})
	}
}

func TestWorkspaceClientNoSubscription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"product":"go"}`)
	}))
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-key")
	err := c.Refresh(context.Background())
	if err == nil || err.Error() != "no active Go subscription for this service key" {
		t.Fatalf("Refresh error = %v", err)
	}
	snap := c.Snapshot()
	if len(snap.Workspaces) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(snap.Workspaces))
	}
	ws := snap.Workspaces[0]
	if ws.ID != "opencode-go" || ws.Name != "OpenCode Go" || ws.Error != "no active Go subscription for this service key" {
		t.Fatalf("workspace = %+v", ws)
	}
}

func TestWorkspaceClientPartialMeters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/api/go/status":
			fmt.Fprint(w, `{"product":"go","access":{"startsAt":"2026-08-26T15:52:58.000Z","endsAt":"2026-09-26T15:52:58.000Z","meters":{"fiveHour":{"startsAt":"2026-09-24T04:59:21.277Z","resetsAt":"2026-09-24T09:59:21.277Z","limitMicroCents":"1200000000","usedMicroCents":"84289897"}}}}`)
		case "/console/api/usage/models":
			fmt.Fprint(w, `{"items":[],"pageInfo":{"page":1,"pageCount":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-key")
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	ws := c.Snapshot().Workspaces[0]
	if _, ok := ws.Windows["rolling"]; !ok {
		t.Fatal("rolling window missing")
	}
	if _, ok := ws.Windows["weekly"]; ok {
		t.Fatal("weekly window should be omitted when the meter is absent")
	}
	if _, ok := ws.Windows["monthly"]; ok {
		t.Fatal("monthly window should be omitted when the meter is absent")
	}
}

func TestWorkspaceClientPagination(t *testing.T) {
	var requests atomic.Int32
	var mu sync.Mutex
	var pages []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/api/go/status":
			if r.Header.Get("Accept") != "application/json" {
				http.Error(w, "expected Accept: application/json, got "+r.Header.Get("Accept"), http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"product":"go","access":{"startsAt":"2026-08-26T15:52:58.000Z","endsAt":"2026-09-26T15:52:58.000Z","meters":{"fiveHour":{"startsAt":"2026-09-24T04:59:21.277Z","limitMicroCents":"1200000000","usedMicroCents":"100000000"}}}}`)
		case "/console/api/usage/models":
			if r.Header.Get("Accept") != "application/json" {
				http.Error(w, "expected Accept: application/json, got "+r.Header.Get("Accept"), http.StatusBadRequest)
				return
			}
			if r.URL.Query().Get("pageSize") != "100" {
				http.Error(w, "expected pageSize=100, got "+r.URL.Query().Get("pageSize"), http.StatusBadRequest)
				return
			}
			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			if err != nil {
				http.Error(w, "bad page: "+r.URL.Query().Get("page"), http.StatusBadRequest)
				return
			}
			requests.Add(1)
			mu.Lock()
			pages = append(pages, page)
			mu.Unlock()
			fmt.Fprintf(w, `{"items":[{"model":"model-%d","totalCostMicroCents":"10000000"}],"pageInfo":{"page":%d,"pageCount":50}}`, page, page)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-key")
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := requests.Load(); got != maxModelPages {
		t.Fatalf("model page requests = %d, want %d", got, maxModelPages)
	}
	mu.Lock()
	gotPages := append([]int(nil), pages...)
	mu.Unlock()
	if len(gotPages) != maxModelPages {
		t.Fatalf("pages = %v, want 1..%d", gotPages, maxModelPages)
	}
	for i, p := range gotPages {
		if p != i+1 {
			t.Fatalf("pages = %v, want 1..%d in order", gotPages, maxModelPages)
		}
	}
	rows := c.Snapshot().Workspaces[0].Windows["rolling"].Rows
	if len(rows) != maxModelPages {
		t.Fatalf("rows = %d, want %d", len(rows), maxModelPages)
	}
}

func TestWorkspaceClientZeroCostRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/api/go/status":
			fmt.Fprint(w, `{"product":"go","access":{"startsAt":"2026-08-26T15:52:58.000Z","endsAt":"2026-09-26T15:52:58.000Z","meters":{"fiveHour":{"startsAt":"2026-09-24T04:59:21.277Z","limitMicroCents":"0","usedMicroCents":"0"}}}}`)
		case "/console/api/usage/models":
			fmt.Fprint(w, `{"items":[{"model":"free-model","totalCostMicroCents":"0"}],"pageInfo":{"page":1,"pageCount":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-key")
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	win := c.Snapshot().Workspaces[0].Windows["rolling"]
	if win.UsagePercent != 0 || win.LimitUSD != 0 {
		t.Fatalf("zero limit window = %+v", win)
	}
	if len(win.Rows) != 1 || win.Rows[0].ContributionPercent != 0 {
		t.Fatalf("rows = %+v", win.Rows)
	}
}

func TestWorkspaceClientWindowFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/api/go/status":
			fmt.Fprint(w, `{"product":"go","access":{"startsAt":"2026-08-26T15:52:58.000Z","endsAt":"2026-09-26T15:52:58.000Z","meters":{"fiveHour":{"startsAt":"2026-09-24T04:59:21.277Z","limitMicroCents":"1200000000","usedMicroCents":"100000000"}}}}`)
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-key")
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("window failure should not fail the refresh: %v", err)
	}
	ws := c.Snapshot().Workspaces[0]
	if !strings.Contains(ws.Error, "models(rolling)") {
		t.Fatalf("workspace error = %q", ws.Error)
	}
	if len(ws.Windows) != 0 {
		t.Fatalf("windows = %+v, want none", ws.Windows)
	}
}

func TestWorkspaceClientMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"product":`)
	}))
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-key")
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
	snap := c.Snapshot()
	if snap.Error == "" {
		t.Fatal("expected snapshot error")
	}
	if len(snap.Workspaces) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(snap.Workspaces))
	}
	ws := snap.Workspaces[0]
	if ws.ID != "opencode-go" || ws.Name != "OpenCode Go" || ws.Error != snap.Error {
		t.Fatalf("workspace = %+v, want error %q", ws, snap.Error)
	}
}

func TestWorkspaceClientDisabled(t *testing.T) {
	c := NewWorkspaceUsageClient("", "")
	if c != nil {
		t.Fatalf("client = %+v, want nil without a key", c)
	}
	snap := c.Snapshot()
	if snap.Enabled {
		t.Fatalf("nil client should report disabled: %+v", snap)
	}
	if snap.Workspaces == nil {
		t.Fatal("workspaces should be an empty array, not null")
	}
}

// The poller starts even without a client so a config reload that installs one
// (for example after key rotation) is picked up on the next tick.
func TestWorkspaceUsagePollerPicksUpReloadedClient(t *testing.T) {
	cfg := defaultConfig()
	cfg.WorkspaceUsage.Interval = 20 * time.Millisecond
	app := newApp(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.startWorkspaceUsagePoller(ctx)
	if app.workspace.Load() != nil {
		t.Fatal("expected no client before the reload")
	}

	srv := newConsoleTestServer(t)
	app.workspace.Store(NewWorkspaceUsageClient(srv.URL, "test-key"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snap := app.workspace.Load().Snapshot(); len(snap.Workspaces) == 1 && snap.Workspaces[0].Error == "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("poller never refreshed the reloaded client")
}

func TestAdminWorkspaceUsageEndpoint(t *testing.T) {
	cfg := Config{
		ProxyAPIKey:         "test-proxy-key",
		UpstreamAPIKeys:     []string{testKey0},
		UpstreamBaseURL:     "http://127.0.0.1:1",
		MaxRequestBodyBytes: 1024,
		DisableUsagePolling: true,
	}
	app := newApp(cfg)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/workspace-usage", nil)
	app.serve(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/workspace-usage", nil)
	req.Header.Set("Authorization", "Bearer test-proxy-key")
	app.serve(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated = %d, want 200", rec.Code)
	}
	var snap WorkspaceUsageSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Enabled {
		t.Fatalf("feature should be disabled without a service key: %+v", snap)
	}
	if snap.Workspaces == nil {
		t.Fatalf("workspaces should be an empty array, not null")
	}
}
