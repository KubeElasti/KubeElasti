package scalers

import (
	"context"
)

type Scaler interface {
	IsHealthy(ctx context.Context) (bool, error)
	ShouldScaleToZero(ctx context.Context) (bool, error)
	ShouldScaleFromZero(ctx context.Context) (bool, error)
	Close(ctx context.Context) error
}

// HealthKeyer is implemented by scalers whose IsHealthy answer is shared by
// every scaler with the same key (same backend, credentials and health
// query), so the scale-down loop can check each key once per cycle. An empty
// key opts out.
type HealthKeyer interface {
	HealthKey() string
}
