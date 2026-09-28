package scaling

import (
	"context"
	"fmt"
	"sync"

	"github.com/truefoundry/elasti/pkg/scaling/scalers"
)

// healthMemo runs each scaler health check at most once per scale-down
// cycle: scalers with the same scalers.HealthKeyer key share the answer,
// including an error. Safe for concurrent use.
type healthMemo struct {
	mu      sync.Mutex
	results map[string]*healthResult
}

type healthResult struct {
	once    sync.Once
	healthy bool
	err     error
}

func newHealthMemo() *healthMemo {
	return &healthMemo{results: map[string]*healthResult{}}
}

// isHealthy returns the scaler's health, checking each key once; the first
// caller's ctx runs the shared check.
func (m *healthMemo) isHealthy(ctx context.Context, scaler scalers.Scaler) (bool, error) {
	keyer, ok := scaler.(scalers.HealthKeyer)
	if !ok || keyer.HealthKey() == "" {
		return checkHealth(ctx, scaler)
	}
	key := keyer.HealthKey()

	m.mu.Lock()
	result, ok := m.results[key]
	if !ok {
		result = &healthResult{}
		m.results[key] = result
	}
	m.mu.Unlock()

	result.once.Do(func() { result.healthy, result.err = checkHealth(ctx, scaler) })
	return result.healthy, result.err
}

func checkHealth(ctx context.Context, scaler scalers.Scaler) (bool, error) {
	healthy, err := scaler.IsHealthy(ctx)
	if err != nil {
		return false, fmt.Errorf("scaler health check: %w", err)
	}
	return healthy, nil
}
