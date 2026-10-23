package hostmanager

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

const fuzzHeaderForHost = "X-Envoy-Decorator-Operation"

func newFuzzHostManager() *HostManager {
	return NewHostManager(zap.NewNop(), time.Second, time.Second, fuzzHeaderForHost)
}

// FuzzGetHost feeds arbitrary Host values and header values to GetHost. Malformed traffic
// must never panic the resolver, and a successful lookup must always yield usable URLs.
func FuzzGetHost(f *testing.F) {
	seeds := [][2]string{
		{"target.com", "service.namespace.svc.cluster.local:8080/test/*"},
		{"service.namespace.svc.cluster.local", ""},
		{"service", ""},
		{"", ""},
		{"http://service.namespace", "https://x.y.svc/z"},
		{"a.b.c.d.e.f.g", "/*"},
		{"\x00\xff\n\r", "..//**"},
		{strings.Repeat("a.", 300), strings.Repeat("-", 1000)},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1])
	}

	f.Fuzz(func(t *testing.T, host, headerValue string) {
		hm := newFuzzHostManager()
		req := &http.Request{Host: host, Header: http.Header{}}
		if headerValue != "" {
			req.Header[fuzzHeaderForHost] = []string{headerValue}
		}

		got, err := hm.GetHost(req)
		if got == nil {
			t.Fatalf("GetHost(%q, %q) returned a nil host", host, headerValue)
		}
		if err != nil {
			return
		}
		if !strings.HasPrefix(got.SourceHost, "http://") && !strings.HasPrefix(got.SourceHost, "https://") {
			t.Fatalf("source host %q has no scheme (host %q, header %q)", got.SourceHost, host, headerValue)
		}
		if !strings.HasPrefix(got.TargetHost, "http://") && !strings.HasPrefix(got.TargetHost, "https://") {
			t.Fatalf("target host %q has no scheme (host %q, header %q)", got.TargetHost, host, headerValue)
		}
		if got.SourceService == "" || got.TargetService == "" || got.Namespace == "" {
			t.Fatalf("incomplete host %+v (host %q, header %q)", got, host, headerValue)
		}
	})
}

// FuzzExtractNamespaceAndService checks the URL parsing on its own.
func FuzzExtractNamespaceAndService(f *testing.F) {
	for _, s := range []string{
		"http://svc.ns.svc.cluster.local:8080/*",
		"svc.ns.svc.cluster.local",
		"svc.ns",
		"svc",
		"",
		".",
		"-.-",
		"\x00",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, url string) {
		hm := newFuzzHostManager()
		service, namespace, err := hm.extractNamespaceAndService(url)
		if err == nil && (service == "" || namespace == "") {
			t.Fatalf("no error but empty result for %q: service=%q namespace=%q", url, service, namespace)
		}
	})
}

// TestGetHostEmptyHeaderValues makes sure a header present without any value does not panic.
func TestGetHostEmptyHeaderValues(t *testing.T) {
	hm := newFuzzHostManager()
	req := &http.Request{
		Host:   "service.namespace.svc.cluster.local:8080",
		Header: http.Header{fuzzHeaderForHost: nil},
	}
	got, err := hm.GetHost(req)
	if err != nil {
		t.Fatalf("expected the Host field to be used, got error: %v", err)
	}
	if got.SourceService != "service" || got.Namespace != "namespace" {
		t.Fatalf("unexpected host %+v", got)
	}
}
