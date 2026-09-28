package scaling

import (
	"context"
	"errors"
	"testing"

	"truefoundry/elasti/operator/api/v1alpha1"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// checkAndScale contract: each cycle evaluates exactly the ElastiServices
// its lister returns, and a partial list failure still evaluates what was
// listed before reporting the error.

func handlerWithLister(lister ElastiServiceLister) (*ScaleHandler, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.InfoLevel)
	h := &ScaleHandler{logger: zap.New(core), scanner: newScanner(scanConfig{})}
	h.SetElastiServiceLister(lister)
	return h, logs
}

// untriggeredES is evaluated but never scaled: with no triggers,
// calculateScaleDirection logs it and moves on.
func untriggeredES(ns, name string) v1alpha1.ElastiService {
	es := v1alpha1.ElastiService{}
	es.Namespace = ns
	es.Name = name
	es.Spec.Service = name
	return es
}

func evaluated(logs *observer.ObservedLogs, ns, svc string) bool {
	for _, e := range logs.FilterMessage("No triggers found, skipping scale to zero").All() {
		if e.ContextMap()["namespace"] == ns && e.ContextMap()["service"] == svc {
			return true
		}
	}
	return false
}

func TestCheckAndScaleEvaluatesServicesFromLister(t *testing.T) {
	calls := 0
	h, logs := handlerWithLister(func(context.Context) ([]v1alpha1.ElastiService, error) {
		calls++
		return []v1alpha1.ElastiService{untriggeredES("team-a", "app"), untriggeredES("team-b", "app")}, nil
	})

	if err := h.checkAndScale(context.Background()); err != nil {
		t.Fatalf("checkAndScale: %v", err)
	}
	if calls != 1 {
		t.Fatalf("lister called %d times, want 1", calls)
	}
	if !evaluated(logs, "team-a", "app") || !evaluated(logs, "team-b", "app") {
		t.Fatal("not every listed ElastiService was evaluated")
	}
}

func TestCheckAndScaleEvaluatesListedServicesDespiteListError(t *testing.T) {
	listErr := errors.New("namespace team-b: forbidden")
	h, logs := handlerWithLister(func(context.Context) ([]v1alpha1.ElastiService, error) {
		return []v1alpha1.ElastiService{untriggeredES("team-a", "app")}, listErr
	})

	err := h.checkAndScale(context.Background())
	if !errors.Is(err, listErr) {
		t.Fatalf("checkAndScale error = %v, want it to wrap %v", err, listErr)
	}
	if !evaluated(logs, "team-a", "app") {
		t.Fatal("services listed before the error were not evaluated")
	}
}

func TestCheckAndScaleReportsCycleToScanObserver(t *testing.T) {
	h, _ := handlerWithLister(func(context.Context) ([]v1alpha1.ElastiService, error) {
		return []v1alpha1.ElastiService{untriggeredES("team-a", "app")}, nil
	})
	obs := newRecordingObserver()
	h.SetScanObserver(obs)

	if err := h.checkAndScale(context.Background()); err != nil {
		t.Fatalf("checkAndScale: %v", err)
	}

	obs.mu.Lock()
	defer obs.mu.Unlock()
	if len(obs.cycles) != 1 || obs.cycles[0] != 1 || obs.evaluations[string(EvaluationError)] != 1 {
		t.Fatalf("observer saw cycles=%v evaluations=%v, want one cycle with one (errored) evaluation", obs.cycles, obs.evaluations)
	}
}
