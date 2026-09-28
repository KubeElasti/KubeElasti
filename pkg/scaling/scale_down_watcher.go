package scaling

import (
	"context"
	"os"
	"time"

	"go.uber.org/zap"
)

const defaultPollingInterval = 30 * time.Second

// ScaleDownWatcher runs the scale-down check every polling interval. It is a
// controller-runtime Runnable that needs leader election: added to the
// manager, it runs only on the leader and stops with the manager.
type ScaleDownWatcher struct {
	logger   *zap.Logger
	interval time.Duration
	check    func(ctx context.Context) error
}

// NewScaleDownWatcher returns a watcher that calls check every interval.
func NewScaleDownWatcher(logger *zap.Logger, interval time.Duration, check func(ctx context.Context) error) *ScaleDownWatcher {
	return &ScaleDownWatcher{logger: logger, interval: interval, check: check}
}

// ScaleDownWatcher returns the handler's scale-down watcher, polling every
// POLLING_INTERVAL (default 30s).
func (h *ScaleHandler) ScaleDownWatcher() *ScaleDownWatcher {
	return NewScaleDownWatcher(h.logger, pollingIntervalFromEnv(h.logger), h.checkAndScale)
}

// Start runs the check every interval, measured from the end of the
// previous check, until ctx is cancelled.
func (w *ScaleDownWatcher) Start(ctx context.Context) error {
	timer := time.NewTimer(w.interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		if err := w.check(ctx); err != nil {
			w.logger.Error("failed to run the scale down check", zap.Error(err))
		}
		timer.Reset(w.interval)
	}
}

// NeedLeaderElection makes the manager run the watcher only while it holds
// the leader lease; two watchers would race to scale the same targets.
func (w *ScaleDownWatcher) NeedLeaderElection() bool { return true }

// pollingIntervalFromEnv reads POLLING_INTERVAL, falling back to 30s when it
// is unset, unparsable or not positive.
func pollingIntervalFromEnv(logger *zap.Logger) time.Duration {
	envInterval := os.Getenv("POLLING_INTERVAL")
	if envInterval == "" {
		return defaultPollingInterval
	}
	duration, err := time.ParseDuration(envInterval)
	switch {
	case err != nil:
		logger.Warn("Invalid POLLING_INTERVAL value, using default 30s", zap.Error(err))
		return defaultPollingInterval
	case duration <= 0:
		logger.Warn("POLLING_INTERVAL must be positive, using default 30s", zap.String("value", envInterval))
		return defaultPollingInterval
	default:
		return duration
	}
}
