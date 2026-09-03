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
)

const (
	fakeHashWorkspaces   = "1111111111111111111111111111111111111111111111111111111111111111"
	fakeHashSubscription = "2222222222222222222222222222222222222222222222222222222222222222"
	fakeHashDetails      = "3333333333333333333333333333333333333333333333333333333333333333"
)

func wrapRoot(expr string) string {
	return fmt.Sprintf(`((self.$R=self.$R||{})["server-fn:0"]=[],($R=>$R[0]=%s)($R["server-fn:0"]))`, expr)
}

// newWorkspaceTestServer simulates the landing page, bundle chain, and /_server.
func newWorkspaceTestServer(t *testing.T, failFirstCall *atomic.Bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<html><script src="/_build/assets/entry-client-ABC.js"></script></html>`)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/_build/assets/entry-client-ABC.js", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `import("./workspace-XYZ.js");import("./index-GO1.js")`)
	})
	mux.HandleFunc("/_build/assets/workspace-XYZ.js", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `const getWorkspaces_query = createServerReference("%s")`, fakeHashWorkspaces)
	})
	mux.HandleFunc("/_build/assets/index-GO1.js", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `const queryLiteSubscription_query = createServerReference("%s");const queryLiteUsageDetails_query = createServerReference("%s")`, fakeHashSubscription, fakeHashDetails)
	})
	mux.HandleFunc("/_server", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "auth=test-cookie" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if failFirstCall != nil && failFirstCall.Load() {
			failFirstCall.Store(false)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		id := r.URL.Query().Get("id")
		args := r.URL.Query().Get("args")
		switch id {
		case fakeHashWorkspaces:
			fmt.Fprint(w, frame(wrapRoot(`[{id:"wrk_test1",name:"Default",slug:null},{id:"wrk_test2",name:"Workspace 2",slug:null}]`)))
		case fakeHashSubscription:
			if !strings.Contains(args, "wrk_test1") {
				http.Error(w, "unexpected workspace", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, frame(wrapRoot(`{mine:!0,useBalance:!1,allowTraining:!0,region:["us"],rollingUsage:{status:"ok",resetInSec:10901,usagePercent:16.4,usage:197024067,limit:1200000000},weeklyUsage:{status:"ok",resetInSec:392081,usagePercent:12.6,usage:379299700,limit:3000000000},monthlyUsage:{status:"ok",resetInSec:2447083,usagePercent:6.3,usage:379299700,limit:6000000000}}`)))
		case fakeHashDetails:
			switch {
			case strings.Contains(args, `"rolling"`):
				fmt.Fprint(w, frame(wrapRoot(`{usage:197024067,limit:1200000000,usagePercent:16.4,rows:[{model:"glm-5.3-flash",name:"GLM 5.3 Flash",cost:89949456,quotaCost:179898912,multiplier:2,estimated:!1,contributionPercent:15}]}`)))
			case strings.Contains(args, `"weekly"`):
				fmt.Fprint(w, frame(wrapRoot(`{usage:379299700,limit:3000000000,usagePercent:12.6,rows:[{model:"glm-5.3-flash",name:"GLM 5.3 Flash",cost:89949456,quotaCost:179898912,multiplier:2,estimated:!1,contributionPercent:6.3}]}`)))
			default:
				fmt.Fprint(w, frame(wrapRoot(`{usage:379299700,limit:6000000000,usagePercent:6.3,rows:[{model:"glm-5.3-flash",name:"GLM 5.3 Flash",cost:89949456,quotaCost:179898912,multiplier:2,estimated:!1,contributionPercent:5.9},{model:"muse-spark-1.2-contributor",name:"Muse Spark 1.2 Contributor",cost:22184350,quotaCost:22184350,multiplier:1,estimated:!1,contributionPercent:0.4}]}`)))
			}
		default:
			http.Error(w, "unknown id", http.StatusNotFound)
		}
	})
	return httptest.NewServer(mux)
}

func TestWorkspaceClientRefreshAndSnapshot(t *testing.T) {
	srv := newWorkspaceTestServer(t, nil)
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-cookie", nil)
	ctx := context.Background()
	if err := c.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	snap := c.Snapshot()
	if !snap.Enabled || snap.UpdatedAt == "" {
		t.Fatalf("snapshot not populated: %+v", snap)
	}
	if len(snap.Workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(snap.Workspaces))
	}
	ws := snap.Workspaces[0]
	if ws.ID != "wrk_test1" || ws.Name != "Default" {
		t.Fatalf("ws[0] = %+v", ws)
	}
	roll := ws.Windows["rolling"]
	if roll.Status != "ok" || roll.UsagePercent != 16.4 {
		t.Fatalf("rolling window = %+v", roll)
	}
	if len(roll.Rows) != 1 || roll.Rows[0].Model != "glm-5.3-flash" {
		t.Fatalf("rolling rows = %+v", roll.Rows)
	}
	wantCost := 89949456.0 / 1e8
	if roll.Rows[0].Cost != wantCost || roll.Rows[0].QuotaCost != 2*wantCost {
		t.Fatalf("row costs = %v/%v, want %v/%v", roll.Rows[0].Cost, roll.Rows[0].QuotaCost, wantCost, 2*wantCost)
	}
	if roll.Rows[0].Multiplier == nil || *roll.Rows[0].Multiplier != 2 {
		t.Fatalf("multiplier = %+v, want 2", roll.Rows[0].Multiplier)
	}
	if roll.UsageUSD != 197024067.0/1e8 || roll.LimitUSD != 12.0 {
		t.Fatalf("rolling usage/limit USD = %v/%v", roll.UsageUSD, roll.LimitUSD)
	}
	if _, ok := ws.Windows["monthly"]; !ok {
		t.Fatalf("monthly window missing")
	}
	// Second refresh reuses cached hashes; must still succeed.
	if err := c.Refresh(ctx); err != nil {
		t.Fatalf("second Refresh: %v", err)
	}
}

func TestWorkspaceClientWorkspaceIDFilter(t *testing.T) {
	srv := newWorkspaceTestServer(t, nil)
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-cookie", []string{"wrk_test2"})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	snap := c.Snapshot()
	if len(snap.Workspaces) != 1 || snap.Workspaces[0].ID != "wrk_test2" {
		t.Fatalf("filtered workspaces = %+v", snap.Workspaces)
	}
}

func TestWorkspaceClientUnauthorized(t *testing.T) {
	srv := newWorkspaceTestServer(t, nil)
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "wrong-cookie", nil)
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("expected error for unauthorized refresh")
	}
	snap := c.Snapshot()
	if snap.Error == "" {
		t.Fatalf("expected snapshot.Error to be set: %+v", snap)
	}
}

func TestWorkspaceClientRetriesAfterServerError(t *testing.T) {
	fail := atomic.Bool{}
	fail.Store(true)
	srv := newWorkspaceTestServer(t, &fail)
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-cookie", nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh should self-heal after a 500: %v", err)
	}
	snap := c.Snapshot()
	if snap.Error != "" || len(snap.Workspaces) != 2 {
		t.Fatalf("snapshot after retry = %+v", snap)
	}
}

func TestWorkspaceClientDiscoveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<html><body>no scripts here</body></html>`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-cookie", nil)
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("expected discovery failure error")
	}
	snap := c.Snapshot()
	if !snap.Enabled || snap.Error == "" {
		t.Fatalf("snapshot should stay enabled with error set: %+v", snap)
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
		t.Fatalf("feature should be disabled without a cookie: %+v", snap)
	}
	if snap.Workspaces == nil {
		t.Fatalf("workspaces should be an empty array, not null")
	}
}

func TestWorkspaceClientDiscoveryNewBundleFormat(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><script src="/_build/assets/entry-client-NEW.js"></script></html>`)
	})
	mux.HandleFunc("/_build/assets/entry-client-NEW.js", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `"build": () => __vitePreload(() => import(
  /* @vite-ignore */
  "./go-PAGE.js"
), true ? __vite__mapDeps([1,2]) : void 0)`)
	})
	mux.HandleFunc("/_build/assets/go-PAGE.js", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `const queryLiteSubscription_query = createServerReference("%s");const queryLiteUsageDetails_query = createServerReference("%s");const getWorkspaces_query = createServerReference("%s")`, fakeHashSubscription, fakeHashDetails, fakeHashWorkspaces)
	})
	mux.HandleFunc("/_server", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, frame(wrapRoot(`[{id:"wrk_test1",name:"Default",slug:null},{id:"wrk_test2",name:"Workspace 2",slug:null}]`)))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewWorkspaceUsageClient(srv.URL, "test-cookie", nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("discovery must survive vite-preload import formatting: %v", err)
	}
	if snap := c.Snapshot(); len(snap.Workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(snap.Workspaces))
	}
}
