// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

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
	// ClusterObservabilityPlaneCleanupFinalizer is the finalizer that is used to clean up clusterobservabilityplane resources.
	ClusterObservabilityPlaneCleanupFinalizer = "openchoreo.dev/clusterobservabilityplane-cleanup"
)

// Reconciler reconciles a ClusterObservabilityPlane object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client // Client for notifying cluster-gateway
	CacheVersion  string                // Cache key version prefix (e.g., "v2")
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterobservabilityplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterobservabilityplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterobservabilityplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ClusterObservabilityPlane instance
	clusterObservabilityPlane := &openchoreov1alpha1.ClusterObservabilityPlane{}
	if err := r.Get(ctx, req.NamespacedName, clusterObservabilityPlane); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ClusterObservabilityPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ClusterObservabilityPlane")
		return ctrl.Result{}, err
	}

	// Keep a copy of the old ClusterObservabilityPlane object
	old := clusterObservabilityPlane.DeepCopy()

	// Handle the deletion of the clusterobservabilityplane
	if !clusterObservabilityPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing clusterobservabilityplane", "name", clusterObservabilityPlane.Name)
		return r.finalize(ctx, old, clusterObservabilityPlane)
	}

	// Ensure the finalizer is added to the clusterobservabilityplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, clusterObservabilityPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the ClusterObservabilityPlane CR (credentials, observerURL, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" && !r.shouldIgnoreReconcile(clusterObservabilityPlane) {
		r.invalidateCache(ctx, clusterObservabilityPlane)
	}

	// Handle create
	// Ignore reconcile if the ClusterObservabilityPlane is already available since this is a one-time create
	// However, we still want to update agent connection status periodically
	if r.shouldIgnoreReconcile(clusterObservabilityPlane) {
		if err := r.populateAgentConnectionStatus(ctx, clusterObservabilityPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		}

		if err := r.Status().Update(ctx, clusterObservabilityPlane); err != nil {
			logger.Error(err, "failed to update ClusterObservabilityPlane status")
		}

		// Requeue to refresh agent connection status
		return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
	}

	// Set the observed generation
	clusterObservabilityPlane.Status.ObservedGeneration = clusterObservabilityPlane.Generation

	// Update the status condition to indicate the observability plane is created/ready
	meta.SetStatusCondition(
		&clusterObservabilityPlane.Status.Conditions,
		NewClusterObservabilityPlaneCreatedCondition(clusterObservabilityPlane.Generation),
	)

	// Notify gateway of ClusterObservabilityPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, clusterObservabilityPlane, "updated"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ClusterObservabilityPlane reconciliation"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Query agent connection status from gateway and add to clusterObservabilityPlane status
	// This must be done BEFORE updating status to avoid conflicts
	if err := r.populateAgentConnectionStatus(ctx, clusterObservabilityPlane); err != nil {
		logger.Error(err, "failed to get agent connection status")
		// Don't fail reconciliation for status query errors
	}

	// Update status with both conditions and agent connection status in a single update
	// We use Status().Update() directly instead of UpdateStatusConditions to preserve agentConnection field
	if err := r.Status().Update(ctx, clusterObservabilityPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(clusterObservabilityPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", clusterObservabilityPlane.Name))

	// Requeue to refresh agent connection status
	return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(clusterObservabilityPlane *openchoreov1alpha1.ClusterObservabilityPlane) bool {
	return meta.FindStatusCondition(clusterObservabilityPlane.Status.Conditions, string(controller.TypeCreated)) != nil
}

// ensureFinalizer ensures that the finalizer is added to the clusterobservabilityplane.
// The first return value indicates whether the finalizer was added to the clusterobservabilityplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, clusterObservabilityPlane *openchoreov1alpha1.ClusterObservabilityPlane) (bool, error) {
	if !clusterObservabilityPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(clusterObservabilityPlane, ClusterObservabilityPlaneCleanupFinalizer) {
		return true, r.Update(ctx, clusterObservabilityPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, _, clusterObservabilityPlane *openchoreov1alpha1.ClusterObservabilityPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("clusterobservabilityplane", clusterObservabilityPlane.Name)

	if !controllerutil.ContainsFinalizer(clusterObservabilityPlane, ClusterObservabilityPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of ClusterObservabilityPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, clusterObservabilityPlane, "deleted"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ClusterObservabilityPlane deletion"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Invalidate cached Kubernetes client before removing finalizer
	// This ensures the cache is cleaned up even if the ClusterObservabilityPlane CR is deleted
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, clusterObservabilityPlane)
	}

	if controllerutil.RemoveFinalizer(clusterObservabilityPlane, ClusterObservabilityPlaneCleanupFinalizer) {
		if err := r.Update(ctx, clusterObservabilityPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized clusterobservabilityplane")
	return ctrl.Result{}, nil
}

// invalidateCache invalidates the cached Kubernetes client for this ClusterObservabilityPlane
func (r *Reconciler) invalidateCache(ctx context.Context, clusterObservabilityPlane *openchoreov1alpha1.ClusterObservabilityPlane) {
	logger := log.FromContext(ctx).WithValues("clusterobservabilityplane", clusterObservabilityPlane.Name)

	// Cache key format: v2/clusterobservabilityplane/{name}
	// No namespace or planeID for cluster-scoped resources
	cacheKey := fmt.Sprintf("%s/clusterobservabilityplane/%s", r.CacheVersion, clusterObservabilityPlane.Name)
	r.ClientMgr.RemoveClient(cacheKey)

	logger.Info("Invalidated cached Kubernetes client for ClusterObservabilityPlane",
		"cacheKey", cacheKey,
	)
}

// notifyGateway notifies the cluster gateway about ClusterObservabilityPlane lifecycle events
func (r *Reconciler) notifyGateway(ctx context.Context, clusterObservabilityPlane *openchoreov1alpha1.ClusterObservabilityPlane, event string) error {
	logger := log.FromContext(ctx).WithValues("clusterobservabilityplane", clusterObservabilityPlane.Name)

	notification := &gatewayClient.PlaneNotification{
		PlaneType: "observabilityplane", // TODO: change to clusterobservabilityplane once the gateway is updated
		PlaneID:   clusterObservabilityPlane.Spec.PlaneID,
		Event:     event,
		Name:      clusterObservabilityPlane.Name,
		// Namespace is intentionally empty for cluster-scoped resources
	}

	logger.Info("notifying gateway of ClusterObservabilityPlane lifecycle event",
		"event", event,
		"planeID", clusterObservabilityPlane.Spec.PlaneID,
	)

	resp, err := r.GatewayClient.NotifyPlaneLifecycle(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to notify gateway: %w", err)
	}

	logger.Info("gateway notification successful",
		"event", event,
		"planeID", clusterObservabilityPlane.Spec.PlaneID,
		"disconnectedAgents", resp.DisconnectedAgents,
	)

	return nil
}

// populateAgentConnectionStatus queries the cluster-gateway for agent connection status
// and populates the ClusterObservabilityPlane status fields (without persisting to API server)
func (r *Reconciler) populateAgentConnectionStatus(ctx context.Context, clusterObservabilityPlane *openchoreov1alpha1.ClusterObservabilityPlane) error {
	logger := log.FromContext(ctx).WithValues("clusterobservabilityplane", clusterObservabilityPlane.Name)

	// Skip if gateway client not configured
	if r.GatewayClient == nil {
		return nil
	}

	// Query gateway for connection status using the required planeID
	// For cluster-scoped resources, namespace is empty
	status, err := r.GatewayClient.GetPlaneStatus(ctx, "observabilityplane", clusterObservabilityPlane.Spec.PlaneID, "", clusterObservabilityPlane.Name)
	if err != nil {
		// Log error but don't fail reconciliation
		// If gateway is unreachable, we'll try again on next requeue
		logger.Error(err, "failed to get plane connection status from gateway", "planeID", clusterObservabilityPlane.Spec.PlaneID, "name", clusterObservabilityPlane.Name)
		return err
	}

	// Populate ClusterObservabilityPlane status fields (caller will persist to API server)
	now := metav1.Now()
	if clusterObservabilityPlane.Status.AgentConnection == nil {
		clusterObservabilityPlane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{}
	}

	// Track previous connection state to detect transitions
	previouslyConnected := clusterObservabilityPlane.Status.AgentConnection.Connected

	clusterObservabilityPlane.Status.AgentConnection.Connected = status.Connected
	clusterObservabilityPlane.Status.AgentConnection.ConnectedAgents = status.ConnectedAgents

	if status.Connected {
		clusterObservabilityPlane.Status.AgentConnection.LastHeartbeatTime = &metav1.Time{Time: status.LastSeen}

		// Only update LastConnectedTime on transition from disconnected to connected
		if !previouslyConnected {
			clusterObservabilityPlane.Status.AgentConnection.LastConnectedTime = &now
		}

		if status.ConnectedAgents == 1 {
			clusterObservabilityPlane.Status.AgentConnection.Message = "1 agent connected"
		} else {
			clusterObservabilityPlane.Status.AgentConnection.Message = fmt.Sprintf("%d agents connected (HA mode)", status.ConnectedAgents)
		}
	} else {
		// Only update LastDisconnectedTime on transition from connected to disconnected
		if previouslyConnected {
			clusterObservabilityPlane.Status.AgentConnection.LastDisconnectedTime = &now
		}
		clusterObservabilityPlane.Status.AgentConnection.Message = "No agents connected"
	}

	logger.Info("populated agent connection status",
		"planeID", clusterObservabilityPlane.Spec.PlaneID,
		"connected", status.Connected,
		"connectedAgents", status.ConnectedAgents,
	)

	return nil
}

// NewClusterObservabilityPlaneCreatedCondition returns a condition indicating the observability plane is created
func NewClusterObservabilityPlaneCreatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(controller.TypeCreated),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ClusterObservabilityPlaneCreated",
		Message:            "Observabilityplane is created",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("clusterobservabilityplane-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ClusterObservabilityPlane{}).
		Named("clusterobservabilityplane").
		Complete(r)
}
