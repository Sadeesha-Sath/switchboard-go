# Workspace usage API rewrite implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the dead console scraper in `workspace.go` with a service-key client for the new OpenCode console JSON API, keeping the dashboard contract unchanged.

**Architecture:** `WorkspaceUsageClient` keeps its public API (`NewWorkspaceUsageClient`, `Enabled`, `Refresh`, `Snapshot`) and the snapshot schema. Each refresh calls `GET /console/api/go/status` for the three quota meters, then `GET /console/api/usage/models?since=...` per window for per-model rows, mapping fiveHour/week/month to rolling/weekly/monthly. The seroval parser and bundle-hash discovery are deleted.

**Tech Stack:** Go standard library only (`net/http`, `encoding/json`, `sort`, `math`), `httptest` for tests. No new module dependencies.

**Spec:** `docs/superpowers/specs/2026-09-24-workspace-usage-api-rewrite-design.md`

## Global Constraints

- Base URL default `https://opencode.ai`; paths `/console/api/go/status` and `/console/api/usage/models`.
- Every console call sends `Authorization: Bearer <service_api_key>` and `Accept: application/json`.
- Microcents per USD is 1e8.
- Window map: rolling to fiveHour, weekly to week, monthly to month.
- Page size 100, at most 10 pages per window.
- Keep the `GET /admin/workspace-usage` JSON schema unchanged.
- Work happens in a git worktree under `.worktrees/` created with the using-git-worktrees skill. All commands below run from the worktree root.
- Sign off every commit with `git commit -s`.
- Do not add Go module dependencies.

## Review Focus

These are the input classes most likely to break real use. Each has a test in Task 1.

1. A missing meter (for example no `week`) must omit that window and still refresh the others.
2. The month meter has no `startsAt` or `resetsAt`, so rows must use `access.startsAt` and the reset must use `access.endsAt`.
3. All-zero row costs must yield contribution 0, never NaN.
4. A rejected key (401 or 403) must produce exactly `service API key rejected (expired or revoked)` and leave the snapshot enabled.
5. A response without `access` must produce `no active Go subscription for this service key`, not a panic.

---

### Task 1: Rewrite the workspace usage client

**Files:**
- Rewrite: `workspace.go`
- Rewrite: `workspace_test.go`
- Delete: `seroval.go`, `seroval_test.go`
- Modify: `main.go` (the two `NewWorkspaceUsageClient` call sites)
- Modify: `docs/admin-api.md` (Workspace usage section)

**Interfaces:**
- Consumes: `Config.WorkspaceUsage.SessionCookie` still exists in this task (Task 2 renames it). The call sites pass it as the service key so the build stays green.
- Produces: `NewWorkspaceUsageClient(baseURL, serviceAPIKey string) *WorkspaceUsageClient`; methods `Enabled() bool`, `Refresh(ctx context.Context) error`, `Snapshot() WorkspaceUsageSnapshot`. Snapshot types stay as they are: `WorkspaceUsageSnapshot`, `WorkspaceStatus`, `WorkspaceWindowSnapshot`, `WorkspaceModelRow`. Constants `goStatusPath`, `usageModelsPath`, `modelsPageSize`, `maxModelPages`.

- [ ] **Step 1: Write the new test file**

Replace `workspace_test.go` with:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newConsoleTestServer serves the two console endpoints the client uses.
// Requests must carry "Bearer test-key". An unknown `since` value fails the
// request, so a wrong window mapping surfaces as a test failure.
func newConsoleTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	resetsAt := time.Now().Add(90 * time.Minute).UTC().Format(time.RFC3339)
	endsAt := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	mux := http.NewServeMux()
	mux.HandleFunc("/console/api/go/status", func(w http.ResponseWriter, r *http.Request) {
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
	srv := newConsoleTestServer(t)
	c := NewWorkspaceUsageClient(srv.URL, "wrong-key")
	err := c.Refresh(context.Background())
	if err == nil || !strings.Contains(err.Error(), "service API key rejected") {
		t.Fatalf("Refresh error = %v", err)
	}
	snap := c.Snapshot()
	if snap.Error != "service API key rejected (expired or revoked)" {
		t.Fatalf("snapshot error = %q", snap.Error)
	}
	if !snap.Enabled {
		t.Fatalf("snapshot should stay enabled: %+v", snap)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/api/go/status":
			fmt.Fprint(w, `{"product":"go","access":{"startsAt":"2026-08-26T15:52:58.000Z","endsAt":"2026-09-26T15:52:58.000Z","meters":{"fiveHour":{"startsAt":"2026-09-24T04:59:21.277Z","limitMicroCents":"1200000000","usedMicroCents":"100000000"}}}}`)
		case "/console/api/usage/models":
			requests.Add(1)
			fmt.Fprintf(w, `{"items":[{"model":"model-%s","totalCostMicroCents":"10000000"}],"pageInfo":{"page":1,"pageCount":50}}`, r.URL.Query().Get("page"))
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
	if c.Snapshot().Error == "" {
		t.Fatal("expected snapshot error")
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestWorkspaceClient|TestAdminWorkspaceUsage' -count=1`
Expected: build failure, because the new constructor takes two arguments and the client functions do not exist yet.

- [ ] **Step 3: Replace `workspace.go`**

Replace the whole file with:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultWorkspaceBaseURL = "https://opencode.ai"
	microCentsPerUSD        = 1e8
	goStatusPath            = "/console/api/go/status"
	usageModelsPath         = "/console/api/usage/models"
	modelsPageSize          = 100
	maxModelPages           = 10
)

var (
	workspaceWindows     = []string{"rolling", "weekly", "monthly"}
	workspaceWindowMeter = map[string]string{"rolling": "fiveHour", "weekly": "week", "monthly": "month"}
)

type WorkspaceModelRow struct {
	Model               string   `json:"model"`
	Name                string   `json:"name"`
	Cost                float64  `json:"cost"`
	QuotaCost           float64  `json:"quota_cost"`
	Multiplier          *float64 `json:"multiplier,omitempty"`
	ContributionPercent float64  `json:"contribution_percent"`
	Estimated           bool     `json:"estimated"`
}

type WorkspaceWindowSnapshot struct {
	Status       string              `json:"status"`
	UsageUSD     float64             `json:"usage_usd"`
	LimitUSD     float64             `json:"limit_usd"`
	UsagePercent float64             `json:"usage_percent"`
	ResetInSec   float64             `json:"reset_in_sec"`
	Rows         []WorkspaceModelRow `json:"rows"`
}

type WorkspaceStatus struct {
	ID      string                             `json:"id"`
	Name    string                             `json:"name"`
	Windows map[string]WorkspaceWindowSnapshot `json:"windows"`
	Error   string                             `json:"error,omitempty"`
}

type WorkspaceUsageSnapshot struct {
	Enabled    bool              `json:"enabled"`
	UpdatedAt  string            `json:"updated_at,omitempty"`
	Error      string            `json:"error,omitempty"`
	Workspaces []WorkspaceStatus `json:"workspaces"`
}

// flexFloat decodes a JSON number or a decimal string. The console returns
// microcent and token fields as strings.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("parse number %q: %w", s, err)
	}
	*f = flexFloat(v)
	return nil
}

type goMeter struct {
	StartsAt        string    `json:"startsAt"`
	ResetsAt        string    `json:"resetsAt"`
	LimitMicroCents flexFloat `json:"limitMicroCents"`
	UsedMicroCents  flexFloat `json:"usedMicroCents"`
}

type goStatusResponse struct {
	Product string `json:"product"`
	Access  *struct {
		StartsAt string             `json:"startsAt"`
		EndsAt   string             `json:"endsAt"`
		Meters   map[string]goMeter `json:"meters"`
	} `json:"access"`
}

type usageModelItem struct {
	Model               string    `json:"model"`
	TotalCostMicroCents flexFloat `json:"totalCostMicroCents"`
}

type usageModelsResponse struct {
	Items    []usageModelItem `json:"items"`
	PageInfo struct {
		PageCount int `json:"pageCount"`
	} `json:"pageInfo"`
}

type WorkspaceUsageClient struct {
	baseURL    string
	serviceKey string
	http       *http.Client

	mu       sync.Mutex
	snapshot WorkspaceUsageSnapshot
}

func NewWorkspaceUsageClient(baseURL, serviceAPIKey string) *WorkspaceUsageClient {
	if strings.TrimSpace(serviceAPIKey) == "" {
		return nil
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultWorkspaceBaseURL
	}
	return &WorkspaceUsageClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		serviceKey: serviceAPIKey,
		http:       &http.Client{Timeout: 15 * time.Second},
		snapshot:   WorkspaceUsageSnapshot{Enabled: true},
	}
}

func (w *WorkspaceUsageClient) Enabled() bool { return w != nil }

func (w *WorkspaceUsageClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	u := w.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.serviceKey)
	req.Header.Set("Accept", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("service API key rejected (expired or revoked)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned status %d", path, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func (w *WorkspaceUsageClient) fetchModelPage(ctx context.Context, since string, page int) ([]usageModelItem, int, error) {
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("pageSize", strconv.Itoa(modelsPageSize))
	var out usageModelsResponse
	if err := w.getJSON(ctx, usageModelsPath, q, &out); err != nil {
		return nil, 0, err
	}
	pageCount := out.PageInfo.PageCount
	if pageCount < 1 {
		pageCount = 1
	}
	return out.Items, pageCount, nil
}

func (w *WorkspaceUsageClient) fetchModelRows(ctx context.Context, since string) ([]WorkspaceModelRow, error) {
	rows := []WorkspaceModelRow{}
	for page := 1; page <= maxModelPages; page++ {
		items, pageCount, err := w.fetchModelPage(ctx, since, page)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			cost := float64(item.TotalCostMicroCents) / microCentsPerUSD
			rows = append(rows, WorkspaceModelRow{
				Model:     item.Model,
				Name:      item.Model,
				Cost:      cost,
				QuotaCost: cost,
			})
		}
		if page >= pageCount {
			break
		}
	}
	total := 0.0
	for _, row := range rows {
		total += row.Cost
	}
	for i := range rows {
		if total > 0 {
			rows[i].ContributionPercent = math.Round(rows[i].Cost/total*1000) / 10
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Cost > rows[j].Cost })
	return rows, nil
}

func (w *WorkspaceUsageClient) buildWindow(meter goMeter, resetAt string, rows []WorkspaceModelRow) WorkspaceWindowSnapshot {
	used := float64(meter.UsedMicroCents) / microCentsPerUSD
	limit := float64(meter.LimitMicroCents) / microCentsPerUSD
	percent := 0.0
	if limit > 0 {
		percent = math.Round(used/limit*1000) / 10
	}
	reset := 0.0
	if resetAt != "" {
		if t, err := time.Parse(time.RFC3339, resetAt); err == nil {
			if secs := time.Until(t).Seconds(); secs > 0 {
				reset = secs
			}
		}
	}
	return WorkspaceWindowSnapshot{
		Status:       "ok",
		UsageUSD:     used,
		LimitUSD:     limit,
		UsagePercent: percent,
		ResetInSec:   reset,
		Rows:         rows,
	}
}

// Refresh runs one console read cycle. Errors are recorded in the snapshot and
// the poller retries on the next tick.
func (w *WorkspaceUsageClient) Refresh(ctx context.Context) error {
	if w == nil {
		return nil
	}
	return w.refreshOnce(ctx)
}

func (w *WorkspaceUsageClient) refreshOnce(ctx context.Context) error {
	var status goStatusResponse
	if err := w.getJSON(ctx, goStatusPath, nil, &status); err != nil {
		w.setSnapshotError(err.Error())
		return err
	}
	if status.Access == nil {
		err := errors.New("no active Go subscription for this service key")
		w.setSnapshotError(err.Error())
		return err
	}
	ws := WorkspaceStatus{ID: "opencode-go", Name: "OpenCode Go", Windows: map[string]WorkspaceWindowSnapshot{}}
	for _, window := range workspaceWindows {
		meter, ok := status.Access.Meters[workspaceWindowMeter[window]]
		if !ok {
			continue
		}
		since := meter.StartsAt
		resetAt := meter.ResetsAt
		if window == "monthly" {
			if since == "" {
				since = status.Access.StartsAt
			}
			if resetAt == "" {
				resetAt = status.Access.EndsAt
			}
		}
		rows, err := w.fetchModelRows(ctx, since)
		if err != nil {
			ws.Error = fmt.Errorf("models(%s): %w", window, err).Error()
			break
		}
		ws.Windows[window] = w.buildWindow(meter, resetAt, rows)
	}
	w.mu.Lock()
	w.snapshot = WorkspaceUsageSnapshot{
		Enabled:    true,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		Workspaces: []WorkspaceStatus{ws},
	}
	w.mu.Unlock()
	return nil
}

func (w *WorkspaceUsageClient) setSnapshotError(msg string) {
	w.mu.Lock()
	w.snapshot.Error = msg
	w.mu.Unlock()
}

func (a *App) startWorkspaceUsagePoller(ctx context.Context) {
	ws := a.workspace.Load()
	interval := a.cfg().WorkspaceUsage.Interval
	if ws == nil || interval <= 0 {
		return
	}
	go ws.Refresh(ctx)
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ws.Refresh(ctx)
			}
		}
	}()
}

// Snapshot returns the latest read result. Safe on a nil receiver.
func (w *WorkspaceUsageClient) Snapshot() WorkspaceUsageSnapshot {
	if w == nil {
		return WorkspaceUsageSnapshot{Enabled: false, Workspaces: []WorkspaceStatus{}}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.snapshot
	if out.Workspaces == nil {
		out.Workspaces = []WorkspaceStatus{}
	}
	return out
}
```

- [ ] **Step 4: Delete the seroval files**

Run: `git rm seroval.go seroval_test.go`

- [ ] **Step 5: Update the main.go call sites**

In `newApp`, change:

```go
	app.workspace.Store(NewWorkspaceUsageClient(
		"",
		cfg.WorkspaceUsage.SessionCookie,
		cfg.WorkspaceUsage.WorkspaceIDs,
	))
```

to:

```go
	app.workspace.Store(NewWorkspaceUsageClient(
		"",
		cfg.WorkspaceUsage.SessionCookie,
	))
```

In the config reload path, change:

```go
		a.workspace.Store(NewWorkspaceUsageClient(
			"",
			newCfg.WorkspaceUsage.SessionCookie,
			newCfg.WorkspaceUsage.WorkspaceIDs,
		))
```

to:

```go
		a.workspace.Store(NewWorkspaceUsageClient(
			"",
			newCfg.WorkspaceUsage.SessionCookie,
		))
```

The `SessionCookie` name stays for this commit so the build is green; Task 2 renames it.

- [ ] **Step 6: Run the tests**

Run: `go test ./... -run 'TestWorkspaceClient|TestAdminWorkspaceUsage' -count=1`
Expected: PASS.

- [ ] **Step 7: Update the admin API docs**

In `docs/admin-api.md`, replace the whole `## Workspace usage` section (from the heading through the line ending `... for setup.`) with:

````markdown
## Workspace usage

`GET /admin/workspace-usage` returns the last per-model cost and quota breakdown read from the OpenCode console API. Requires `Authorization: Bearer <PROXY_API_KEY>`. When the feature is disabled (no `workspace_usage.service_api_key`), it returns `{"enabled": false, "workspaces": []}`.

```bash
curl http://127.0.0.1:8080/admin/workspace-usage \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Example response:

```json
{
  "enabled": true,
  "updated_at": "2026-09-24T06:30:00Z",
  "workspaces": [
    {
      "id": "opencode-go",
      "name": "OpenCode Go",
      "windows": {
        "monthly": {
          "status": "ok",
          "usage_usd": 50.84232386,
          "limit_usd": 60.0,
          "usage_percent": 84.7,
          "reset_in_sec": 172800,
          "rows": [
            {
              "model": "deepseek-v4.1-flash",
              "name": "deepseek-v4.1-flash",
              "cost": 5.46483151,
              "quota_cost": 5.46483151,
              "contribution_percent": 100.0,
              "estimated": false
            }
          ]
        }
      }
    }
  ]
}
```

One service key is bound to one workspace. Each workspace has up to three windows (`rolling`, `weekly`, `monthly`), each with `status`, `usage_usd`, `limit_usd`, `usage_percent`, `reset_in_sec`, and per-model `rows` (`model`, `name`, `cost`, `quota_cost`, `contribution_percent`, `estimated`). Window totals come from the Go subscription meters; rows come from usage records, so row sums can be lower than the window total. Rows no longer carry a `multiplier`, and `name` equals the model id. On failure the top-level `error` field is set and the dashboard shows an error. See [Configuration](configuration.md#workspace-usage-scraping) for setup.
````

Keep the existing `configuration.md#workspace-usage-scraping` anchor in this task; Task 2 renames it.

- [ ] **Step 8: Commit**

```bash
git add workspace.go workspace_test.go main.go docs/admin-api.md
git commit -s -m "refactor(workspace): read usage from the new console JSON API"
```

---

### Task 2: Rename the config to service_api_key

**Files:**
- Modify: `main.go` (config struct, YAML struct, loader, merge, env overrides, startup summary, call sites)
- Modify: `main_test.go` (config tests)
- Modify: `config.example.yaml`
- Modify: `README.md` (one line)
- Modify: `docs/configuration.md` (Workspace usage section)
- Modify: `docs/admin-api.md` (anchor link only)

**Interfaces:**
- Consumes: `NewWorkspaceUsageClient(baseURL, serviceAPIKey string)` from Task 1.
- Produces: `WorkspaceUsageConfig{ServiceAPIKey string; Interval time.Duration}`; YAML field `workspace_usage.service_api_key`; env override `OPENCODE_SERVICE_API_KEY`.

- [ ] **Step 1: Update the config tests**

In `main_test.go`, replace `TestWorkspaceUsageConfigParsing` with:

```go
func TestWorkspaceUsageConfigParsing(t *testing.T) {
	yaml := `
server:
  proxy_api_key: "k"
  dashboard_auto_key: "false"
upstream:
  api_keys: ["sk-1"]
workspace_usage:
  service_api_key: "oc_sk_test"
  interval: "90s"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadYAMLConfig(path)
	if err != nil {
		t.Fatalf("loadYAMLConfig: %v", err)
	}
	if cfg.WorkspaceUsage.ServiceAPIKey != "oc_sk_test" {
		t.Fatalf("service key = %q", cfg.WorkspaceUsage.ServiceAPIKey)
	}
	if cfg.WorkspaceUsage.Interval != 90*time.Second {
		t.Fatalf("interval = %v", cfg.WorkspaceUsage.Interval)
	}
	if cfg.DashboardAutoKey != "false" {
		t.Fatalf("dashboard_auto_key = %q", cfg.DashboardAutoKey)
	}
}
```

Replace `TestWorkspaceUsageEnvOverrides` with:

```go
func TestWorkspaceUsageEnvOverrides(t *testing.T) {
	base := defaultConfig()
	t.Setenv("OPENCODE_SERVICE_API_KEY", "oc_sk_env")
	t.Setenv("OPENCODE_SESSION_COOKIE", "ignored")
	t.Setenv("WORKSPACE_USAGE_INTERVAL", "5s")
	t.Setenv("DASHBOARD_AUTO_KEY", "true")
	applyEnvOverrides(&base)
	if base.WorkspaceUsage.ServiceAPIKey != "oc_sk_env" {
		t.Fatalf("service key = %q", base.WorkspaceUsage.ServiceAPIKey)
	}
	if base.WorkspaceUsage.Interval != 5*time.Second {
		t.Fatalf("interval = %v", base.WorkspaceUsage.Interval)
	}
	if base.DashboardAutoKey != "true" {
		t.Fatalf("dashboard auto key = %q", base.DashboardAutoKey)
	}
}
```

Add a new test after `TestWorkspaceUsageEnvOverrides`:

```go
func TestWorkspaceUsageLegacySessionCookieIgnored(t *testing.T) {
	yaml := `
server:
  proxy_api_key: "k"
upstream:
  api_keys: ["sk-1"]
workspace_usage:
  session_cookie: "Fe26.2**old"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	cfg, err := loadYAMLConfig(path)
	if err != nil {
		t.Fatalf("loadYAMLConfig: %v", err)
	}
	if cfg.WorkspaceUsage.ServiceAPIKey != "" {
		t.Fatalf("legacy cookie must not become a service key, got %q", cfg.WorkspaceUsage.ServiceAPIKey)
	}
	if !strings.Contains(buf.String(), "session_cookie is no longer supported") {
		t.Fatalf("expected a deprecation warning, got %q", buf.String())
	}
}
```

Add `bytes` and `log` to the `main_test.go` import block if they are missing.

- [ ] **Step 2: Run the config tests to verify they fail**

Run: `go test ./... -run 'TestWorkspaceUsage' -count=1`
Expected: build failure, because `ServiceAPIKey` does not exist yet.

- [ ] **Step 3: Update main.go**

Replace the `WorkspaceUsageConfig` struct:

```go
type WorkspaceUsageConfig struct {
	SessionCookie string
	WorkspaceIDs  []string
	Interval      time.Duration
}
```

with:

```go
type WorkspaceUsageConfig struct {
	ServiceAPIKey string
	Interval      time.Duration
}
```

Replace the YAML struct:

```go
	WorkspaceUsage struct {
		SessionCookie string   `yaml:"session_cookie"`
		WorkspaceIDs  []string `yaml:"workspace_ids"`
		Interval      string   `yaml:"interval"`
	} `yaml:"workspace_usage"`
```

with:

```go
	WorkspaceUsage struct {
		ServiceAPIKey string `yaml:"service_api_key"`
		// Deprecated: ignored. Kept so old configs load and log a warning.
		SessionCookie string `yaml:"session_cookie"`
		Interval      string `yaml:"interval"`
	} `yaml:"workspace_usage"`
```

In `loadYAMLConfig`, replace:

```go
	wsIDs := make([]string, 0, len(yc.WorkspaceUsage.WorkspaceIDs))
	for _, id := range yc.WorkspaceUsage.WorkspaceIDs {
		if id = strings.TrimSpace(id); id != "" {
			wsIDs = append(wsIDs, id)
		}
	}
```

with:

```go
	if strings.TrimSpace(yc.WorkspaceUsage.SessionCookie) != "" {
		log.Printf("workspace_usage.session_cookie is no longer supported and was ignored; use workspace_usage.service_api_key")
	}
```

Replace the assignment:

```go
		WorkspaceUsage: WorkspaceUsageConfig{
			SessionCookie: strings.TrimSpace(yc.WorkspaceUsage.SessionCookie),
			WorkspaceIDs:  wsIDs,
			Interval:      wsInterval,
		},
```

with:

```go
		WorkspaceUsage: WorkspaceUsageConfig{
			ServiceAPIKey: strings.TrimSpace(yc.WorkspaceUsage.ServiceAPIKey),
			Interval:      wsInterval,
		},
```

In `mergeConfig`, replace:

```go
	if src.WorkspaceUsage.SessionCookie != "" {
		dst.WorkspaceUsage.SessionCookie = src.WorkspaceUsage.SessionCookie
	}
	if len(src.WorkspaceUsage.WorkspaceIDs) > 0 {
		dst.WorkspaceUsage.WorkspaceIDs = append([]string(nil), src.WorkspaceUsage.WorkspaceIDs...)
	}
```

with:

```go
	if src.WorkspaceUsage.ServiceAPIKey != "" {
		dst.WorkspaceUsage.ServiceAPIKey = src.WorkspaceUsage.ServiceAPIKey
	}
```

In `applyEnvOverrides`, replace:

```go
	if v := strings.TrimSpace(os.Getenv("OPENCODE_SESSION_COOKIE")); v != "" {
		cfg.WorkspaceUsage.SessionCookie = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENCODE_WORKSPACE_IDS")); v != "" {
		var ids []string
		for _, id := range strings.Split(v, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			cfg.WorkspaceUsage.WorkspaceIDs = ids
		}
	}
```

with:

```go
	if v := strings.TrimSpace(os.Getenv("OPENCODE_SERVICE_API_KEY")); v != "" {
		cfg.WorkspaceUsage.ServiceAPIKey = v
	}
```

In `safeConfigSummary`, replace `len(cfg.WorkspaceUsage.SessionCookie) > 0` with `len(cfg.WorkspaceUsage.ServiceAPIKey) > 0`.

Update the two call sites from Task 1 to use `ServiceAPIKey`:

```go
	app.workspace.Store(NewWorkspaceUsageClient(
		"",
		cfg.WorkspaceUsage.ServiceAPIKey,
	))
```

```go
		a.workspace.Store(NewWorkspaceUsageClient(
			"",
			newCfg.WorkspaceUsage.ServiceAPIKey,
		))
```

- [ ] **Step 4: Run the config tests**

Run: `go test ./... -run 'TestWorkspaceUsage' -count=1`
Expected: PASS.

- [ ] **Step 5: Update the example config, README, and configuration docs**

In `config.example.yaml`, replace the commented `workspace_usage` block with:

```yaml
# OpenCode workspace usage (optional): per-model cost & quota breakdown from
# the console, shown in the dashboard. Feature is off when service_api_key is empty.
# workspace_usage:
#   # Service account API key from the console (Service accounts -> create key).
#   # Starts with "oc_sk_". Prefer OPENCODE_SERVICE_API_KEY in shared environments.
#   service_api_key: ""
#   # Polling interval (default: 60s)
#   interval: "60s"
```

In `README.md`, change `When `workspace_usage.session_cookie` is configured` to `When `workspace_usage.service_api_key` is configured`.

In `docs/configuration.md`, replace the whole `## Workspace usage scraping` section with:

````markdown
## Workspace usage

Per-model cost and quota breakdowns are read from the OpenCode console API and
shown in the dashboard's workspace usage section. The feature is off when no
service account API key is configured.

### YAML

```yaml
workspace_usage:
  # Service account API key from the console (starts with "oc_sk_")
  service_api_key: ""
  # Polling interval (default: 60s, 0 disables polling)
  interval: "60s"

server:
  # Dashboard first-load proxy key: "auto" (default) embeds the key only when
  # listen_addr is loopback; "true" always; "false" never.
  dashboard_auto_key: "auto"
```

| Field | Default | Description |
| --- | --- | --- |
| `workspace_usage.service_api_key` | `""` (disabled) | Service account API key from `https://opencode.ai/console` (Service accounts). When empty, the feature is disabled and `GET /admin/workspace-usage` returns `{"enabled": false}`. |
| `workspace_usage.interval` | `60s` | Polling interval for console reads. `0` disables background polling. Must be `>= 0`. |
| `server.dashboard_auto_key` | `"auto"` | Controls whether the dashboard HTML embeds the proxy key for first-load convenience. `auto` embeds only when `listen_addr` is loopback (`127.0.0.1`, `::1`, `localhost`); `true` always embeds; `false` never embeds. The Settings panel override in localStorage still takes precedence. |

A config that still sets the removed `session_cookie` or `workspace_ids` fields loads, logs a warning, and ignores them.

### Environment variables

| Variable | Description |
| --- | --- |
| `OPENCODE_SERVICE_API_KEY` | Overrides `workspace_usage.service_api_key`. |
| `WORKSPACE_USAGE_INTERVAL` | Overrides `workspace_usage.interval` (Go duration, e.g. `60s`). |
| `DASHBOARD_AUTO_KEY` | Overrides `server.dashboard_auto_key` (`auto` \| `true` \| `false`). |

### Obtaining a service account key

1. Log in at `https://opencode.ai/console/`.
2. Open Service accounts, create a service account, and create an API key with the `all` permission.
3. Copy the key. It starts with `oc_sk_` and is shown once.
4. Paste it into `workspace_usage.service_api_key` or `OPENCODE_SERVICE_API_KEY`.

One key is bound to one workspace. Keys can carry an expiry. When a key expires or is revoked, the dashboard shows an error until a new key is configured. Treat the key as a secret, restrict config file permissions to `0600`, and prefer environment injection in shared environments.

### Failure behavior

Workspace usage reads are best-effort and never affect proxying. If the key is missing, expired, or the console is unreachable, the poller records the error and `GET /admin/workspace-usage` returns the error in its snapshot; the dashboard shows an error banner in the workspace usage section. Proxying, quota polling, and all other endpoints continue to work normally.
````

In `docs/admin-api.md`, change the link at the end of the Workspace usage section from `configuration.md#workspace-usage-scraping` to `configuration.md#workspace-usage`.

- [ ] **Step 6: Run the full suite**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add main.go main_test.go config.example.yaml README.md docs/configuration.md docs/admin-api.md
git commit -s -m "feat(config): replace workspace session cookie with service API key"
```

---

### Task 3: Verify end to end

**Files:**
- Modify: `bin/config.yaml` (untracked local config)

**Interfaces:**
- Consumes: the finished client and config from Tasks 1 and 2.
- Produces: verified evidence that the dashboard data path works against the live console.

- [ ] **Step 1: Format, vet, and test**

Run:

```bash
gofmt -l .
go vet ./...
go test ./... -count=1
```

Expected: `gofmt -l` prints nothing, `go vet` and `go test` pass. If `gofmt` lists files, run `gofmt -w` on them and include them in a commit.

- [ ] **Step 2: Check for leftover references**

Run: `grep -rn "seroval\|SessionCookie\|OPENCODE_SESSION_COOKIE\|workspace_ids" --include='*.go' --include='*.md' --include='*.yaml' --exclude-dir=.git --exclude-dir=node_modules .`
Expected: only the deprecated YAML field, its warning, the legacy-config test, and historical files under `docs/superpowers/`. No live code path uses the old names.

- [ ] **Step 3: Point the local config at a service key**

In `bin/config.yaml`, replace the `session_cookie` line under `workspace_usage` with:

```yaml
    service_api_key: "oc_sk_..."
```

Use a current key from the console. The test key from 2026-09-24 expires 2026-09-26; create a non-expiring key for daily use.

- [ ] **Step 4: Smoke test against the live console**

Run:

```bash
go build -o /tmp/switchboard-go-verify .
SWITCHBOARD_GO_CONFIG=bin/config.yaml /tmp/switchboard-go-verify &
PID=$!
sleep 3
curl -s http://127.0.0.1:8495/admin/workspace-usage \
  -H "Authorization: Bearer $(grep -m1 'proxy_api_key' bin/config.yaml | cut -d'"' -f2)" \
  | python3 -m json.tool | head -80
kill $PID
```

Expected: `"enabled": true`, one workspace `opencode-go` named `OpenCode Go`, three windows (`rolling`, `weekly`, `monthly`) with non-zero `usage_usd` and `limit_usd`, and rows whose `cost` equals `quota_cost`. If the port differs, use the `listen_addr` from `bin/config.yaml`.

- [ ] **Step 5: Report**

Report the smoke test output and any deviations from the spec. No commit is expected in this task; `bin/config.yaml` is untracked. If Step 1 reformatted files, commit them with `git commit -s -m "style: gofmt"`.
