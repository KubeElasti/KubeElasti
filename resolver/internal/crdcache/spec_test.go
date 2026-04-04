package crdcache

import "testing"

func TestHeartbeatPathFromSpec(t *testing.T) {
	if got := HeartbeatPathFromSpec(nil); got != "" {
		t.Errorf("nil spec: got %q", got)
	}
	if got := HeartbeatPathFromSpec([]byte(`{`)); got != "" {
		t.Errorf("invalid json: got %q", got)
	}
	if got := HeartbeatPathFromSpec([]byte(`{"heartbeatPath":"/live"}`)); got != "/live" {
		t.Errorf("got %q want /live", got)
	}
	if got := HeartbeatPathFromSpec([]byte(`{"heartbeatPath":"  /x  "}`)); got != "/x" {
		t.Errorf("trim: got %q want /x", got)
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
