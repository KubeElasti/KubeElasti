package controller

import (
	"context"
	"fmt"

	"github.com/truefoundry/elasti/pkg/scaling"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"truefoundry/elasti/operator/api/v1alpha1"
)

// CachedElastiServiceLister lists ElastiServices from reader, normally the
// manager's informer cache, which is already scoped to the watched
// namespaces.
func CachedElastiServiceLister(reader client.Reader) scaling.ElastiServiceLister {
	return func(ctx context.Context) ([]v1alpha1.ElastiService, error) {
		list := &v1alpha1.ElastiServiceList{}
		if err := reader.List(ctx, list); err != nil {
			return nil, fmt.Errorf("list ElastiServices: %w", err)
		}
		return list.Items, nil
	}
}
