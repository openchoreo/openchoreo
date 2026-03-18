// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

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
	ClusterWorkflowPlaneCleanupFinalizer = "openchoreo.dev/clusterworkflowplane-cleanup"
)

// Reconciler reconciles a ClusterWorkflowPlane object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client // Client for notifying cluster-gateway
	CacheVersion  string                // Cache key version prefix (e.g., "v2")
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterworkflowplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterworkflowplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterworkflowplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ClusterWorkflowPlane instance
	clusterWorkflowPlane := &openchoreov1alpha1.ClusterWorkflowPlane{}
	if err := r.Get(ctx, req.NamespacedName, clusterWorkflowPlane); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ClusterWorkflowPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ClusterWorkflowPlane")
		return ctrl.Result{}, err
	}

	// Keep a copy of the old ClusterWorkflowPlane object
	old := clusterWorkflowPlane.DeepCopy()

	// Handle the deletion of the workflowplane
	if !clusterWorkflowPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing ClusterWorkflowPlane", "name", clusterWorkflowPlane.Name)
		return r.finalize(ctx, old, clusterWorkflowPlane)
	}

	// Ensure the finalizer is added to the workflowplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, clusterWorkflowPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the ClusterWorkflowPlane CR (kubeconfig, credentials, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" && !r.shouldIgnoreReconcile(clusterWorkflowPlane) {
		r.invalidateCache(ctx, clusterWorkflowPlane)
	}

	// Handle create
	// Ignore reconcile if the ClusterWorkflowPlane is already available since this is a one-time create
	// However, we still want to update agent connection status periodically
	if r.shouldIgnoreReconcile(clusterWorkflowPlane) {
		if err := r.populateAgentConnectionStatus(ctx, clusterWorkflowPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		}

		if err := r.Status().Update(ctx, clusterWorkflowPlane); err != nil {
			logger.Error(err, "failed to update ClusterWorkflowPlane status")
		}

		// Requeue to refresh agent connection status
		return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
	}

	// Set the observed generation
	clusterWorkflowPlane.Status.ObservedGeneration = clusterWorkflowPlane.Generation

	// Update the status condition to indicate the workflow plane is created/ready
	meta.SetStatusCondition(
		&clusterWorkflowPlane.Status.Conditions,
		NewClusterWorkflowPlaneCreatedCondition(clusterWorkflowPlane.Generation),
	)

	// Notify gateway of ClusterWorkflowPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	gatewayNotified := false
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, clusterWorkflowPlane, "updated"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ClusterWorkflowPlane reconciliation"); shouldRetry {
				return result, retryErr
			}
		} else {
			gatewayNotified = true
		}
	}

	// Skip immediate status poll after gateway notification — agents may be reconnecting
	// after a revalidation/disconnect cycle. The next periodic requeue will capture the
	// settled state, avoiding false "disconnected" flaps with HA agent replicas.
	if !gatewayNotified {
		if err := r.populateAgentConnectionStatus(ctx, clusterWorkflowPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		}
	} else {
		logger.Info("skipping immediate status poll after gateway notification, agents may be reconnecting")
	}

	// Update status with both conditions and agent connection status in a single update
	// We use Status().Update() directly instead of UpdateStatusConditions to preserve agentConnection field
	if err := r.Status().Update(ctx, clusterWorkflowPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(clusterWorkflowPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", clusterWorkflowPlane.Name))

	// Requeue to refresh agent connection status
	return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(clusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane) bool {
	return meta.FindStatusCondition(clusterWorkflowPlane.Status.Conditions, string(controller.TypeCreated)) != nil
}

// ensureFinalizer ensures that the finalizer is added to the workflowplane.
// The first return value indicates whether the finalizer was added to the workflowplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, clusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane) (bool, error) {
	if !clusterWorkflowPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(clusterWorkflowPlane, ClusterWorkflowPlaneCleanupFinalizer) {
		return true, r.Update(ctx, clusterWorkflowPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, _, clusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("ClusterWorkflowPlane", clusterWorkflowPlane.Name)

	if !controllerutil.ContainsFinalizer(clusterWorkflowPlane, ClusterWorkflowPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of ClusterWorkflowPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, clusterWorkflowPlane, "deleted"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ClusterWorkflowPlane deletion"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Invalidate cached Kubernetes client before removing finalizer
	// This ensures the cache is cleaned up even if the ClusterWorkflowPlane CR is deleted
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, clusterWorkflowPlane)
	}

	if controllerutil.RemoveFinalizer(clusterWorkflowPlane, ClusterWorkflowPlaneCleanupFinalizer) {
		if err := r.Update(ctx, clusterWorkflowPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized workflowplane")
	return ctrl.Result{}, nil
}

// invalidateCache invalidates the cached Kubernetes client for this ClusterWorkflowPlane
func (r *Reconciler) invalidateCache(ctx context.Context, clusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane) {
	logger := log.FromContext(ctx).WithValues("ClusterWorkflowPlane", clusterWorkflowPlane.Name)

	// Cache key format: v2/clusterworkflowplane/{planeID}/{name}
	cacheKey := fmt.Sprintf("%s/clusterworkflowplane/%s/%s", r.CacheVersion, clusterWorkflowPlane.Spec.PlaneID, clusterWorkflowPlane.Name)
	r.ClientMgr.RemoveClient(cacheKey)

	logger.Info("Invalidated cached Kubernetes client for ClusterWorkflowPlane",
		"planeID", clusterWorkflowPlane.Spec.PlaneID,
		"cacheKey", cacheKey,
	)
}

// notifyGateway notifies the cluster gateway about ClusterWorkflowPlane lifecycle events
func (r *Reconciler) notifyGateway(ctx context.Context, clusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane, event string) error {
	logger := log.FromContext(ctx).WithValues("ClusterWorkflowPlane", clusterWorkflowPlane.Name)

	notification := &gatewayClient.PlaneNotification{
		PlaneType: "workflowplane", // TODO: change to clusterworkflowplane once the gateway is updated
		PlaneID:   clusterWorkflowPlane.Spec.PlaneID,
		Event:     event,
		Name:      clusterWorkflowPlane.Name,
		// Namespace is intentionally empty for cluster-scoped resources
	}

	logger.Info("notifying gateway of ClusterWorkflowPlane lifecycle event",
		"event", event,
		"planeID", clusterWorkflowPlane.Spec.PlaneID,
	)

	resp, err := r.GatewayClient.NotifyPlaneLifecycle(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to notify gateway: %w", err)
	}

	logger.Info("gateway notification successful",
		"event", event,
		"planeID", clusterWorkflowPlane.Spec.PlaneID,
		"disconnectedAgents", resp.DisconnectedAgents,
	)

	return nil
}

// populateAgentConnectionStatus queries the cluster-gateway for agent connection status
// and populates the ClusterWorkflowPlane status fields (without persisting to API server)
func (r *Reconciler) populateAgentConnectionStatus(ctx context.Context, clusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane) error {
	logger := log.FromContext(ctx).WithValues("ClusterWorkflowPlane", clusterWorkflowPlane.Name)

	// Skip if gateway client not configured
	if r.GatewayClient == nil {
		return nil
	}

	// Query gateway for connection status using the required planeID
	// For cluster-scoped resources, namespace is empty
	status, err := r.GatewayClient.GetPlaneStatus(ctx, "workflowplane", clusterWorkflowPlane.Spec.PlaneID, "", clusterWorkflowPlane.Name)
	if err != nil {
		// Log error but don't fail reconciliation
		// If gateway is unreachable, we'll try again on next requeue
		logger.Error(err, "failed to get plane connection status from gateway", "planeID", clusterWorkflowPlane.Spec.PlaneID, "name", clusterWorkflowPlane.Name)
		return err
	}

	// Populate ClusterWorkflowPlane status fields (caller will persist to API server)
	now := metav1.Now()
	if clusterWorkflowPlane.Status.AgentConnection == nil {
		clusterWorkflowPlane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{}
	}

	// Track previous connection state to detect transitions
	previouslyConnected := clusterWorkflowPlane.Status.AgentConnection.Connected

	clusterWorkflowPlane.Status.AgentConnection.Connected = status.Connected
	clusterWorkflowPlane.Status.AgentConnection.ConnectedAgents = status.ConnectedAgents

	if status.Connected {
		clusterWorkflowPlane.Status.AgentConnection.LastHeartbeatTime = &metav1.Time{Time: status.LastSeen}

		// Only update LastConnectedTime on transition from disconnected to connected
		if !previouslyConnected {
			clusterWorkflowPlane.Status.AgentConnection.LastConnectedTime = &now
		}

		if status.ConnectedAgents == 1 {
			clusterWorkflowPlane.Status.AgentConnection.Message = "1 agent connected"
		} else {
			clusterWorkflowPlane.Status.AgentConnection.Message = fmt.Sprintf("%d agents connected (HA mode)", status.ConnectedAgents)
		}
	} else {
		// Only update LastDisconnectedTime on transition from connected to disconnected
		if previouslyConnected {
			clusterWorkflowPlane.Status.AgentConnection.LastDisconnectedTime = &now
		}
		clusterWorkflowPlane.Status.AgentConnection.Message = "No agents connected"
	}

	logger.Info("populated agent connection status",
		"planeID", clusterWorkflowPlane.Spec.PlaneID,
		"connected", status.Connected,
		"connectedAgents", status.ConnectedAgents,
	)

	return nil
}

// NewClusterWorkflowPlaneCreatedCondition returns a condition indicating the workflow plane is created
func NewClusterWorkflowPlaneCreatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(controller.TypeCreated),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ClusterWorkflowPlaneCreated",
		Message:            "Workflowplane is created",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("workflowplane-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ClusterWorkflowPlane{}).
		Named("clusterworkflowplane").
		Complete(r)
}
