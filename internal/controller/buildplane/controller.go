// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

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
	BuildPlaneCleanupFinalizer = "openchoreo.dev/buildplane-cleanup"
)

// BuildPlaneReconciler reconciles a BuildPlane object
type BuildPlaneReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client
	CacheVersion  string // Cache key version prefix (e.g., "v2")
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=buildplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=buildplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=buildplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BuildPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	buildPlane := &openchoreov1alpha1.BuildPlane{}
	if err := r.Get(ctx, req.NamespacedName, buildPlane); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("BuildPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get BuildPlane")
		return ctrl.Result{}, err
	}

	if !buildPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing buildplane")
		return r.finalize(ctx, buildPlane)
	}

	if finalizerAdded, err := r.ensureFinalizer(ctx, buildPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the BuildPlane CR (kubeconfig, credentials, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, buildPlane)
	}

	// Notify gateway of BuildPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, buildPlane, "updated"); err != nil {
			// Don't fail reconciliation if gateway notification fails.
			// Rationale: Gateway notification is "best effort" - the system remains
			// eventually consistent through agent reconnection and cert verification.
			// Failing reconciliation would prevent CR status updates and requeue
			// indefinitely if gateway is temporarily unavailable.
			logger.Error(err, "Failed to notify gateway of BuildPlane reconciliation")
		}
	}

	return ctrl.Result{}, nil
}

// ensureFinalizer ensures that the finalizer is added to the buildplane.
// The first return value indicates whether the finalizer was added to the buildplane.
func (r *BuildPlaneReconciler) ensureFinalizer(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane) (bool, error) {
	if !buildPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(buildPlane, BuildPlaneCleanupFinalizer) {
		return true, r.Update(ctx, buildPlane)
	}

	return false, nil
}

func (r *BuildPlaneReconciler) finalize(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("buildplane", buildPlane.Name)

	if !controllerutil.ContainsFinalizer(buildPlane, BuildPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of BuildPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, buildPlane, "deleted"); err != nil {
			// Don't fail finalization if gateway notification fails.
			// Rationale:
			// 1. Gateway unavailability shouldn't block CR deletion (operational resilience)
			// 2. System is eventually consistent: when agent reconnects, gateway will
			//    query for the CR, find it doesn't exist, and reject the connection
			// 3. Prevents CRs from getting stuck in "Terminating" state indefinitely
			// Trade-off: If gateway is unreachable, agents may attempt reconnection
			// before discovering the CR is gone, wasting some resources temporarily.
			logger.Error(err, "Failed to notify gateway of BuildPlane deletion")
		}
	}

	// Invalidate cached Kubernetes client before removing finalizer
	// This ensures the cache is cleaned up even if the BuildPlane CR is deleted
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, buildPlane)
	}

	if controllerutil.RemoveFinalizer(buildPlane, BuildPlaneCleanupFinalizer) {
		if err := r.Update(ctx, buildPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized buildplane")
	return ctrl.Result{}, nil
}

// invalidateCache invalidates the cached Kubernetes client for this BuildPlane
func (r *BuildPlaneReconciler) invalidateCache(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane) {
	logger := log.FromContext(ctx).WithValues("buildplane", buildPlane.Name)

	effectivePlaneID := buildPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = buildPlane.Name
	}

	// Cache key format: v2/buildplane/{planeID}/{namespace}/{name}
	cacheKey := fmt.Sprintf("%s/buildplane/%s/%s/%s", r.CacheVersion, effectivePlaneID, buildPlane.Namespace, buildPlane.Name)
	r.ClientMgr.RemoveClient(cacheKey)

	if effectivePlaneID != buildPlane.Name {
		fallbackKey := fmt.Sprintf("%s/buildplane/%s/%s/%s", r.CacheVersion, buildPlane.Name, buildPlane.Namespace, buildPlane.Name)
		r.ClientMgr.RemoveClient(fallbackKey)
	}

	logger.Info("Invalidated cached Kubernetes client for BuildPlane",
		"planeID", effectivePlaneID,
		"cacheKey", cacheKey,
	)
}

// notifyGateway notifies the cluster gateway about BuildPlane lifecycle events
func (r *BuildPlaneReconciler) notifyGateway(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane, event string) error {
	logger := log.FromContext(ctx).WithValues("buildplane", buildPlane.Name)

	effectivePlaneID := buildPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = buildPlane.Name
	}

	notification := &gatewayClient.PlaneNotification{
		PlaneType: "buildplane",
		PlaneID:   effectivePlaneID,
		Event:     event,
		Namespace: buildPlane.Namespace,
		Name:      buildPlane.Name,
	}

	logger.Info("notifying gateway of BuildPlane lifecycle event",
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
func (r *BuildPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.BuildPlane{}).
		Named("buildplane").
		Complete(r)
}
