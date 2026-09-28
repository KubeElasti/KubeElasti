package scaling

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"truefoundry/elasti/operator/api/v1alpha1"

	"go.uber.org/zap"
)

// EvaluationResult is the outcome of evaluating one ElastiService in a scan.
type EvaluationResult string

const (
	EvaluationScaleDown EvaluationResult = "scale_down"
	EvaluationScaleUp   EvaluationResult = "scale_up"
	EvaluationNoScale   EvaluationResult = "no_scale"
	EvaluationError     EvaluationResult = "error"
	// EvaluationSkipped: the target still has a scale action in flight.
	EvaluationSkipped EvaluationResult = "skipped_in_flight"
)

// ScanObserver receives the scale-down scan's measurements, e.g. to export
// them as metrics.
type ScanObserver interface {
	CycleCompleted(duration time.Duration, services int)
	Evaluated(result EvaluationResult, duration time.Duration)
	ActionStarted(direction ScaleDirection)
	ActionFinished(direction ScaleDirection, duration time.Duration, err error)
}

type noopScanObserver struct{}

func (noopScanObserver) CycleCompleted(time.Duration, int)                   {}
func (noopScanObserver) Evaluated(EvaluationResult, time.Duration)           {}
func (noopScanObserver) ActionStarted(ScaleDirection)                        {}
func (noopScanObserver) ActionFinished(ScaleDirection, time.Duration, error) {}

// scanConfig tunes the scale-down scan. Zero values mean one evaluation at
// a time, no evaluation timeout and no limit on concurrent scale-downs.
type scanConfig struct {
	// concurrency is how many ElastiServices are evaluated at once.
	concurrency int
	// evaluationTimeout bounds one evaluation (its Prometheus queries), not
	// the scale action it decides on.
	evaluationTimeout time.Duration
	// scaleDownConcurrency bounds scale-downs running at once.
	scaleDownConcurrency int
}

type evaluateFunc func(ctx context.Context, es *v1alpha1.ElastiService) (ScaleDirection, error)

type actFunc func(ctx context.Context, es *v1alpha1.ElastiService, direction ScaleDirection) error

// scanner evaluates ElastiServices for the scale-down loop in parallel,
// each evaluation under its own timeout, and dispatches the scale actions
// they decide on asynchronously, so a slow action (a long save, a slow API
// server) never holds up evaluating the others. A target whose action is
// still in flight is skipped until it finishes.
type scanner struct {
	cfg            scanConfig
	observer       ScanObserver
	scaleDownSlots chan struct{} // nil: no limit

	mu       sync.Mutex
	inFlight map[string]struct{}
	actions  sync.WaitGroup // dispatched actions still running
}

func newScanner(cfg scanConfig) *scanner {
	s := &scanner{cfg: cfg, observer: noopScanObserver{}, inFlight: map[string]struct{}{}}
	if cfg.scaleDownConcurrency > 0 {
		s.scaleDownSlots = make(chan struct{}, cfg.scaleDownConcurrency)
	}
	return s
}

// run evaluates services and returns once each is evaluated; the scale
// actions run on ctx and may still be in flight.
func (s *scanner) run(ctx context.Context, services []v1alpha1.ElastiService, evaluate evaluateFunc, act actFunc) {
	start := time.Now()
	workers := make(chan struct{}, max(s.cfg.concurrency, 1))
	var evaluations sync.WaitGroup
	for i := range services {
		if ctx.Err() != nil {
			break
		}
		es := &services[i]
		key := targetKey(es)
		if s.isInFlight(key) {
			s.observer.Evaluated(EvaluationSkipped, 0)
			continue
		}
		workers <- struct{}{}
		evaluations.Add(1)
		go func() {
			defer evaluations.Done()
			defer func() { <-workers }()
			direction, ok := s.evaluate(ctx, es, evaluate)
			if ok && direction != NoScale {
				s.dispatch(ctx, key, es, direction, act)
			}
		}()
	}
	evaluations.Wait()
	s.observer.CycleCompleted(time.Since(start), len(services))
}

func (s *scanner) evaluate(ctx context.Context, es *v1alpha1.ElastiService, evaluate evaluateFunc) (ScaleDirection, bool) {
	if s.cfg.evaluationTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.cfg.evaluationTimeout)
		defer cancel()
	}
	start := time.Now()
	direction, err := evaluate(ctx, es)
	result := EvaluationError
	switch {
	case err != nil:
	case direction == ScaleDown:
		result = EvaluationScaleDown
	case direction == ScaleUp:
		result = EvaluationScaleUp
	default:
		result = EvaluationNoScale
	}
	s.observer.Evaluated(result, time.Since(start))
	return direction, err == nil
}

// dispatch runs act for es in the background unless its target already has
// an action in flight.
func (s *scanner) dispatch(ctx context.Context, key string, es *v1alpha1.ElastiService, direction ScaleDirection, act actFunc) {
	if !s.claim(key) {
		return
	}
	s.actions.Add(1)
	go func() {
		defer s.actions.Done()
		defer s.release(key)
		if direction == ScaleDown && s.scaleDownSlots != nil {
			select {
			case s.scaleDownSlots <- struct{}{}:
				defer func() { <-s.scaleDownSlots }()
			case <-ctx.Done():
				return
			}
			if ctx.Err() != nil { // cancelled while queued; the slot just freed up
				return
			}
		}
		s.observer.ActionStarted(direction)
		start := time.Now()
		err := act(ctx, es, direction)
		s.observer.ActionFinished(direction, time.Since(start), err)
	}()
}

func (s *scanner) isInFlight(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inFlight[key]
	return ok
}

func (s *scanner) claim(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.inFlight[key]; ok {
		return false
	}
	s.inFlight[key] = struct{}{}
	return true
}

func (s *scanner) release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, key)
}

// targetKey identifies the scale target, matching getMutexForScale's key.
func targetKey(es *v1alpha1.ElastiService) string {
	ref := es.Spec.GetScaleTargetRef()
	return es.Namespace + "/" + ref.Kind + "/" + ref.Name
}

const (
	defaultScanConcurrency       = 8
	defaultScanEvaluationTimeout = 20 * time.Second
	defaultScaleDownConcurrency  = 8
)

// scanConfigFromEnv reads SCAN_CONCURRENCY, SCAN_EVALUATION_TIMEOUT and
// SCALE_DOWN_CONCURRENCY, falling back to the default (with a warning) for
// a value that is unparsable or not positive.
func scanConfigFromEnv(logger *zap.Logger) scanConfig {
	return scanConfig{
		concurrency:          positiveIntFromEnv(logger, "SCAN_CONCURRENCY", defaultScanConcurrency),
		evaluationTimeout:    positiveDurationFromEnv(logger, "SCAN_EVALUATION_TIMEOUT", defaultScanEvaluationTimeout),
		scaleDownConcurrency: positiveIntFromEnv(logger, "SCALE_DOWN_CONCURRENCY", defaultScaleDownConcurrency),
	}
}

func positiveIntFromEnv(logger *zap.Logger, name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		logger.Warn("invalid value, using default", zap.String("env", name), zap.String("value", raw), zap.Int("default", def))
		return def
	}
	return n
}

func positiveDurationFromEnv(logger *zap.Logger, name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		logger.Warn("invalid value, using default", zap.String("env", name), zap.String("value", raw), zap.Duration("default", def))
		return def
	}
	return d
}
