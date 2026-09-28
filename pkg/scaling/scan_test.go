package scaling

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"truefoundry/elasti/operator/api/v1alpha1"
)

// scanner contract:
//   - evaluations run in parallel, at most concurrency at a time;
//   - each evaluation is bounded by evaluationTimeout, the action it decides on is not;
//   - actions run asynchronously: run returns once every service is evaluated;
//   - a target whose action is still in flight is not evaluated again;
//   - at most scaleDownConcurrency scale-downs run at once (scale-ups are not limited);
//   - the observer sees every cycle, evaluation and action.

// waitForActions blocks until every dispatched scale action has finished.
func (s *scanner) waitForActions() { s.actions.Wait() }

func deploymentES(name string) v1alpha1.ElastiService {
	es := v1alpha1.ElastiService{}
	es.Namespace = "team-a"
	es.Name = name
	es.Spec.Service = name
	es.Spec.ScaleTargetRef = v1alpha1.ScaleTargetRef{APIVersion: "apps/v1", Kind: "Deployment", Name: name}
	return es
}

func deploymentESs(n int) []v1alpha1.ElastiService {
	services := make([]v1alpha1.ElastiService, n)
	for i := range services {
		services[i] = deploymentES(string(rune('a' + i)))
	}
	return services
}

func always(direction ScaleDirection) evaluateFunc {
	return func(context.Context, *v1alpha1.ElastiService) (ScaleDirection, error) { return direction, nil }
}

func noAction(context.Context, *v1alpha1.ElastiService, ScaleDirection) error { return nil }

// peakTracker records the highest number of concurrent holders.
type peakTracker struct{ current, peak atomic.Int32 }

func (p *peakTracker) enter() {
	n := p.current.Add(1)
	for {
		old := p.peak.Load()
		if n <= old || p.peak.CompareAndSwap(old, n) {
			return
		}
	}
}
func (p *peakTracker) leave() { p.current.Add(-1) }

func TestScannerEvaluatesInParallelUpToConcurrency(t *testing.T) {
	s := newScanner(scanConfig{concurrency: 4})
	var evals peakTracker
	evaluate := func(context.Context, *v1alpha1.ElastiService) (ScaleDirection, error) {
		evals.enter()
		defer evals.leave()
		time.Sleep(30 * time.Millisecond)
		return NoScale, nil
	}

	s.run(context.Background(), deploymentESs(16), evaluate, noAction)

	if got := evals.peak.Load(); got != 4 {
		t.Fatalf("peak concurrent evaluations = %d, want 4", got)
	}
}

func TestScannerEvaluationTimeoutIsolatesSlowService(t *testing.T) {
	s := newScanner(scanConfig{concurrency: 2, evaluationTimeout: 100 * time.Millisecond})
	var slowErr error
	var evaluated atomic.Int32
	evaluate := func(ctx context.Context, es *v1alpha1.ElastiService) (ScaleDirection, error) {
		evaluated.Add(1)
		if es.Name == "a" {
			<-ctx.Done()
			slowErr = ctx.Err()
			return "", ctx.Err()
		}
		return NoScale, nil
	}

	done := make(chan struct{})
	go func() {
		s.run(context.Background(), deploymentESs(5), evaluate, noAction)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a hung evaluation held up the cycle past its timeout")
	}
	if !errors.Is(slowErr, context.DeadlineExceeded) {
		t.Fatalf("slow evaluation ended with %v, want its own deadline", slowErr)
	}
	if got := evaluated.Load(); got != 5 {
		t.Fatalf("%d services evaluated, want all 5", got)
	}
}

func TestScannerActionNotBoundByEvaluationTimeout(t *testing.T) {
	s := newScanner(scanConfig{concurrency: 1, evaluationTimeout: 20 * time.Millisecond})
	actionErr := make(chan error, 1)
	act := func(ctx context.Context, _ *v1alpha1.ElastiService, _ ScaleDirection) error {
		time.Sleep(100 * time.Millisecond) // a save outlasting the evaluation budget
		actionErr <- ctx.Err()
		return nil
	}

	s.run(context.Background(), deploymentESs(1), always(ScaleDown), act)
	s.waitForActions()

	if err := <-actionErr; err != nil {
		t.Fatalf("action context ended with %v; it must not inherit the evaluation timeout", err)
	}
}

func TestScannerReturnsWithoutWaitingForActions(t *testing.T) {
	s := newScanner(scanConfig{concurrency: 1})
	release := make(chan struct{})
	act := func(context.Context, *v1alpha1.ElastiService, ScaleDirection) error {
		<-release
		return nil
	}

	done := make(chan struct{})
	go func() {
		s.run(context.Background(), deploymentESs(2), always(ScaleDown), act)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run waited for its scale actions")
	}
	close(release)
	s.waitForActions()
}

func TestScannerSkipsTargetWithActionInFlight(t *testing.T) {
	s := newScanner(scanConfig{concurrency: 1})
	var evaluations atomic.Int32
	evaluate := func(context.Context, *v1alpha1.ElastiService) (ScaleDirection, error) {
		evaluations.Add(1)
		return ScaleDown, nil
	}
	release := make(chan struct{})
	act := func(context.Context, *v1alpha1.ElastiService, ScaleDirection) error {
		<-release
		return nil
	}
	services := deploymentESs(1)

	s.run(context.Background(), services, evaluate, act) // dispatches, stays in flight
	s.run(context.Background(), services, evaluate, act) // must skip it
	if got := evaluations.Load(); got != 1 {
		t.Fatalf("%d evaluations while the first action was in flight, want 1", got)
	}

	close(release)
	s.waitForActions()
	s.run(context.Background(), services, evaluate, noAction) // evaluated again once done
	if got := evaluations.Load(); got != 2 {
		t.Fatalf("%d evaluations after the action finished, want 2", got)
	}
}

func TestScannerBoundsConcurrentScaleDownsOnly(t *testing.T) {
	for _, tt := range []struct {
		direction ScaleDirection
		wantPeak  int32
	}{
		{ScaleDown, 2},
		{ScaleUp, 8},
	} {
		t.Run(string(tt.direction), func(t *testing.T) {
			s := newScanner(scanConfig{concurrency: 8, scaleDownConcurrency: 2})
			var actions peakTracker
			act := func(context.Context, *v1alpha1.ElastiService, ScaleDirection) error {
				actions.enter()
				defer actions.leave()
				time.Sleep(40 * time.Millisecond)
				return nil
			}

			s.run(context.Background(), deploymentESs(8), always(tt.direction), act)
			s.waitForActions()

			if got := actions.peak.Load(); got != tt.wantPeak {
				t.Fatalf("peak concurrent %s actions = %d, want %d", tt.direction, got, tt.wantPeak)
			}
		})
	}
}

func TestScannerDropsQueuedScaleDownsOnCancel(t *testing.T) {
	s := newScanner(scanConfig{concurrency: 2, scaleDownConcurrency: 1})
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	var ran atomic.Int32
	act := func(context.Context, *v1alpha1.ElastiService, ScaleDirection) error {
		ran.Add(1)
		started <- struct{}{}
		<-release
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())

	s.run(ctx, deploymentESs(2), always(ScaleDown), act)
	<-started // one scale-down holds the only slot; the other is queued
	cancel()
	close(release)
	s.waitForActions()

	if got := ran.Load(); got != 1 {
		t.Fatalf("%d scale-downs ran, want 1: the queued one must be dropped once cancelled", got)
	}
}

func TestScannerActsOnceForServicesSharingATarget(t *testing.T) {
	s := newScanner(scanConfig{concurrency: 2})
	var acted atomic.Int32
	act := func(context.Context, *v1alpha1.ElastiService, ScaleDirection) error {
		acted.Add(1)
		time.Sleep(20 * time.Millisecond)
		return nil
	}
	twin := deploymentES("a")
	twin.Name = "a-twin" // a second ElastiService on the same Deployment

	s.run(context.Background(), []v1alpha1.ElastiService{deploymentES("a"), twin}, always(ScaleDown), act)
	s.waitForActions()

	if got := acted.Load(); got != 1 {
		t.Fatalf("%d actions on one target, want 1", got)
	}
}

func TestScannerDoesNotEvaluateAfterCancel(t *testing.T) {
	s := newScanner(scanConfig{concurrency: 1})
	var evaluations atomic.Int32
	evaluate := func(context.Context, *v1alpha1.ElastiService) (ScaleDirection, error) {
		evaluations.Add(1)
		return NoScale, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.run(ctx, deploymentESs(3), evaluate, noAction)

	if got := evaluations.Load(); got != 0 {
		t.Fatalf("%d evaluations after the watcher was cancelled, want 0", got)
	}
}

// recordingObserver counts what the scanner reports.
type recordingObserver struct {
	mu          sync.Mutex
	cycles      []int
	evaluations map[string]int
	started     map[ScaleDirection]int
	finished    map[ScaleDirection][]error
}

func newRecordingObserver() *recordingObserver {
	return &recordingObserver{evaluations: map[string]int{}, started: map[ScaleDirection]int{}, finished: map[ScaleDirection][]error{}}
}

func (o *recordingObserver) CycleCompleted(_ time.Duration, services int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cycles = append(o.cycles, services)
}
func (o *recordingObserver) Evaluated(result EvaluationResult, _ time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.evaluations[string(result)]++
}
func (o *recordingObserver) ActionStarted(direction ScaleDirection) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.started[direction]++
}
func (o *recordingObserver) ActionFinished(direction ScaleDirection, _ time.Duration, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.finished[direction] = append(o.finished[direction], err)
}

func TestScannerReportsToObserver(t *testing.T) {
	obs := newRecordingObserver()
	s := newScanner(scanConfig{concurrency: 2})
	s.observer = obs
	actErr := errors.New("scale failed")
	evaluate := func(_ context.Context, es *v1alpha1.ElastiService) (ScaleDirection, error) {
		switch es.Name {
		case "a":
			return ScaleDown, nil
		case "b":
			return NoScale, nil
		default:
			return "", errors.New("prometheus down")
		}
	}
	act := func(context.Context, *v1alpha1.ElastiService, ScaleDirection) error { return actErr }

	s.run(context.Background(), deploymentESs(3), evaluate, act)
	s.waitForActions()

	obs.mu.Lock()
	defer obs.mu.Unlock()
	if len(obs.cycles) != 1 || obs.cycles[0] != 3 {
		t.Fatalf("cycles = %v, want one cycle over 3 services", obs.cycles)
	}
	want := map[string]int{string(EvaluationScaleDown): 1, string(EvaluationNoScale): 1, string(EvaluationError): 1}
	for result, n := range want {
		if obs.evaluations[result] != n {
			t.Fatalf("evaluations = %v, want %v", obs.evaluations, want)
		}
	}
	if obs.started[ScaleDown] != 1 || len(obs.finished[ScaleDown]) != 1 || !errors.Is(obs.finished[ScaleDown][0], actErr) {
		t.Fatalf("actions started=%v finished=%v, want one failed scale-down", obs.started, obs.finished)
	}
}
