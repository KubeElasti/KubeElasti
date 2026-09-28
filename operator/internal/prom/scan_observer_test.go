package prom

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/truefoundry/elasti/pkg/scaling"
)

// The metrics are process-global, so each test asserts on deltas.

func histogramCount(t *testing.T, h prometheus.Histogram) uint64 {
	t.Helper()
	m := &dto.Metric{}
	if err := h.Write(m); err != nil {
		t.Fatalf("read histogram: %v", err)
	}
	return m.GetHistogram().GetSampleCount()
}

func value(t *testing.T, c prometheus.Collector) float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 1)
	c.Collect(ch)
	m := &dto.Metric{}
	if err := (<-ch).Write(m); err != nil {
		t.Fatalf("read metric: %v", err)
	}
	if m.GetGauge() != nil {
		return m.GetGauge().GetValue()
	}
	return m.GetCounter().GetValue()
}

func histogramVecCount(t *testing.T, direction, result string) uint64 {
	t.Helper()
	h, err := ScaleActionDuration.GetMetricWithLabelValues(direction, result)
	if err != nil {
		t.Fatalf("scale action histogram: %v", err)
	}
	return histogramCount(t, h.(prometheus.Histogram))
}

func TestScanObserverCycleCompleted(t *testing.T) {
	samples := histogramCount(t, ScanCycleDuration)

	ScanObserver{}.CycleCompleted(3*time.Second, 7)

	if got := value(t, ScanServices); got != 7 {
		t.Fatalf("elasti_operator_scan_services = %v, want 7", got)
	}
	if got := histogramCount(t, ScanCycleDuration); got != samples+1 {
		t.Fatalf("cycle duration samples = %d, want %d", got, samples+1)
	}
}

func TestScanObserverEvaluated(t *testing.T) {
	down := ScanEvaluations.WithLabelValues(string(scaling.EvaluationScaleDown))
	skipped := ScanEvaluations.WithLabelValues(string(scaling.EvaluationSkipped))
	downBefore, skippedBefore := value(t, down), value(t, skipped)
	samples := histogramCount(t, ScanEvaluationDuration)

	ScanObserver{}.Evaluated(scaling.EvaluationScaleDown, 50*time.Millisecond)
	ScanObserver{}.Evaluated(scaling.EvaluationSkipped, 0)

	if got := value(t, down) - downBefore; got != 1 {
		t.Fatalf("scale_down evaluations +%v, want +1", got)
	}
	if got := value(t, skipped) - skippedBefore; got != 1 {
		t.Fatalf("skipped evaluations +%v, want +1", got)
	}
	// A skipped target was not evaluated, so it has no duration.
	if got := histogramCount(t, ScanEvaluationDuration); got != samples+1 {
		t.Fatalf("evaluation duration samples = %d, want %d", got, samples+1)
	}
}

func TestScanObserverActions(t *testing.T) {
	inFlight := ScaleActionsInFlight.WithLabelValues(string(scaling.ScaleDown))
	base := value(t, inFlight)
	failed := histogramVecCount(t, "scaledown", "failure")

	ScanObserver{}.ActionStarted(scaling.ScaleDown)
	if got := value(t, inFlight) - base; got != 1 {
		t.Fatalf("scale-downs in flight +%v after start, want +1", got)
	}
	ScanObserver{}.ActionFinished(scaling.ScaleDown, 20*time.Second, errors.New("save failed"))
	if got := value(t, inFlight) - base; got != 0 {
		t.Fatalf("scale-downs in flight +%v after finish, want back to +0", got)
	}
	if got := histogramVecCount(t, "scaledown", "failure"); got != failed+1 {
		t.Fatalf("failed scale-down duration samples = %d, want %d", got, failed+1)
	}
}
