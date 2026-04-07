package crdcache

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchHeartbeatFromSpec(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if _, ok := MatchHeartbeatFromSpec(nil, req); ok {
			t.Fatal("nil spec")
		}
		if _, ok := MatchHeartbeatFromSpec([]byte(`{`), req); ok {
			t.Fatal("invalid json")
		}
		if _, ok := MatchHeartbeatFromSpec([]byte(`{}`), nil); ok {
			t.Fatal("nil req")
		}
	})
	t.Run("legacy string path exact", func(t *testing.T) {
		spec := []byte(`{"heartbeat":[{"method":"POST","path":"/hook","response":"pong"},{"path":"/z","response":"z"}]}`)
		req1 := httptest.NewRequest(http.MethodPost, "/hook", nil)
		got, ok := MatchHeartbeatFromSpec(spec, req1)
		if !ok || got != "pong" {
			t.Fatalf("POST /hook: got %q ok=%v", got, ok)
		}
		req2 := httptest.NewRequest(http.MethodGet, "/z", nil)
		got2, ok2 := MatchHeartbeatFromSpec(spec, req2)
		if !ok2 || got2 != "z" {
			t.Fatalf("GET /z: got %q ok=%v", got2, ok2)
		}
		req3 := httptest.NewRequest(http.MethodGet, "/hook", nil)
		if _, ok := MatchHeartbeatFromSpec(spec, req3); ok {
			t.Fatal("GET should not match POST rule")
		}
	})
	t.Run("gateway path prefix object", func(t *testing.T) {
		spec := []byte(`{"heartbeat":[{"path":{"type":"PathPrefix","value":"/api"},"response":"ok"}]}`)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
		got, ok := MatchHeartbeatFromSpec(spec, req)
		if !ok || got != "ok" {
			t.Fatalf("prefix /api: got %q ok=%v", got, ok)
		}
		reqNo := httptest.NewRequest(http.MethodGet, "/other", nil)
		if _, ok := MatchHeartbeatFromSpec(spec, reqNo); ok {
			t.Fatal("/other should not match /api prefix")
		}
	})
	t.Run("default path prefix slash", func(t *testing.T) {
		spec := []byte(`{"heartbeat":[{"response":"any"}]}`)
		req := httptest.NewRequest(http.MethodGet, "/anything", nil)
		got, ok := MatchHeartbeatFromSpec(spec, req)
		if !ok || got != "any" {
			t.Fatalf("default path: got %q ok=%v", got, ok)
		}
	})
	t.Run("headers AND", func(t *testing.T) {
		spec := []byte(`{"heartbeat":[{"path":{"type":"Exact","value":"/h"},"headers":[{"name":"X-Probe","value":"1"}],"response":"yes"}]}`)
		reqOK := httptest.NewRequest(http.MethodGet, "/h", nil)
		reqOK.Header.Set("X-Probe", "1")
		got, ok := MatchHeartbeatFromSpec(spec, reqOK)
		if !ok || got != "yes" {
			t.Fatalf("with header: got %q ok=%v", got, ok)
		}
		reqBad := httptest.NewRequest(http.MethodGet, "/h", nil)
		if _, ok := MatchHeartbeatFromSpec(spec, reqBad); ok {
			t.Fatal("missing header should not match")
		}
	})
	t.Run("query params", func(t *testing.T) {
		spec := []byte(`{"heartbeat":[{"path":{"type":"Exact","value":"/q"},"queryParams":[{"name":"ping","value":"pong"}],"response":"qp"}]}`)
		req := httptest.NewRequest(http.MethodGet, "/q?ping=pong", nil)
		got, ok := MatchHeartbeatFromSpec(spec, req)
		if !ok || got != "qp" {
			t.Fatalf("query: got %q ok=%v", got, ok)
		}
	})
}

func TestHeartbeatContentType(t *testing.T) {
	if got := HeartbeatContentType("hello"); got != "text/plain; charset=utf-8" {
		t.Errorf("plain: %q", got)
	}
	if got := HeartbeatContentType(`{"a":1}`); got != "application/json; charset=utf-8" {
		t.Errorf("json: %q", got)
	}
}

func TestPathPrefixMatchElements(t *testing.T) {
	tests := []struct {
		req, prefix string
		want        bool
	}{
		{"/api/v1", "/api", true},
		{"/api", "/api", true},
		{"/api/", "/api", true},
		{"/ap", "/api", false},
		{"/apiextra", "/api", false},
		{"/anything", "/", true},
	}
	for _, tt := range tests {
		if got := pathPrefixMatchElements(tt.req, tt.prefix); got != tt.want {
			t.Errorf("pathPrefixMatchElements(%q,%q)=%v want %v", tt.req, tt.prefix, got, tt.want)
		}
	}
}

func TestNormalizeHTTPPath(t *testing.T) {
	if got := NormalizeHTTPPath("healthz"); got != "/healthz" {
		t.Errorf("normalize: %q", got)
	}
	if got := NormalizeHTTPPath("/healthz//x"); got != "/healthz/x" {
		t.Errorf("clean: %q", got)
	}
}
