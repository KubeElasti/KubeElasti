package throttler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/truefoundry/elasti/pkg/k8shelper"
	"github.com/truefoundry/elasti/pkg/messages"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

// apiServerWithEndpoints stubs the EndpointSlice LIST the throttler's
// readiness check makes: one ready endpoint when ready, none otherwise.
func apiServerWithEndpoints(t *testing.T, ready bool) *rest.Config {
	t.Helper()
	endpoints := `[]`
	if ready {
		endpoints = `[{"addresses":["10.0.0.1"],"conditions":{"ready":true}}]`
	}
	body := `{"kind":"EndpointSliceList","apiVersion":"discovery.k8s.io/v1","metadata":{},` +
		`"items":[{"metadata":{"name":"app-pvt-x"},"addressType":"IPv4","endpoints":` + endpoints + `}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &rest.Config{Host: srv.URL}
}

func newTestThrottler(t *testing.T, ready bool, retry time.Duration) *Throttler {
	t.Helper()
	return NewThrottler(&Params{
		QueueRetryDuration:      retry,
		TrafficReEnableDuration: time.Minute,
		K8sUtil:                 k8shelper.NewOps(zap.NewNop(), apiServerWithEndpoints(t, ready)),
		QueueDepth:              10,
		MaxConcurrency:          10,
		InitialCapacity:         10,
		Logger:                  zap.NewNop(),
	})
}

var testHost = &messages.Host{Namespace: "ns", SourceService: "app", TargetService: "app-pvt"}

// The context bounds how long a request waits for the target to become
// ready. Once the request has been proxied it has succeeded, however long
// the proxied response took: reporting it as a timeout makes the caller
// write a second, timeout response over the one already sent.
func TestTry_ProxiedRequestOutlastingTheHoldIsNotATimeout(t *testing.T) {
	th := newTestThrottler(t, true, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := th.Try(ctx, testHost, func(int) error {
		time.Sleep(50 * time.Millisecond) // a response slower than the hold
		return nil
	}, func() {})
	if err != nil {
		t.Fatalf("Try = %v, want nil: the request was proxied", err)
	}
}

// A zero hold (the context is already done) still proxies to a ready
// target: the hold only applies when the target isn't ready.
func TestTry_ZeroHoldStillProxiesToAReadyTarget(t *testing.T) {
	th := newTestThrottler(t, true, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	proxied := false
	err := th.Try(ctx, testHost, func(int) error { proxied = true; return nil }, func() {})
	if err != nil || !proxied {
		t.Fatalf("Try = %v, proxied = %v; want nil, true", err, proxied)
	}
}

// A zero hold on a target that isn't ready answers at once with a deadline
// error (the caller's cue to send the loading page or a 503) and still
// sends the wake.
func TestTry_ZeroHoldOnAColdTargetWakesAndReturnsAtOnce(t *testing.T) {
	th := newTestThrottler(t, false, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	woken := make(chan struct{}, 1)

	start := time.Now()
	err := th.Try(ctx, testHost, func(int) error {
		t.Error("proxied to a target with no ready endpoints")
		return nil
	}, func() { woken <- struct{}{} })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Try = %v, want a DeadlineExceeded error", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Try took %s with a zero hold", elapsed)
	}
	select {
	case <-woken:
	case <-time.After(2 * time.Second):
		t.Fatal("no wake sent for a cold target")
	}
}

// The hold bounds the wait for readiness: a retry interval longer than what
// is left of the hold must not delay the answer to the end of the interval.
func TestTry_HoldEndingDuringARetryIntervalAnswersAtOnce(t *testing.T) {
	th := newTestThrottler(t, false, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := th.Try(ctx, testHost, func(int) error { return nil }, func() {})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Try = %v, want a DeadlineExceeded error", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Try answered after %s; the hold was 50ms", elapsed)
	}
}
