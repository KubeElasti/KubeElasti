package scaling

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"truefoundry/elasti/operator/api/v1alpha1"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Scaler health is a property of the Prometheus backend, not of the
// ElastiService: each scale-down cycle checks it once per backend (and
// health query window), not once per ElastiService.

// stubPrometheus counts health (uptime) and trigger queries. Triggers read
// 0, i.e. idle; health reads 1 unless healthStatus is set.
type stubPrometheus struct {
	healthQueries, triggerQueries atomic.Int32
	healthStatus                  int
}

func (p *stubPrometheus) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("query"), "min_over_time((max(up{") {
			p.healthQueries.Add(1)
			if p.healthStatus != 0 {
				w.WriteHeader(p.healthStatus)
				return
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"1"]}]}}`))
			return
		}
		p.triggerQueries.Add(1)
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1,"0"]}]}}`))
	}))
	t.Cleanup(srv.Close)
	// httptest listens on loopback; allow-listing it lifts the SSRF dial guard.
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse stub URL: %v", err)
	}
	t.Setenv("PROMETHEUS_TRIGGER_ALLOWED_SERVER_ADDRESSES", u.Host)
	return srv
}

// idleES has one Prometheus trigger. It was just scaled up, so a ScaleDown
// decision stops at the cooldown check without touching the cluster.
func idleES(t *testing.T, name, serverURL string, cooldownSeconds int32) v1alpha1.ElastiService {
	t.Helper()
	metadata, err := json.Marshal(map[string]string{"query": "sum(rate(requests[1m]))", "serverAddress": serverURL, "threshold": "1"})
	if err != nil {
		t.Fatalf("marshal trigger metadata: %v", err)
	}
	es := v1alpha1.ElastiService{}
	es.Namespace = "team-a"
	es.Name = name
	es.CreationTimestamp = metav1.NewTime(time.Now().Add(-24 * time.Hour))
	es.Spec.Service = name
	es.Spec.CooldownPeriod = cooldownSeconds
	es.Spec.Triggers = []v1alpha1.ScaleTrigger{{Type: "prometheus", Metadata: metadata}}
	now := metav1.Now()
	es.Status.LastScaledUpTime = &now
	return es
}

func handlerListing(services ...v1alpha1.ElastiService) *ScaleHandler {
	h := &ScaleHandler{logger: zap.NewNop()}
	h.SetElastiServiceLister(func(context.Context) ([]v1alpha1.ElastiService, error) {
		return services, nil
	})
	return h
}

func TestCheckAndScaleChecksScalerHealthOncePerCycle(t *testing.T) {
	prom := &stubPrometheus{}
	srv := prom.start(t)
	var services []v1alpha1.ElastiService
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		services = append(services, idleES(t, name, srv.URL, 300))
	}
	h := handlerListing(services...)

	for cycle := 1; cycle <= 2; cycle++ {
		if err := h.checkAndScale(context.Background()); err != nil {
			t.Fatalf("cycle %d: checkAndScale: %v", cycle, err)
		}
		if got := prom.healthQueries.Load(); got != int32(cycle) {
			t.Fatalf("after cycle %d: %d health queries, want %d (one per cycle)", cycle, got, cycle)
		}
	}
	if got := prom.triggerQueries.Load(); got != 20 {
		t.Fatalf("%d trigger queries over 2 cycles, want 20 (one per ElastiService per cycle)", got)
	}
}

func TestCheckAndScaleChecksScalerHealthPerCooldownWindow(t *testing.T) {
	prom := &stubPrometheus{}
	srv := prom.start(t)
	h := handlerListing(idleES(t, "a", srv.URL, 300), idleES(t, "b", srv.URL, 300), idleES(t, "c", srv.URL, 900))

	if err := h.checkAndScale(context.Background()); err != nil {
		t.Fatalf("checkAndScale: %v", err)
	}
	if got := prom.healthQueries.Load(); got != 2 {
		t.Fatalf("%d health queries, want 2 (one per distinct health window)", got)
	}
}

func TestCheckAndScaleFailedHealthCheckIsNotRetriedWithinCycle(t *testing.T) {
	prom := &stubPrometheus{healthStatus: http.StatusServiceUnavailable}
	srv := prom.start(t)
	h := handlerListing(idleES(t, "a", srv.URL, 300), idleES(t, "b", srv.URL, 300), idleES(t, "c", srv.URL, 300))

	if err := h.checkAndScale(context.Background()); err != nil {
		t.Fatalf("checkAndScale: %v", err)
	}
	if got := prom.healthQueries.Load(); got != 1 {
		t.Fatalf("%d health queries against a failing Prometheus, want 1", got)
	}
	if got := prom.triggerQueries.Load(); got != 0 {
		t.Fatalf("%d trigger queries, want 0 while the scaler is unhealthy", got)
	}
}

// countingScaler has no HealthKey, so its health can't be shared.
type countingScaler struct{ healthChecks int }

func (s *countingScaler) IsHealthy(context.Context) (bool, error) {
	s.healthChecks++
	return true, nil
}
func (s *countingScaler) ShouldScaleToZero(context.Context) (bool, error)   { return false, nil }
func (s *countingScaler) ShouldScaleFromZero(context.Context) (bool, error) { return false, nil }
func (s *countingScaler) Close(context.Context) error                       { return nil }

func TestHealthMemoChecksUnkeyedScalerEveryTime(t *testing.T) {
	memo := newHealthMemo()
	s := &countingScaler{}
	for i := 0; i < 3; i++ {
		if healthy, err := memo.isHealthy(context.Background(), s); err != nil || !healthy {
			t.Fatalf("isHealthy = %v, %v; want true, nil", healthy, err)
		}
	}
	if s.healthChecks != 3 {
		t.Fatalf("%d health checks, want 3 (no key, nothing to share)", s.healthChecks)
	}
}
