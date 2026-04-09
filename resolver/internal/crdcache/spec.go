package crdcache

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Gateway API HTTPRoute-style match type strings (JSON "type" fields and defaults).
const (
	matchTypePathPrefix        = "PathPrefix"
	matchTypeExact             = "Exact"
	matchTypeRegularExpression = "RegularExpression"
)

type heartbeatSpecJSON struct {
	Heartbeat []heartbeatRuleJSON `json:"heartbeat,omitempty"`
}

type heartbeatRuleJSON struct {
	PathRaw     json.RawMessage           `json:"path,omitempty"`
	Headers     []httpHeaderMatchJSON     `json:"headers,omitempty"`
	QueryParams []httpQueryParamMatchJSON `json:"queryParams,omitempty"`
	Method      *string                   `json:"method,omitempty"`
	Response    string                    `json:"response"`
}

type httpHeaderMatchJSON struct {
	Type  *string `json:"type,omitempty"`
	Name  string  `json:"name"`
	Value string  `json:"value"`
}

type httpQueryParamMatchJSON struct {
	Type  *string `json:"type,omitempty"`
	Name  string  `json:"name"`
	Value string  `json:"value"`
}

type effectivePathMatch struct {
	Type  string
	Value string
}

// MatchHeartbeatFromSpec returns the configured response body when spec JSON matches the request
// using Gateway API HTTPRouteMatch semantics (path, headers, queryParams, method ANDed).
// Rules are tried in order; first match wins.
func MatchHeartbeatFromSpec(specJSON []byte, req *http.Request) (response string, matched bool) {
	if req == nil || len(specJSON) == 0 {
		return "", false
	}
	var s heartbeatSpecJSON
	if err := json.Unmarshal(specJSON, &s); err != nil {
		return "", false
	}
	for _, r := range s.Heartbeat {
		if strings.TrimSpace(r.Response) == "" {
			continue
		}
		if !methodMatchesHeartbeat(r.Method, req.Method) {
			continue
		}
		ep := parseHeartbeatPath(r.PathRaw)
		if ep == nil || !pathMatchesGateway(req.URL.Path, ep) {
			continue
		}
		if !headersMatchGateway(r.Headers, req.Header) {
			continue
		}
		if !queryParamsMatchGateway(r.QueryParams, req.URL.Query()) {
			continue
		}
		return r.Response, true
	}
	return "", false
}

func methodMatchesHeartbeat(ruleMethod *string, requestMethod string) bool {
	if ruleMethod == nil || strings.TrimSpace(*ruleMethod) == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(*ruleMethod), requestMethod)
}

func parseHeartbeatPath(raw json.RawMessage) *effectivePathMatch {
	if len(raw) == 0 {
		return &effectivePathMatch{Type: matchTypePathPrefix, Value: "/"}
	}
	b := bytes.TrimSpace(raw)
	if len(b) == 0 {
		return &effectivePathMatch{Type: matchTypePathPrefix, Value: "/"}
	}
	// Legacy: JSON string path uses exact match (previous ElastiService behavior).
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil
		}
		return &effectivePathMatch{Type: matchTypeExact, Value: s}
	}
	var obj struct {
		Type  *string `json:"type,omitempty"`
		Value string  `json:"value,omitempty"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	typ := matchTypePathPrefix
	if obj.Type != nil && strings.TrimSpace(*obj.Type) != "" {
		typ = strings.TrimSpace(*obj.Type)
	}
	val := obj.Value
	if val == "" {
		val = "/"
	}
	return &effectivePathMatch{Type: typ, Value: val}
}

func pathMatchesGateway(requestPath string, m *effectivePathMatch) bool {
	switch m.Type {
	case matchTypeExact:
		return NormalizeHTTPPath(requestPath) == NormalizeHTTPPath(m.Value)
	case matchTypePathPrefix:
		return pathPrefixMatchElements(requestPath, m.Value)
	case matchTypeRegularExpression:
		re, err := regexp.Compile(m.Value)
		if err != nil {
			return false
		}
		return re.MatchString(requestPath)
	default:
		return false
	}
}

// pathPrefixMatchElements implements Gateway API PathPrefix (path element boundaries, trailing slash ignored on prefix).
func pathPrefixMatchElements(requestPath, prefix string) bool {
	req := NormalizeHTTPPath(requestPath)
	pre := NormalizeHTTPPath(prefix)
	if pre == "/" || pre == "" {
		return true
	}
	reqParts := pathSplitElements(req)
	preParts := pathSplitElements(pre)
	if len(preParts) > len(reqParts) {
		return false
	}
	for i := range preParts {
		if reqParts[i] != preParts[i] {
			return false
		}
	}
	return true
}

func pathSplitElements(p string) []string {
	p = strings.TrimPrefix(NormalizeHTTPPath(p), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func headersMatchGateway(rules []httpHeaderMatchJSON, h http.Header) bool {
	for _, rule := range rules {
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return false
		}
		typ := matchTypeExact
		if rule.Type != nil && strings.TrimSpace(*rule.Type) != "" {
			typ = strings.TrimSpace(*rule.Type)
		}
		got := strings.TrimSpace(h.Get(name))
		want := rule.Value
		switch typ {
		case matchTypeExact:
			if got != want {
				return false
			}
		case matchTypeRegularExpression:
			re, err := regexp.Compile(want)
			if err != nil {
				return false
			}
			if !re.MatchString(got) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func queryParamsMatchGateway(rules []httpQueryParamMatchJSON, q url.Values) bool {
	for _, rule := range rules {
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return false
		}
		typ := matchTypeExact
		if rule.Type != nil && strings.TrimSpace(*rule.Type) != "" {
			typ = strings.TrimSpace(*rule.Type)
		}
		got := strings.TrimSpace(q.Get(name))
		want := rule.Value
		switch typ {
		case matchTypeExact:
			if got != want {
				return false
			}
		case matchTypeRegularExpression:
			re, err := regexp.Compile(want)
			if err != nil {
				return false
			}
			if !re.MatchString(got) {
				return false
			}
		default:
			return false
		}
	}
	return true
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
