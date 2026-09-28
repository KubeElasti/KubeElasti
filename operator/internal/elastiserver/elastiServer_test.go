package elastiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/truefoundry/elasti/pkg/messages"
	"go.uber.org/zap"
)

// resolverReqHandler contract: the resolver is told "received" as soon as
// the wake is accepted, and the scale-up it triggers runs to completion
// regardless of what happens to that HTTP request afterwards.

func postWake(t *testing.T, url string) *http.Response {
	t.Helper()
	body, err := json.Marshal(messages.RequestCount{Count: 1, Svc: "app", Namespace: "team-a"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST wake: %v", err)
	}
	resp.Body.Close()
	return resp
}

func TestResolverReqHandlerRespondsBeforeScaleCompletes(t *testing.T) {
	s := NewServer(zap.NewNop(), nil, time.Second)
	release := make(chan struct{})
	s.scaleTarget = func(context.Context, string, string) error {
		<-release
		return nil
	}
	srv := httptest.NewServer(http.HandlerFunc(s.resolverReqHandler))
	defer srv.Close()
	defer close(release) // before srv.Close, which waits for in-flight handlers

	if resp := postWake(t, srv.URL); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestResolverReqHandlerScaleOutlivesTheRequest(t *testing.T) {
	s := NewServer(zap.NewNop(), nil, time.Second)
	responded := make(chan struct{})
	ctxErr := make(chan error, 1)
	s.scaleTarget = func(ctx context.Context, svc, ns string) error {
		if svc != "app" || ns != "team-a" {
			t.Errorf("scaleTarget(%q, %q), want (app, team-a)", svc, ns)
		}
		select {
		case <-responded:
		case <-time.After(3 * time.Second):
		}
		ctxErr <- ctx.Err()
		return nil
	}
	srv := httptest.NewServer(http.HandlerFunc(s.resolverReqHandler))
	defer srv.Close()

	postWake(t, srv.URL)
	close(responded)

	select {
	case err := <-ctxErr:
		if err != nil {
			t.Fatalf("scale context was cancelled with the request: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scaleTarget never finished")
	}
}
