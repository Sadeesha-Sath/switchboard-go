package main

import (
	"fmt"
	"reflect"
	"testing"
)

// frame wraps a seroval payload in the ;0x<hex-len>; framing used by /_server.
func frame(parts ...string) string {
	out := ""
	for _, p := range parts {
		out += fmt.Sprintf(";0x%08x;%s", len(p), p)
	}
	return out
}

func wrapServerFn(instance string, expr string) string {
	return fmt.Sprintf(`((self.$R=self.$R||{})["%s"]=[],($R=>$R[0]=%s)($R["%s"]))`, instance, expr, instance)
}

func TestParseSerovalStreamWorkspaces(t *testing.T) {
	payload := wrapServerFn("server-fn:0", `[$R[1]={id:"wrk_test1",name:"Default",slug:null},$R[2]={id:"wrk_test2",name:"Workspace 2",slug:null}]`)
	got, err := parseSerovalStream(frame(payload))
	if err != nil {
		t.Fatalf("parseSerovalStream: %v", err)
	}
	want := []any{
		map[string]any{"id": "wrk_test1", "name": "Default", "slug": nil},
		map[string]any{"id": "wrk_test2", "name": "Workspace 2", "slug": nil},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseSerovalStreamUsageDetails(t *testing.T) {
	payload := wrapServerFn("server-fn:1", `{usage:378193408,limit:6000000000,usagePercent:6.3,rows:$R[1]=[$R[2]={model:"glm-5.3-flash",name:"GLM 5.3 Flash",cost:178004529,quotaCost:356009058,multiplier:2,estimated:!1,contributionPercent:5.9},$R[3]={model:"muse-spark-1.2-contributor",name:"Muse Spark 1.2 Contributor",cost:22184350,quotaCost:22184350,multiplier:1,estimated:!1,contributionPercent:0.4}]}`)
	got, err := parseSerovalStream(frame(payload))
	if err != nil {
		t.Fatalf("parseSerovalStream: %v", err)
	}
	root, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("root = %T, want map", got)
	}
	if root["usagePercent"] != 6.3 {
		t.Fatalf("usagePercent = %#v, want 6.3", root["usagePercent"])
	}
	rows, ok := root["rows"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("rows = %#v, want 2 entries", root["rows"])
	}
	first := rows[0].(map[string]any)
	if first["cost"] != int64(178004529) || first["estimated"] != false || first["multiplier"] != int64(2) {
		t.Fatalf("row[0] = %#v", first)
	}
}

func TestParseSerovalStreamMultiFrameAndStrings(t *testing.T) {
	a := wrapServerFn("server-fn:2", `{region:["us","eu","sg","cn"],mine:!0,reset:10901}`)
	b := wrapServerFn("server-fn:3", `{msg:"line\nbreak \"quoted\""}`)
	got, err := parseSerovalStream(frame(a, b))
	if err != nil {
		t.Fatalf("parseSerovalStream: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil, want non-nil root")
	}
}

func TestParseSerovalStreamEscapes(t *testing.T) {
	payload := wrapServerFn("server-fn:esc", `{msg:"line\nbreak \"quoted\"",uni:"\u0041",tab:"a\tb",bs:"a\\b"}`)
	got, err := parseSerovalStream(frame(payload))
	if err != nil {
		t.Fatalf("parseSerovalStream: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("root = %T, want map", got)
	}
	if m["msg"] != "line\nbreak \"quoted\"" {
		t.Fatalf("msg = %#v, want %q", m["msg"], "line\nbreak \"quoted\"")
	}
	if m["uni"] != "A" {
		t.Fatalf("uni = %#v, want %q", m["uni"], "A")
	}
	if m["tab"] != "a\tb" {
		t.Fatalf("tab = %#v, want %q", m["tab"], "a\tb")
	}
	if m["bs"] != "a\\b" {
		t.Fatalf("bs = %#v, want %q", m["bs"], "a\\b")
	}
}

func TestParseSerovalStreamTruncatedAndBadFrame(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "truncated after {", body: ";0x00000007;$R[0]={"},
		{name: "truncated after trailing comma", body: frame("$R[0]={a:1,")},
		{name: "bad frame header", body: "not-a-frame"},
		{name: "negative frame length", body: ";0x-5;hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic on %q: %v", tc.name, r)
				}
			}()
			_, err := parseSerovalStream(tc.body)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.name)
			}
		})
	}
}
