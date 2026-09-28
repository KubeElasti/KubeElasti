package operator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/truefoundry/elasti/pkg/messages"
	"go.uber.org/zap"
)

// SendIncomingRequestInfo contract: the operator hears about the first
// request for each namespace/service within retryDuration, and a slow or
// hung operator never holds that dedup slot for longer than retryDuration
// past the call.

// wakeRecorder is a stub operator that records the namespace/service of
// every incoming-request notification.
type wakeRecorder struct {
	mu    sync.Mutex
	wakes []string
}

func (r *wakeRecorder) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var body messages.RequestCount
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("decode wake body: %v", err)
		}
		r.mu.Lock()
		r.wakes = append(r.wakes, body.Namespace+"/"+body.Svc)
		r.mu.Unlock()
	}
}

func (r *wakeRecorder) got() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.wakes...)
}

func TestSendIncomingRequestInfoSameServiceNameInTwoNamespacesSendsBoth(t *testing.T) {
	rec := &wakeRecorder{}
	srv := httptest.NewServer(rec.handler(t))
	defer srv.Close()
	c := NewOperatorClientWithURL(zap.NewNop(), time.Minute, srv.URL)

	c.SendIncomingRequestInfo("team-a", "app")
	c.SendIncomingRequestInfo("team-b", "app")

	if got := rec.got(); len(got) != 2 || got[0] != "team-a/app" || got[1] != "team-b/app" {
		t.Fatalf("operator saw wakes %v, want [team-a/app team-b/app]", got)
	}
}

func TestSendIncomingRequestInfoDedupsSameServiceWithinRetryDuration(t *testing.T) {
	rec := &wakeRecorder{}
	srv := httptest.NewServer(rec.handler(t))
	defer srv.Close()
	c := NewOperatorClientWithURL(zap.NewNop(), time.Minute, srv.URL)

	c.SendIncomingRequestInfo("team-a", "app")
	c.SendIncomingRequestInfo("team-a", "app")

	if got := rec.got(); len(got) != 1 {
		t.Fatalf("operator saw wakes %v, want exactly one", got)
	}
}

func TestSendIncomingRequestInfoHungOperatorReleasesDedupSlot(t *testing.T) {
	release := make(chan struct{})
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
		<-release
	}))
	defer srv.Close()
	defer close(release)
	const retry = 100 * time.Millisecond
	c := NewOperatorClientWithURL(zap.NewNop(), retry, srv.URL)

	returned := make(chan struct{})
	go func() {
		c.SendIncomingRequestInfo("team-a", "app")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(10 * retry):
		t.Fatal("SendIncomingRequestInfo blocked on a hung operator")
	}

	// Once the timed-out call's retry window passes, the next request for
	// the same service reaches the operator again.
	for deadline := time.Now().Add(20 * retry); time.Now().Before(deadline); time.Sleep(retry / 2) {
		c.SendIncomingRequestInfo("team-a", "app")
		if hits.Load() >= 2 {
			return
		}
	}
	t.Fatal("dedup slot for team-a/app was never released after the operator hung")
}
