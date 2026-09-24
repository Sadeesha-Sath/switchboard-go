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
