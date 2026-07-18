package controller

import (
	"context"
	"fmt"
	"time"

	"truefoundry/elasti/operator/api/v1alpha1"

	"go.uber.org/zap"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// defaultReadyEndpointsTimeout bounds how long transitionToServe waits for the
	// target Service to have at least one ready endpoint before it gives up waiting
	// and removes the resolver hijack anyway.
	//
	// NOTE: this is intentionally a fixed, package-level value for now. Making this
	// configurable per-ElastiService (and running transitionToServe in parallel across
	// services) is tracked as a separate, smaller follow-up PR.
	defaultReadyEndpointsTimeout = 120 * time.Second

	// readyEndpointsPollInterval is how often waitForReadyEndpoints re-checks the
	// target Service's EndpointSlices while waiting.
	readyEndpointsPollInterval = 2 * time.Second

	// endpointSliceControllerManagedByValue is the well-known value the standard,
	// in-tree EndpointSlice controller sets on discoveryv1.LabelManagedBy for every
	// EndpointSlice it creates for a Service. It's part of the GA discovery.k8s.io/v1
	// API surface (stable since Kubernetes 1.21) but isn't exported as a Go constant
	// from k8s.io/api, since it's defined server-side in kube-controller-manager.
	endpointSliceControllerManagedByValue = "endpointslice-controller.k8s.io"
)

// waitForReadyEndpoints polls the given Service's EndpointSlices until at least one
// endpoint is Ready, or until timeout elapses. It reuses the same "ready" definition
// as k8shelper.Ops.CheckIfServiceEndpointSliceActive: an endpoint with Conditions.Ready
// == nil is treated as ready, matching Kubernetes' documented default.
//
// IMPORTANT: while a service is in proxy mode, createOrUpdateEndpointsliceToResolver
// creates a second EndpointSlice carrying the *same* `kubernetes.io/service-name` label
// as the real, Kubernetes-managed one -- pointing at the resolver, not the target pods.
// Its endpoints have no Conditions set, so by the same "nil == ready" rule they'd read
// as trivially ready. If we didn't exclude it, this function would report success
// immediately whenever the hijack slice is still present, i.e. exactly during the
// window transitionToServe calls this to guard -- defeating the wait entirely.
//
// We filter to endpointslice.kubernetes.io/managed-by=endpointslice-controller.k8s.io,
// the label the standard EndpointSlice controller stamps on every slice it creates
// (GA since EndpointSlices themselves went GA in Kubernetes 1.21, well within
// KubeElasti's supported range). The resolver's hijack slice never sets this label, so
// it's naturally excluded without needing to know its name or any other internal detail.
//
// NOTE: this assumes the cluster's Service endpoints are managed by the standard
// in-tree EndpointSlice controller. Some CNIs/service meshes (e.g. Cilium, Antrea) that
// manage EndpointSlices themselves may use a different managed-by value, in which case
// this filter would need to be revisited.
//
// It does not log anything itself -- the caller decides what to log and whether to
// treat a timeout as fatal.
func (r *ElastiServiceReconciler) waitForReadyEndpoints(ctx context.Context, namespacedName types.NamespacedName, timeout time.Duration) error {
	err := wait.PollUntilContextTimeout(ctx, readyEndpointsPollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		endpointSlices := &discoveryv1.EndpointSliceList{}
		if err := r.List(ctx, endpointSlices,
			client.InNamespace(namespacedName.Namespace),
			client.MatchingLabels{
				discoveryv1.LabelServiceName: namespacedName.Name,
				discoveryv1.LabelManagedBy:   endpointSliceControllerManagedByValue,
			},
		); err != nil {
			// Transient list errors shouldn't abort the whole wait; keep polling.
			return false, nil //nolint:nilerr
		}

		for _, slice := range endpointSlices.Items {
			for _, endpoint := range slice.Endpoints {
				if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
					return true, nil
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for ready endpoints for %s: %w", namespacedName.String(), err)
	}
	return nil
}

// transitionToServe is the single, reusable sequence for safely moving a service out
// of proxy mode: scale the real target back up, wait for it to actually be able to
// serve traffic, and only then remove the resolver hijack.
//
// Precondition: the caller must hold the SwitchModeLocks mutex for this ElastiService
// (switchMode already does this; finalizeCRD acquires it explicitly before calling in).
func (r *ElastiServiceReconciler) transitionToServe(ctx context.Context, es *v1alpha1.ElastiService) error {
	targetNamespacedName := types.NamespacedName{
		Name:      es.Spec.Service,
		Namespace: es.Namespace,
	}

	// Step 1: unpause KEDA (if applicable) and scale the real target back up to
	// MinTargetReplicas.
	if err := r.ScaleHandler.HandleScaleFromZero(ctx, es); err != nil {
		return fmt.Errorf("failed to scale target from zero: %w", err)
	}
	r.Logger.Info("1. Scaled target from zero", zap.String("service", targetNamespacedName.String()))

	// Step 2: wait for the target to actually be ready to serve traffic. On timeout
	// we deliberately proceed rather than abort -- the caller (CRD deletion or resolver
	// deletion) can't be stopped at this point anyway, so removing the hijack is still
	// the right move; we just make noise about it instead of failing silently.
	if err := r.waitForReadyEndpoints(ctx, targetNamespacedName, defaultReadyEndpointsTimeout); err != nil {
		r.Logger.Warn("Timed out waiting for target to become ready, proceeding to remove resolver hijack anyway",
			zap.String("service", targetNamespacedName.String()), zap.Error(err))
		r.ScaleHandler.EventRecorder.Eventf(es, "Warning", "ReadyTimeout",
			"Timed out after %s waiting for %s to become ready; removing resolver hijack anyway", defaultReadyEndpointsTimeout, targetNamespacedName.String())
	} else {
		r.Logger.Info("2. Target is ready to serve", zap.String("service", targetNamespacedName.String()))
	}

	// Step 3: remove the resolver hijack now that (or regardless of whether) traffic
	// can flow to the real target.
	if err := r.deleteEndpointsliceToResolver(ctx, targetNamespacedName); err != nil {
		return fmt.Errorf("failed to delete endpointslice to resolver: %w", err)
	}
	r.Logger.Info("3. Deleted endpointslice to resolver", zap.String("service", targetNamespacedName.String()))

	// Step 4: remove the private service used to route to the resolver.
	if err := r.deletePrivateService(ctx, targetNamespacedName); err != nil {
		return fmt.Errorf("failed to delete private service: %w", err)
	}
	r.Logger.Info("4. Deleted private service", zap.String("service", targetNamespacedName.String()))

	return nil
}
