package crdcache

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
)

type heartbeatSpecJSON struct {
	Heartbeat []heartbeatRuleJSON `json:"heartbeat,omitempty"`
}

type heartbeatRuleJSON struct {
	Method   string `json:"method,omitempty"`
	Path     string `json:"path"`
	Response string `json:"response"`
}

// MatchHeartbeatFromSpec returns the configured response body when spec JSON matches method and path.
// Rules are tried in order; first match wins.
func MatchHeartbeatFromSpec(specJSON []byte, method, requestPath string) (response string, matched bool) {
	if len(specJSON) == 0 {
		return "", false
	}
	var s heartbeatSpecJSON
	if err := json.Unmarshal(specJSON, &s); err != nil {
		return "", false
	}
	for _, r := range s.Heartbeat {
		if strings.TrimSpace(r.Path) == "" {
			continue
		}
		if !httpMethodMatchesRule(r.Method, method) {
			continue
		}
		if !PathsMatchHTTP(requestPath, r.Path) {
			continue
		}
		return r.Response, true
	}
	return "", false
}

func httpMethodMatchesRule(ruleMethod, requestMethod string) bool {
	rm := strings.TrimSpace(strings.ToUpper(ruleMethod))
	req := strings.ToUpper(requestMethod)
	if rm == "" || rm == "*" {
		return req == http.MethodGet || req == http.MethodHead
	}
	return rm == req
}

// HeartbeatContentType picks Content-Type for a synthetic heartbeat body.
func HeartbeatContentType(body string) string {
	b := strings.TrimSpace(body)
	n := len(b)
	if n >= 2 {
		if b[0] == '{' && b[n-1] == '}' {
			return "application/json; charset=utf-8"
		}
		if b[0] == '[' && b[n-1] == ']' {
			return "application/json; charset=utf-8"
		}
	}
	return "text/plain; charset=utf-8"
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
