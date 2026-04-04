package crdcache

import (
	"net/http"
	"testing"
)

func TestMatchHeartbeatFromSpec(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, ok := MatchHeartbeatFromSpec(nil, http.MethodGet, "/"); ok {
			t.Fatal("nil spec")
		}
		if _, ok := MatchHeartbeatFromSpec([]byte(`{`), http.MethodGet, "/"); ok {
			t.Fatal("invalid json")
		}
	})
	t.Run("heartbeat rules", func(t *testing.T) {
		spec := []byte(`{"heartbeat":[{"method":"POST","path":"/hook","response":"pong"},{"path":"/z","response":"z"}]}`)
		got, ok := MatchHeartbeatFromSpec(spec, http.MethodPost, "/hook")
		if !ok || got != "pong" {
			t.Fatalf("POST /hook: got %q ok=%v", got, ok)
		}
		got2, ok2 := MatchHeartbeatFromSpec(spec, http.MethodGet, "/z")
		if !ok2 || got2 != "z" {
			t.Fatalf("GET /z empty method: got %q ok=%v", got2, ok2)
		}
		if _, ok := MatchHeartbeatFromSpec(spec, http.MethodGet, "/hook"); ok {
			t.Fatal("GET should not match POST rule")
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

func TestPathsMatchHTTP(t *testing.T) {
	tests := []struct {
		req, cfg string
		want     bool
	}{
		{"/healthz", "/healthz", true},
		{"/healthz/", "/healthz", true},
		{"healthz", "/healthz", true},
		{"/other", "/healthz", false},
		{"/x", "", false},
		{"", "/x", false},
	}
	for _, tt := range tests {
		if got := PathsMatchHTTP(tt.req, tt.cfg); got != tt.want {
			t.Errorf("PathsMatchHTTP(%q,%q)=%v want %v", tt.req, tt.cfg, got, tt.want)
		}
	}
}
