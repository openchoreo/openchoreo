// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ObservabilityPlaneCleanupFinalizer is the finalizer that is used to clean up observabilityplane resources.
	ObservabilityPlaneCleanupFinalizer = "openchoreo.dev/observabilityplane-cleanup"
)

// Reconciler reconciles a ObservabilityPlane object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client // Client for notifying cluster-gateway
	CacheVersion  string                // Cache key version prefix (e.g., "v2")
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

	// Keep a copy of the old ObservabilityPlane object
	old := observabilityPlane.DeepCopy()

	// Handle the deletion of the observabilityplane
	if !observabilityPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing observabilityplane")
		return r.finalize(ctx, old, observabilityPlane)
	}

	// Ensure the finalizer is added to the observabilityplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, observabilityPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the ObservabilityPlane CR (credentials, observerURL, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" && !r.shouldIgnoreReconcile(observabilityPlane) {
		r.invalidateCache(ctx, observabilityPlane)
	}

	// Handle create
	// Ignore reconcile if the ObservabilityPlane is already available since this is a one-time create
	// However, we still want to update agent connection status periodically
	if r.shouldIgnoreReconcile(observabilityPlane) {
		if err := r.populateAgentConnectionStatus(ctx, observabilityPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		}

		if err := r.Status().Update(ctx, observabilityPlane); err != nil {
			logger.Error(err, "failed to update ObservabilityPlane status")
		}

		// Requeue to refresh agent connection status
		return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
	}

	// Set the observed generation
	observabilityPlane.Status.ObservedGeneration = observabilityPlane.Generation

	// Update the status condition to indicate the observability plane is created/ready
	meta.SetStatusCondition(
		&observabilityPlane.Status.Conditions,
		NewObservabilityPlaneCreatedCondition(observabilityPlane.Generation),
	)

	// Notify gateway of ObservabilityPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, observabilityPlane, "updated"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ObservabilityPlane reconciliation"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Query agent connection status from gateway and add to observabilityPlane status
	// This must be done BEFORE updating status to avoid conflicts
	if err := r.populateAgentConnectionStatus(ctx, observabilityPlane); err != nil {
		logger.Error(err, "failed to get agent connection status")
		// Don't fail reconciliation for status query errors
	}

	// Update status with both conditions and agent connection status in a single update
	// We use Status().Update() directly instead of UpdateStatusConditions to preserve agentConnection field
	if err := r.Status().Update(ctx, observabilityPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(observabilityPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", observabilityPlane.Name))

	// Requeue to refresh agent connection status
	return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(observabilityPlane *openchoreov1alpha1.ObservabilityPlane) bool {
	return meta.FindStatusCondition(observabilityPlane.Status.Conditions, string(controller.TypeCreated)) != nil
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

func (r *Reconciler) finalize(ctx context.Context, _, observabilityPlane *openchoreov1alpha1.ObservabilityPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("observabilityplane", observabilityPlane.Name)

	if !controllerutil.ContainsFinalizer(observabilityPlane, ObservabilityPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of ObservabilityPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, observabilityPlane, "deleted"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ObservabilityPlane deletion"); shouldRetry {
				return result, retryErr
			}
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

// populateAgentConnectionStatus queries the cluster-gateway for agent connection status
// and populates the ObservabilityPlane status fields (without persisting to API server)
func (r *Reconciler) populateAgentConnectionStatus(ctx context.Context, observabilityPlane *openchoreov1alpha1.ObservabilityPlane) error {
	logger := log.FromContext(ctx).WithValues("observabilityplane", observabilityPlane.Name)

	// Skip if gateway client not configured
	if r.GatewayClient == nil {
		return nil
	}

	effectivePlaneID := observabilityPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = observabilityPlane.Name
	}

	// Query gateway for connection status
	status, err := r.GatewayClient.GetPlaneStatus(ctx, "observabilityplane", effectivePlaneID)
	if err != nil {
		// Log error but don't fail reconciliation
		// If gateway is unreachable, we'll try again on next requeue
		logger.Error(err, "failed to get plane connection status from gateway", "planeID", effectivePlaneID)
		return err
	}

	// Populate ObservabilityPlane status fields (caller will persist to API server)
	now := metav1.Now()
	if observabilityPlane.Status.AgentConnection == nil {
		observabilityPlane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{}
	}

	// Track previous connection state to detect transitions
	previouslyConnected := observabilityPlane.Status.AgentConnection.Connected

	observabilityPlane.Status.AgentConnection.Connected = status.Connected
	observabilityPlane.Status.AgentConnection.ConnectedAgents = status.ConnectedAgents

	if status.Connected {
		observabilityPlane.Status.AgentConnection.LastHeartbeatTime = &metav1.Time{Time: status.LastSeen}

		// Only update LastConnectedTime on transition from disconnected to connected
		if !previouslyConnected {
			observabilityPlane.Status.AgentConnection.LastConnectedTime = &now
		}

		if status.ConnectedAgents == 1 {
			observabilityPlane.Status.AgentConnection.Message = "1 agent connected"
		} else {
			observabilityPlane.Status.AgentConnection.Message = fmt.Sprintf("%d agents connected (HA mode)", status.ConnectedAgents)
		}
	} else {
		// Only update LastDisconnectedTime on transition from connected to disconnected
		if previouslyConnected {
			observabilityPlane.Status.AgentConnection.LastDisconnectedTime = &now
		}
		observabilityPlane.Status.AgentConnection.Message = "No agents connected"
	}

	logger.Info("populated agent connection status",
		"planeID", effectivePlaneID,
		"connected", status.Connected,
		"connectedAgents", status.ConnectedAgents,
	)

	return nil
}

// NewObservabilityPlaneCreatedCondition returns a condition indicating the observability plane is created
func NewObservabilityPlaneCreatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(controller.TypeCreated),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ObservabilityPlaneCreated",
		Message:            "Observabilityplane is created",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("observabilityplane-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ObservabilityPlane{}).
		Named("observabilityplane").
		Complete(r)
}
