package scaling

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"truefoundry/elasti/operator/api/v1alpha1"
)

// A scale-down cycle over a large fleet stays well inside the polling
// interval: 500 ElastiServices whose trigger query takes 20ms each would
// need >= 10s one at a time; 16 at once takes well under a second.
func TestCheckAndScaleLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("load test")
	}
	const services, latency, workers = 500, 20 * time.Millisecond, 16
	var triggerQueries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := "1" // healthy
		if !strings.HasPrefix(r.URL.Query().Get("query"), "min_over_time((max(up{") {
			triggerQueries.Add(1)
			time.Sleep(latency)
			value = "0" // idle
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"` + value + `"]}]}}`))
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse stub URL: %v", err)
	}
	t.Setenv("PROMETHEUS_TRIGGER_ALLOWED_SERVER_ADDRESSES", u.Host)
	fleet := make([]v1alpha1.ElastiService, services)
	for i := range fleet {
		fleet[i] = idleES(t, fmt.Sprintf("app-%d", i), srv.URL, 300)
	}
	h := handlerListing(fleet...)
	h.scanner = newScanner(scanConfig{concurrency: workers, evaluationTimeout: 5 * time.Second})

	start := time.Now()
	if err := h.checkAndScale(context.Background()); err != nil {
		t.Fatalf("checkAndScale: %v", err)
	}
	elapsed := time.Since(start)
	h.scanner.waitForActions()

	if got := triggerQueries.Load(); got != services {
		t.Fatalf("%d trigger queries, want %d", got, services)
	}
	if limit := services * latency / 2; elapsed > limit {
		t.Fatalf("cycle over %d services took %v, want < %v (sequential would be >= %v)", services, elapsed, limit, services*latency)
	}
	t.Logf("cycle over %d services at %v per query with %d workers: %v", services, latency, workers, elapsed)
}
