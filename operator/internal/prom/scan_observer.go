package prom

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/truefoundry/elasti/pkg/scaling"
)

var (
	ScanCycleDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "elasti_operator_scan_cycle_duration_seconds",
			Help:    "Time to evaluate every ElastiService in one scale-down cycle (scale actions run asynchronously and are not included)",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 20, 30, 60, 120},
		},
	)

	ScanServices = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "elasti_operator_scan_services",
			Help: "ElastiServices in the last scale-down cycle",
		},
	)

	ScanEvaluations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "elasti_operator_scan_evaluations_total",
			Help: "ElastiService evaluations in the scale-down scan, by result",
		},
		[]string{"result"},
	)

	ScanEvaluationDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "elasti_operator_scan_evaluation_duration_seconds",
			Help:    "Time to evaluate one ElastiService (its scaler queries) in the scale-down scan",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 20},
		},
	)

	ScaleActionsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "elasti_operator_scale_actions_in_flight",
			Help: "Scale actions dispatched by the scale-down scan that are still running, by direction",
		},
		[]string{"direction"},
	)

	ScaleActionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "elasti_operator_scale_action_duration_seconds",
			Help:    "Duration of scale actions dispatched by the scale-down scan, by direction and result",
			Buckets: []float64{0.1, 0.5, 1, 5, 15, 30, 60, 120, 300, 600, 1800},
		},
		[]string{"direction", "result"},
	)
)

// ScanObserver exports the scale-down scan's measurements as the metrics
// above.
type ScanObserver struct{}

var _ scaling.ScanObserver = ScanObserver{}

func (ScanObserver) CycleCompleted(duration time.Duration, services int) {
	ScanCycleDuration.Observe(duration.Seconds())
	ScanServices.Set(float64(services))
}

func (ScanObserver) Evaluated(result scaling.EvaluationResult, duration time.Duration) {
	ScanEvaluations.WithLabelValues(string(result)).Inc()
	if result != scaling.EvaluationSkipped {
		ScanEvaluationDuration.Observe(duration.Seconds())
	}
}

func (ScanObserver) ActionStarted(direction scaling.ScaleDirection) {
	ScaleActionsInFlight.WithLabelValues(string(direction)).Inc()
}

func (ScanObserver) ActionFinished(direction scaling.ScaleDirection, duration time.Duration, err error) {
	ScaleActionsInFlight.WithLabelValues(string(direction)).Dec()
	result := "success"
	if err != nil {
		result = "failure"
	}
	ScaleActionDuration.WithLabelValues(string(direction), result).Observe(duration.Seconds())
}
