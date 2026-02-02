// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterbuildplane

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
	ClusterBuildPlaneCleanupFinalizer = "openchoreo.dev/clusterbuildplane-cleanup"
)

// Reconciler reconciles a ClusterBuildPlane object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client // Client for notifying cluster-gateway
	CacheVersion  string                // Cache key version prefix (e.g., "v2")
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterbuildplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterbuildplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterbuildplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ClusterBuildPlane instance
	clusterBuildPlane := &openchoreov1alpha1.ClusterBuildPlane{}
	if err := r.Get(ctx, req.NamespacedName, clusterBuildPlane); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ClusterBuildPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ClusterBuildPlane")
		return ctrl.Result{}, err
	}

	// Keep a copy of the old ClusterBuildPlane object
	old := clusterBuildPlane.DeepCopy()

	// Handle the deletion of the buildplane
	if !clusterBuildPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing ClusterBuildPlane", "name", clusterBuildPlane.Name)
		return r.finalize(ctx, old, clusterBuildPlane)
	}

	// Ensure the finalizer is added to the buildplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, clusterBuildPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the ClusterBuildPlane CR (kubeconfig, credentials, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" && !r.shouldIgnoreReconcile(clusterBuildPlane) {
		r.invalidateCache(ctx, clusterBuildPlane)
	}

	// Handle create
	// Ignore reconcile if the ClusterBuildPlane is already available since this is a one-time create
	// However, we still want to update agent connection status periodically
	if r.shouldIgnoreReconcile(clusterBuildPlane) {
		if err := r.populateAgentConnectionStatus(ctx, clusterBuildPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		}

		if err := r.Status().Update(ctx, clusterBuildPlane); err != nil {
			logger.Error(err, "failed to update ClusterBuildPlane status")
		}

		// Requeue to refresh agent connection status
		return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
	}

	// Set the observed generation
	clusterBuildPlane.Status.ObservedGeneration = clusterBuildPlane.Generation

	// Update the status condition to indicate the build plane is created/ready
	meta.SetStatusCondition(
		&clusterBuildPlane.Status.Conditions,
		NewClusterBuildPlaneCreatedCondition(clusterBuildPlane.Generation),
	)

	// Notify gateway of ClusterBuildPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, clusterBuildPlane, "updated"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ClusterBuildPlane reconciliation"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Query agent connection status from gateway and add to clusterBuildPlane status
	// This must be done BEFORE updating status to avoid conflicts
	if err := r.populateAgentConnectionStatus(ctx, clusterBuildPlane); err != nil {
		logger.Error(err, "failed to get agent connection status")
		// Don't fail reconciliation for status query errors
	}

	// Update status with both conditions and agent connection status in a single update
	// We use Status().Update() directly instead of UpdateStatusConditions to preserve agentConnection field
	if err := r.Status().Update(ctx, clusterBuildPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(clusterBuildPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", clusterBuildPlane.Name))

	// Requeue to refresh agent connection status
	return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(clusterBuildPlane *openchoreov1alpha1.ClusterBuildPlane) bool {
	return meta.FindStatusCondition(clusterBuildPlane.Status.Conditions, string(controller.TypeCreated)) != nil
}

// ensureFinalizer ensures that the finalizer is added to the buildplane.
// The first return value indicates whether the finalizer was added to the buildplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, clusterBuildPlane *openchoreov1alpha1.ClusterBuildPlane) (bool, error) {
	if !clusterBuildPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(clusterBuildPlane, ClusterBuildPlaneCleanupFinalizer) {
		return true, r.Update(ctx, clusterBuildPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, _, clusterBuildPlane *openchoreov1alpha1.ClusterBuildPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("ClusterBuildPlane", clusterBuildPlane.Name)

	if !controllerutil.ContainsFinalizer(clusterBuildPlane, ClusterBuildPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of ClusterBuildPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, clusterBuildPlane, "deleted"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ClusterBuildPlane deletion"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Invalidate cached Kubernetes client before removing finalizer
	// This ensures the cache is cleaned up even if the ClusterBuildPlane CR is deleted
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, clusterBuildPlane)
	}

	if controllerutil.RemoveFinalizer(clusterBuildPlane, ClusterBuildPlaneCleanupFinalizer) {
		if err := r.Update(ctx, clusterBuildPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized buildplane")
	return ctrl.Result{}, nil
}

// invalidateCache invalidates the cached Kubernetes client for this ClusterBuildPlane
func (r *Reconciler) invalidateCache(ctx context.Context, clusterBuildPlane *openchoreov1alpha1.ClusterBuildPlane) {
	logger := log.FromContext(ctx).WithValues("ClusterBuildPlane", clusterBuildPlane.Name)

	// Cache key format: v2/clusterbuildplane/{planeID}/{name}
	cacheKey := fmt.Sprintf("%s/clusterbuildplane/%s/%s", r.CacheVersion, clusterBuildPlane.Spec.PlaneID, clusterBuildPlane.Name)
	r.ClientMgr.RemoveClient(cacheKey)

	logger.Info("Invalidated cached Kubernetes client for ClusterBuildPlane",
		"planeID", clusterBuildPlane.Spec.PlaneID,
		"cacheKey", cacheKey,
	)
}

// notifyGateway notifies the cluster gateway about ClusterBuildPlane lifecycle events
func (r *Reconciler) notifyGateway(ctx context.Context, clusterBuildPlane *openchoreov1alpha1.ClusterBuildPlane, event string) error {
	logger := log.FromContext(ctx).WithValues("ClusterBuildPlane", clusterBuildPlane.Name)

	notification := &gatewayClient.PlaneNotification{
		PlaneType: "buildplane", // TODO: change to clusterbuildplane once the gateway is updated
		PlaneID:   clusterBuildPlane.Spec.PlaneID,
		Event:     event,
		Name:      clusterBuildPlane.Name,
		// Namespace is intentionally empty for cluster-scoped resources
	}

	logger.Info("notifying gateway of ClusterBuildPlane lifecycle event",
		"event", event,
		"planeID", clusterBuildPlane.Spec.PlaneID,
	)

	resp, err := r.GatewayClient.NotifyPlaneLifecycle(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to notify gateway: %w", err)
	}

	logger.Info("gateway notification successful",
		"event", event,
		"planeID", clusterBuildPlane.Spec.PlaneID,
		"disconnectedAgents", resp.DisconnectedAgents,
	)

	return nil
}

// populateAgentConnectionStatus queries the cluster-gateway for agent connection status
// and populates the ClusterBuildPlane status fields (without persisting to API server)
func (r *Reconciler) populateAgentConnectionStatus(ctx context.Context, clusterBuildPlane *openchoreov1alpha1.ClusterBuildPlane) error {
	logger := log.FromContext(ctx).WithValues("ClusterBuildPlane", clusterBuildPlane.Name)

	// Skip if gateway client not configured
	if r.GatewayClient == nil {
		return nil
	}

	// Query gateway for connection status using the required planeID
	// For cluster-scoped resources, namespace is empty
	status, err := r.GatewayClient.GetPlaneStatus(ctx, "buildplane", clusterBuildPlane.Spec.PlaneID, "", clusterBuildPlane.Name)
	if err != nil {
		// Log error but don't fail reconciliation
		// If gateway is unreachable, we'll try again on next requeue
		logger.Error(err, "failed to get plane connection status from gateway", "planeID", clusterBuildPlane.Spec.PlaneID, "name", clusterBuildPlane.Name)
		return err
	}

	// Populate ClusterBuildPlane status fields (caller will persist to API server)
	now := metav1.Now()
	if clusterBuildPlane.Status.AgentConnection == nil {
		clusterBuildPlane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{}
	}

	// Track previous connection state to detect transitions
	previouslyConnected := clusterBuildPlane.Status.AgentConnection.Connected

	clusterBuildPlane.Status.AgentConnection.Connected = status.Connected
	clusterBuildPlane.Status.AgentConnection.ConnectedAgents = status.ConnectedAgents

	if status.Connected {
		clusterBuildPlane.Status.AgentConnection.LastHeartbeatTime = &metav1.Time{Time: status.LastSeen}

		// Only update LastConnectedTime on transition from disconnected to connected
		if !previouslyConnected {
			clusterBuildPlane.Status.AgentConnection.LastConnectedTime = &now
		}

		if status.ConnectedAgents == 1 {
			clusterBuildPlane.Status.AgentConnection.Message = "1 agent connected"
		} else {
			clusterBuildPlane.Status.AgentConnection.Message = fmt.Sprintf("%d agents connected (HA mode)", status.ConnectedAgents)
		}
	} else {
		// Only update LastDisconnectedTime on transition from connected to disconnected
		if previouslyConnected {
			clusterBuildPlane.Status.AgentConnection.LastDisconnectedTime = &now
		}
		clusterBuildPlane.Status.AgentConnection.Message = "No agents connected"
	}

	logger.Info("populated agent connection status",
		"planeID", clusterBuildPlane.Spec.PlaneID,
		"connected", status.Connected,
		"connectedAgents", status.ConnectedAgents,
	)

	return nil
}

// NewClusterBuildPlaneCreatedCondition returns a condition indicating the build plane is created
func NewClusterBuildPlaneCreatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(controller.TypeCreated),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ClusterBuildPlaneCreated",
		Message:            "Buildplane is created",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("buildplane-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ClusterBuildPlane{}).
		Named("clusterbuildplane").
		Complete(r)
}
