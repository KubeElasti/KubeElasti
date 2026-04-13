package crdcache

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/truefoundry/elasti/operator/api/v1alpha1"
)

// MatchProbeResponseFromSpec returns the configured response body and HTTP status when spec JSON
// matches the request using ElastiService CRD probeResponse semantics (path, headers, queryParams, method ANDed).
// Rules are tried in order; first match wins.
func MatchProbeResponseFromSpec(specJSON []byte, req *http.Request) (body string, status int, matched bool) {
	if req == nil || len(specJSON) == 0 {
		return "", 0, false
	}
	var s v1alpha1.ElastiServiceSpec
	if err := json.Unmarshal(specJSON, &s); err != nil {
		return "", 0, false
	}
	for _, r := range s.ProbeResponse {
		if strings.TrimSpace(r.Response.Body) == "" {
			continue
		}
		if !methodMatchesProbeResponse(r.Method, req.Method) {
			continue
		}
		if !pathMatches(req.URL.Path, r.Path) {
			continue
		}
		if !headersMatch(r.Headers, req.Header) {
			continue
		}
		if !queryParamsMatch(r.QueryParams, req.URL.Query()) {
			continue
		}
		return r.Response.Body, normalizeProbeHTTPStatus(r.Response.Status), true
	}
	return "", 0, false
}

func normalizeProbeHTTPStatus(code int) int {
	if code == 0 {
		return http.StatusOK
	}
	if code < 100 || code > 599 {
		return http.StatusOK
	}
	return code
}

func methodMatchesProbeResponse(ruleMethod *string, requestMethod string) bool {
	if ruleMethod == nil || strings.TrimSpace(*ruleMethod) == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(*ruleMethod), requestMethod)
}

func pathMatches(reqPath string, p *v1alpha1.ProbeResponsePathMatch) bool {
	pm := normalizeProbePathMatch(p)
	switch canonicalPathMatchType(pm.Type) {
	case "Exact":
		return reqPath == pm.Value
	case "RegularExpression":
		re, err := regexp.Compile(pm.Value)
		if err != nil {
			return false
		}
		return re.MatchString(reqPath)
	default: // PathPrefix
		return pathPrefix(reqPath, pm.Value)
	}
}

type probePathMatch struct {
	Type  string
	Value string
}

func normalizeProbePathMatch(p *v1alpha1.ProbeResponsePathMatch) probePathMatch {
	if p == nil {
		return probePathMatch{Type: "PathPrefix", Value: "/"}
	}
	typ := strings.TrimSpace(p.Type)
	if typ == "" {
		typ = "PathPrefix"
	}
	val := p.Value
	if canonicalPathMatchType(typ) == "PathPrefix" && val == "" {
		val = "/"
	}
	return probePathMatch{Type: typ, Value: val}
}

func canonicalPathMatchType(t string) string {
	ts := strings.TrimSpace(t)
	switch {
	case ts == "", strings.EqualFold(ts, "PathPrefix"):
		return "PathPrefix"
	case strings.EqualFold(ts, "Exact"):
		return "Exact"
	case strings.EqualFold(ts, "RegularExpression"):
		return "RegularExpression"
	default:
		return "PathPrefix"
	}
}

// pathPrefix implements segment-aware PathPrefix matching.
// HTTPPathMatch: /foo matches /foo and /foo/bar but not /foobar; "/" matches any path with a
// leading slash.
func pathPrefix(path, prefix string) bool {
	if prefix == "" || prefix == "/" {
		return strings.HasPrefix(path, "/")
	}
	if path == prefix {
		return true
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if strings.HasSuffix(prefix, "/") {
		return true
	}
	return len(path) > len(prefix) && path[len(prefix)] == '/'
}

func headersMatch(rules []v1alpha1.ProbeResponseHeaderMatch, h http.Header) bool {
	for _, rule := range rules {
		if strings.TrimSpace(rule.Name) == "" {
			return false
		}
		if !headerValueMatches(rule, h) {
			return false
		}
	}
	return true
}

func headerValueMatches(rule v1alpha1.ProbeResponseHeaderMatch, h http.Header) bool {
	key := http.CanonicalHeaderKey(strings.TrimSpace(rule.Name))
	values := h.Values(key)
	if len(values) == 0 {
		return false
	}
	typ := strings.TrimSpace(rule.Type)
	switch {
	case typ == "" || strings.EqualFold(typ, "Exact"):
		for _, v := range values {
			if v == rule.Value {
				return true
			}
		}
		return false
	case strings.EqualFold(typ, "RegularExpression"):
		re, err := regexp.Compile(rule.Value)
		if err != nil {
			return false
		}
		for _, v := range values {
			if re.MatchString(v) {
				return true
			}
		}
		return false
	default:
		for _, v := range values {
			if v == rule.Value {
				return true
			}
		}
		return false
	}
}

func queryParamsMatch(rules []v1alpha1.ProbeResponseQueryParamMatch, q url.Values) bool {
	for _, rule := range rules {
		if strings.TrimSpace(rule.Name) == "" {
			return false
		}
		if !queryParamValueMatches(rule, q) {
			return false
		}
	}
	return true
}

func queryParamValueMatches(rule v1alpha1.ProbeResponseQueryParamMatch, q url.Values) bool {
	values := q[rule.Name]
	if len(values) == 0 {
		return false
	}
	typ := strings.TrimSpace(rule.Type)
	switch {
	case typ == "" || strings.EqualFold(typ, "Exact"):
		for _, v := range values {
			if v == rule.Value {
				return true
			}
		}
		return false
	case strings.EqualFold(typ, "RegularExpression"):
		re, err := regexp.Compile(rule.Value)
		if err != nil {
			return false
		}
		for _, v := range values {
			if re.MatchString(v) {
				return true
			}
		}
		return false
	default:
		for _, v := range values {
			if v == rule.Value {
				return true
			}
		}
		return false
	}
}
