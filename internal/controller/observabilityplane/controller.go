// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
)

const (
	// ObservabilityPlaneCleanupFinalizer is the finalizer that is used to clean up observabilityplane resources.
	ObservabilityPlaneCleanupFinalizer = "openchoreo.dev/observabilityplane-cleanup"
)

// Reconciler reconciles a ObservabilityPlane object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client
	CacheVersion  string // Cache key version prefix (e.g., "v2")
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ObservabilityPlane instance
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
	if err := r.Get(ctx, req.NamespacedName, observabilityPlane); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ObservabilityPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ObservabilityPlane")
		return ctrl.Result{}, err
	}

	if !observabilityPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing observabilityplane")
		return r.finalize(ctx, observabilityPlane)
	}

	if finalizerAdded, err := r.ensureFinalizer(ctx, observabilityPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the ObservabilityPlane CR (credentials, observerURL, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, observabilityPlane)
	}

	// Notify gateway of ObservabilityPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, observabilityPlane, "updated"); err != nil {
			// Don't fail reconciliation if gateway notification fails.
			// Rationale: Gateway notification is "best effort" - the system remains
			// eventually consistent through agent reconnection and cert verification.
			// Failing reconciliation would prevent CR status updates and requeue
			// indefinitely if gateway is temporarily unavailable.
			logger.Error(err, "Failed to notify gateway of ObservabilityPlane reconciliation")
		}
	}

	return ctrl.Result{}, nil
}

// ensureFinalizer ensures that the finalizer is added to the observabilityplane.
// The first return value indicates whether the finalizer was added to the observabilityplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, observabilityPlane *openchoreov1alpha1.ObservabilityPlane) (bool, error) {
	if !observabilityPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(observabilityPlane, ObservabilityPlaneCleanupFinalizer) {
		return true, r.Update(ctx, observabilityPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, observabilityPlane *openchoreov1alpha1.ObservabilityPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("observabilityplane", observabilityPlane.Name)

	if !controllerutil.ContainsFinalizer(observabilityPlane, ObservabilityPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of ObservabilityPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, observabilityPlane, "deleted"); err != nil {
			// Don't fail finalization if gateway notification fails.
			// Rationale:
			// 1. Gateway unavailability shouldn't block CR deletion (operational resilience)
			// 2. System is eventually consistent: when agent reconnects, gateway will
			//    query for the CR, find it doesn't exist, and reject the connection
			// 3. Prevents CRs from getting stuck in "Terminating" state indefinitely
			// Trade-off: If gateway is unreachable, agents may attempt reconnection
			// before discovering the CR is gone, wasting some resources temporarily.
			logger.Error(err, "Failed to notify gateway of ObservabilityPlane deletion")
		}
	}

	// Invalidate cached Kubernetes client before removing finalizer
	// This ensures the cache is cleaned up even if the ObservabilityPlane CR is deleted
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, observabilityPlane)
	}

	if controllerutil.RemoveFinalizer(observabilityPlane, ObservabilityPlaneCleanupFinalizer) {
		if err := r.Update(ctx, observabilityPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized observabilityplane")
	return ctrl.Result{}, nil
}

// invalidateCache invalidates the cached Kubernetes client for this ObservabilityPlane
func (r *Reconciler) invalidateCache(ctx context.Context, observabilityPlane *openchoreov1alpha1.ObservabilityPlane) {
	logger := log.FromContext(ctx).WithValues("observabilityplane", observabilityPlane.Name)

	// ObservabilityPlane uses a different cache key format: v2/observabilityplane/{namespace}/{name}
	// It doesn't include planeID in the path
	cacheKey := fmt.Sprintf("%s/observabilityplane/%s/%s", r.CacheVersion, observabilityPlane.Namespace, observabilityPlane.Name)
	r.ClientMgr.RemoveClient(cacheKey)

	logger.Info("Invalidated cached Kubernetes client for ObservabilityPlane",
		"cacheKey", cacheKey,
	)
}

// notifyGateway notifies the cluster gateway about ObservabilityPlane lifecycle events
func (r *Reconciler) notifyGateway(ctx context.Context, observabilityPlane *openchoreov1alpha1.ObservabilityPlane, event string) error {
	logger := log.FromContext(ctx).WithValues("observabilityplane", observabilityPlane.Name)

	effectivePlaneID := observabilityPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = observabilityPlane.Name
	}

	notification := &gatewayClient.PlaneNotification{
		PlaneType: "observabilityplane",
		PlaneID:   effectivePlaneID,
		Event:     event,
		Namespace: observabilityPlane.Namespace,
		Name:      observabilityPlane.Name,
	}

	logger.Info("notifying gateway of ObservabilityPlane lifecycle event",
		"event", event,
		"planeID", effectivePlaneID,
	)

	resp, err := r.GatewayClient.NotifyPlaneLifecycle(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to notify gateway: %w", err)
	}

	logger.Info("gateway notification successful",
		"event", event,
		"planeID", effectivePlaneID,
		"disconnectedAgents", resp.DisconnectedAgents,
	)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ObservabilityPlane{}).
		Named("observabilityplane").
		Complete(r)
}
