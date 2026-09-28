package scaling

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// ScaleDownWatcher contract: it runs the check every interval, measured from
// the end of the previous check (a slow check never causes back-to-back
// runs), and returns when its context is cancelled.

func TestScaleDownWatcherRunsCheckRepeatedly(t *testing.T) {
	var runs atomic.Int32
	w := NewScaleDownWatcher(zap.NewNop(), 10*time.Millisecond, func(context.Context) error {
		runs.Add(1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Start(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for runs.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("check ran %d times in 2s at a 10ms interval, want >= 3", runs.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestScaleDownWatcherStartReturnsWhenContextCancelled(t *testing.T) {
	w := NewScaleDownWatcher(zap.NewNop(), time.Hour, func(context.Context) error { return nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	// Start must still be running (blocking) until cancelled.
	select {
	case err := <-done:
		t.Fatalf("Start returned %v before its context was cancelled", err)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned %v on cancel, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return after its context was cancelled")
	}
}

func TestScaleDownWatcherWaitsFullIntervalAfterSlowCheck(t *testing.T) {
	const interval, checkTime = 30 * time.Millisecond, 60 * time.Millisecond
	var mu sync.Mutex
	var starts []time.Time
	w := NewScaleDownWatcher(zap.NewNop(), interval, func(context.Context) error {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		time.Sleep(checkTime)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = w.Start(ctx) }()
	time.Sleep(10 * (interval + checkTime))
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if len(starts) < 3 {
		t.Fatalf("check ran %d times, want >= 3", len(starts))
	}
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < interval+checkTime {
			t.Fatalf("checks %d and %d started %v apart, want >= %v (check time + interval)", i-1, i, gap, interval+checkTime)
		}
	}
}

func TestScaleDownWatcherKeepsRunningAfterCheckError(t *testing.T) {
	var runs atomic.Int32
	w := NewScaleDownWatcher(zap.NewNop(), 10*time.Millisecond, func(context.Context) error {
		runs.Add(1)
		return errors.New("prometheus unreachable")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Start(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for runs.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("check ran %d times after failing, want the watcher to keep going (>= 3)", runs.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
