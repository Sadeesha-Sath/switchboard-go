package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultWorkspaceBaseURL = "https://opencode.ai"
	microCentsPerUSD        = 1e8
)

var workspaceServerFnNames = []string{"getWorkspaces", "queryLiteSubscription", "queryLiteUsageDetails"}

var workspaceWindows = []string{"rolling", "weekly", "monthly"}

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

type WorkspaceInfo struct {
	ID   string
	Name string
}

type WorkspaceUsageClient struct {
	baseURL      string
	cookie       string
	workspaceIDs []string
	http         *http.Client

	mu       sync.Mutex
	hashes   map[string]string
	snapshot WorkspaceUsageSnapshot
}

func NewWorkspaceUsageClient(baseURL, cookie string, workspaceIDs []string) *WorkspaceUsageClient {
	if strings.TrimSpace(cookie) == "" {
		return nil
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultWorkspaceBaseURL
	}
	return &WorkspaceUsageClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		cookie:       cookie,
		workspaceIDs: workspaceIDs,
		http:         &http.Client{Timeout: 15 * time.Second},
		snapshot:     WorkspaceUsageSnapshot{Enabled: true},
	}
}

func (w *WorkspaceUsageClient) Enabled() bool { return w != nil }

func usdFromMicroCents(v float64) float64 { return v / microCentsPerUSD }

func toFloat(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func toStr(v any) string {
	s, _ := v.(string)
	return s
}

// callServerFn invokes a console server function by content hash with plain
// JSON args (verified wire protocol) and decodes the seroval stream response.
func (w *WorkspaceUsageClient) callServerFn(ctx context.Context, hash string, args []any) (any, error) {
	u := w.baseURL + "/_server?id=" + url.QueryEscape(hash)
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		u += "&args=" + url.QueryEscape(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", "auth="+w.cookie)
	req.Header.Set("X-Server-Instance", "server-fn:0")
	resp, err := w.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("workspace /_server returned status %d", resp.StatusCode)
	}
	return parseSerovalStream(string(body))
}

// getHashes returns cached function ids, discovering them on first use.
func (w *WorkspaceUsageClient) getHashes(ctx context.Context) (map[string]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.hashes) == len(workspaceServerFnNames) {
		return w.hashes, nil
	}
	hashes, err := w.discoverHashes(ctx)
	if err != nil {
		return nil, err
	}
	w.hashes = hashes
	return hashes, nil
}

func (w *WorkspaceUsageClient) invalidateHashes() {
	w.mu.Lock()
	w.hashes = nil
	w.mu.Unlock()
}

func (w *WorkspaceUsageClient) listWorkspaces(ctx context.Context, hashes map[string]string) ([]WorkspaceInfo, error) {
	v, err := w.callServerFn(ctx, hashes["getWorkspaces"], nil)
	if err != nil {
		return nil, fmt.Errorf("getWorkspaces: %w", err)
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("getWorkspaces: unexpected payload type %T", v)
	}
	out := make([]WorkspaceInfo, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := toStr(m["id"])
		if id == "" {
			continue
		}
		out = append(out, WorkspaceInfo{ID: id, Name: toStr(m["name"])})
	}
	return out, nil
}

var (
	entryScriptRe = regexp.MustCompile(`<script[^>]+src="(/_build/assets/entry-client-[^"]+\.js)"`)
	chunkImportRe = regexp.MustCompile(`"\./([^"]+\.js)"`)
	serverFnRefRe = regexp.MustCompile(`(getWorkspaces|queryLiteSubscription|queryLiteUsageDetails)_query\s*=\s*createServerReference\("([0-9a-f]{64})"\)`)
)

// discoverHashes walks the deployed JS bundle chain (entry chunk -> imported
// chunks) and extracts the current server function ids. Bundle fetch errors
// are swallowed; a partial result surfaces as the "discovered N of M" error.
func (w *WorkspaceUsageClient) discoverHashes(ctx context.Context) (map[string]string, error) {
	m := entryScriptRe.FindStringSubmatch(w.fetchText(ctx, w.baseURL+"/"))
	if m == nil {
		return nil, fmt.Errorf("workspace: entry script not found on landing page")
	}
	hashes := map[string]string{}
	visited := map[string]bool{}
	queue := []string{strings.TrimPrefix(m[1], "/_build/assets/")}
	for depth := 0; len(queue) > 0 && depth < 3 && len(hashes) < len(workspaceServerFnNames); depth++ {
		var next []string
		for _, name := range queue {
			if visited[name] || len(visited) >= 40 || len(hashes) == len(workspaceServerFnNames) {
				continue
			}
			visited[name] = true
			text := w.fetchText(ctx, w.baseURL+"/_build/assets/"+name)
			for _, sm := range serverFnRefRe.FindAllStringSubmatch(text, -1) {
				hashes[sm[1]] = sm[2]
			}
			for _, im := range chunkImportRe.FindAllStringSubmatch(text, -1) {
				next = append(next, im[1])
			}
		}
		queue = next
	}
	if len(hashes) != len(workspaceServerFnNames) {
		return nil, fmt.Errorf("workspace: discovered %d of %d server function ids", len(hashes), len(workspaceServerFnNames))
	}
	return hashes, nil
}

func (w *WorkspaceUsageClient) fetchText(ctx context.Context, u string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := w.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return string(b)
}

// Refresh runs a full scrape cycle. A transient server failure (e.g. rotated
// function hashes after a deploy) triggers one hash re-discovery + retry.
func (w *WorkspaceUsageClient) Refresh(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if err := w.refreshOnce(ctx); err != nil {
		w.invalidateHashes()
		if retryErr := w.refreshOnce(ctx); retryErr != nil {
			w.mu.Lock()
			w.snapshot.Error = retryErr.Error()
			w.mu.Unlock()
			return retryErr
		}
	}
	return nil
}

func (w *WorkspaceUsageClient) refreshOnce(ctx context.Context) error {
	hashes, err := w.getHashes(ctx)
	if err != nil {
		w.setSnapshotError(err.Error())
		return err
	}
	infos := []WorkspaceInfo{}
	if len(w.workspaceIDs) > 0 {
		for _, id := range w.workspaceIDs {
			infos = append(infos, WorkspaceInfo{ID: id, Name: id})
		}
	} else {
		infos, err = w.listWorkspaces(ctx, hashes)
		if err != nil {
			w.setSnapshotError(err.Error())
			return err
		}
	}
	statuses := make([]WorkspaceStatus, 0, len(infos))
	for _, info := range infos {
		status := WorkspaceStatus{ID: info.ID, Name: info.Name, Windows: map[string]WorkspaceWindowSnapshot{}}
		subV, err := w.callServerFn(ctx, hashes["queryLiteSubscription"], []any{info.ID})
		if err != nil {
			status.Error = err.Error()
			statuses = append(statuses, status)
			continue
		}
		sub, _ := subV.(map[string]any)
		for _, window := range workspaceWindows {
			var subWin map[string]any
			if sub != nil {
				subWin, _ = sub[map[string]string{"rolling": "rollingUsage", "weekly": "weeklyUsage", "monthly": "monthlyUsage"}[window]].(map[string]any)
			}
			snap, err := w.fetchWindowFromSubscription(ctx, hashes, info.ID, window, sub, subWin)
			if err != nil {
				status.Error = err.Error()
				break
			}
			status.Windows[window] = snap
		}
		statuses = append(statuses, status)
	}
	w.mu.Lock()
	w.snapshot = WorkspaceUsageSnapshot{
		Enabled:    true,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		Workspaces: statuses,
	}
	w.mu.Unlock()
	return nil
}

// fetchWindowFromSubscription fetches the per-model details for one window and
// merges them with the subscription window stats (status, resetInSec).
func (w *WorkspaceUsageClient) fetchWindowFromSubscription(ctx context.Context, hashes map[string]string, wsID, window string, sub map[string]any, subWin map[string]any) (WorkspaceWindowSnapshot, error) {
	detailsV, err := w.callServerFn(ctx, hashes["queryLiteUsageDetails"], []any{wsID, window})
	if err != nil {
		return WorkspaceWindowSnapshot{}, fmt.Errorf("details(%s): %w", window, err)
	}
	details, ok := detailsV.(map[string]any)
	if !ok {
		return WorkspaceWindowSnapshot{}, fmt.Errorf("details(%s): unexpected payload type %T", window, detailsV)
	}
	snap := WorkspaceWindowSnapshot{
		Status:       "ok",
		UsageUSD:     usdFromMicroCents(toFloat(details["usage"])),
		LimitUSD:     usdFromMicroCents(toFloat(details["limit"])),
		UsagePercent: toFloat(details["usagePercent"]),
		ResetInSec:   toFloat(subWin["resetInSec"]),
		Rows:         []WorkspaceModelRow{},
	}
	if s := toStr(subWin["status"]); s != "" {
		snap.Status = s
	}
	rows, _ := details["rows"].([]any)
	for _, r := range rows {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		row := WorkspaceModelRow{
			Model:               toStr(m["model"]),
			Name:                toStr(m["name"]),
			Cost:                usdFromMicroCents(toFloat(m["cost"])),
			QuotaCost:           usdFromMicroCents(toFloat(m["quotaCost"])),
			ContributionPercent: toFloat(m["contributionPercent"]),
			Estimated:           m["estimated"] == true,
		}
		if m["multiplier"] != nil {
			f := toFloat(m["multiplier"])
			row.Multiplier = &f
		}
		snap.Rows = append(snap.Rows, row)
	}
	return snap, nil
}

func (w *WorkspaceUsageClient) setSnapshotError(msg string) {
	w.mu.Lock()
	w.snapshot.Error = msg
	w.mu.Unlock()
}

func (a *App) startWorkspaceUsagePoller(ctx context.Context) {
	if a.workspace == nil || a.config.WorkspaceUsage.Interval <= 0 {
		return
	}
	go a.workspace.Refresh(ctx)
	ticker := time.NewTicker(a.config.WorkspaceUsage.Interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.workspace.Refresh(ctx)
			}
		}
	}()
}

// Snapshot returns the latest scrape result. Safe on a nil receiver.
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
