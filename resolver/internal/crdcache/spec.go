package crdcache

import (
	"encoding/json"
	"path"
	"strings"
)

type elastiserviceSpecFields struct {
	HeartbeatPath string `json:"heartbeatPath,omitempty"`
}

// HeartbeatPathFromSpec returns the trimmed heartbeatPath from marshaled ElastiService spec JSON.
func HeartbeatPathFromSpec(specJSON []byte) string {
	if len(specJSON) == 0 {
		return ""
	}
	var s elastiserviceSpecFields
	if err := json.Unmarshal(specJSON, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s.HeartbeatPath)
}

// NormalizeHTTPPath returns a canonical path for comparison (leading slash, path.Clean).
func NormalizeHTTPPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	c := path.Clean(p)
	if c == "." {
		return "/"
	}
	return c
}

// PathsMatchHTTP reports whether the request path matches a configured heartbeat path.
func PathsMatchHTTP(requestPath, configuredPath string) bool {
	cfg := NormalizeHTTPPath(configuredPath)
	if cfg == "" {
		return false
	}
	return NormalizeHTTPPath(requestPath) == cfg
}
