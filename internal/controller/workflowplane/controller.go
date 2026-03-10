// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

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
	WorkflowPlaneCleanupFinalizer = "openchoreo.dev/workflowplane-cleanup"
)

// Reconciler reconciles a WorkflowPlane object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client // Client for notifying cluster-gateway
	CacheVersion  string                // Cache key version prefix (e.g., "v2")
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the WorkflowPlane instance
	workflowPlane := &openchoreov1alpha1.WorkflowPlane{}
	if err := r.Get(ctx, req.NamespacedName, workflowPlane); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("WorkflowPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get WorkflowPlane")
		return ctrl.Result{}, err
	}

	// Keep a copy of the old WorkflowPlane object
	old := workflowPlane.DeepCopy()

	// Handle the deletion of the workflowplane
	if !workflowPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing workflowplane")
		return r.finalize(ctx, old, workflowPlane)
	}

	// Ensure the finalizer is added to the workflowplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, workflowPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the WorkflowPlane CR (kubeconfig, credentials, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" && !r.shouldIgnoreReconcile(workflowPlane) {
		r.invalidateCache(ctx, workflowPlane)
	}

	// Handle create
	// Ignore reconcile if the WorkflowPlane is already available since this is a one-time create
	// However, we still want to update agent connection status periodically
	if r.shouldIgnoreReconcile(workflowPlane) {
		// Check if spec has changed (generation > observedGeneration)
		// If so, notify gateway to re-validate agent certificates with updated CA
		specChanged := workflowPlane.Status.ObservedGeneration < workflowPlane.Generation
		if specChanged && r.GatewayClient != nil {
			logger.Info("detected spec change, notifying gateway for certificate re-validation",
				"generation", workflowPlane.Generation,
				"observedGeneration", workflowPlane.Status.ObservedGeneration,
			)
			if err := r.notifyGateway(ctx, workflowPlane, "updated"); err != nil {
				if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "WorkflowPlane spec update"); shouldRetry {
					return result, retryErr
				}
			}
			// Update observedGeneration to track that we processed this change
			workflowPlane.Status.ObservedGeneration = workflowPlane.Generation
		}

		if err := r.populateAgentConnectionStatus(ctx, workflowPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		}

		if err := r.Status().Update(ctx, workflowPlane); err != nil {
			logger.Error(err, "failed to update WorkflowPlane status")
		}

		// Requeue to refresh agent connection status
		return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
	}

	// Set the observed generation
	workflowPlane.Status.ObservedGeneration = workflowPlane.Generation

	// Update the status condition to indicate the workflow plane is created/ready
	meta.SetStatusCondition(
		&workflowPlane.Status.Conditions,
		NewWorkflowPlaneCreatedCondition(workflowPlane.Generation),
	)

	// Notify gateway of WorkflowPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, workflowPlane, "updated"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "WorkflowPlane reconciliation"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Query agent connection status from gateway and add to workflowPlane status
	// This must be done BEFORE updating status to avoid conflicts
	if err := r.populateAgentConnectionStatus(ctx, workflowPlane); err != nil {
		logger.Error(err, "failed to get agent connection status")
		// Don't fail reconciliation for status query errors
	}

	// Update status with both conditions and agent connection status in a single update
	// We use Status().Update() directly instead of UpdateStatusConditions to preserve agentConnection field
	if err := r.Status().Update(ctx, workflowPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(workflowPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", workflowPlane.Name))

	// Requeue to refresh agent connection status
	return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(workflowPlane *openchoreov1alpha1.WorkflowPlane) bool {
	return meta.FindStatusCondition(workflowPlane.Status.Conditions, string(controller.TypeCreated)) != nil
}

// ensureFinalizer ensures that the finalizer is added to the workflowplane.
// The first return value indicates whether the finalizer was added to the workflowplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, workflowPlane *openchoreov1alpha1.WorkflowPlane) (bool, error) {
	if !workflowPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(workflowPlane, WorkflowPlaneCleanupFinalizer) {
		return true, r.Update(ctx, workflowPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, _, workflowPlane *openchoreov1alpha1.WorkflowPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("workflowplane", workflowPlane.Name)

	if !controllerutil.ContainsFinalizer(workflowPlane, WorkflowPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of WorkflowPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, workflowPlane, "deleted"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "WorkflowPlane deletion"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Invalidate cached Kubernetes client before removing finalizer
	// This ensures the cache is cleaned up even if the WorkflowPlane CR is deleted
	if r.ClientMgr != nil && r.CacheVersion != "" {
		r.invalidateCache(ctx, workflowPlane)
	}

	if controllerutil.RemoveFinalizer(workflowPlane, WorkflowPlaneCleanupFinalizer) {
		if err := r.Update(ctx, workflowPlane); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized workflowplane")
	return ctrl.Result{}, nil
}

// invalidateCache invalidates the cached Kubernetes client for this WorkflowPlane
func (r *Reconciler) invalidateCache(ctx context.Context, workflowPlane *openchoreov1alpha1.WorkflowPlane) {
	logger := log.FromContext(ctx).WithValues("workflowplane", workflowPlane.Name)

	effectivePlaneID := workflowPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = workflowPlane.Name
	}

	// Cache key format: v2/workflowplane/{planeID}/{namespace}/{name}
	cacheKey := fmt.Sprintf("%s/workflowplane/%s/%s/%s", r.CacheVersion, effectivePlaneID, workflowPlane.Namespace, workflowPlane.Name)
	r.ClientMgr.RemoveClient(cacheKey)

	if effectivePlaneID != workflowPlane.Name {
		fallbackKey := fmt.Sprintf("%s/workflowplane/%s/%s/%s", r.CacheVersion, workflowPlane.Name, workflowPlane.Namespace, workflowPlane.Name)
		r.ClientMgr.RemoveClient(fallbackKey)
	}

	logger.Info("Invalidated cached Kubernetes client for WorkflowPlane",
		"planeID", effectivePlaneID,
		"cacheKey", cacheKey,
	)
}

// notifyGateway notifies the cluster gateway about WorkflowPlane lifecycle events
func (r *Reconciler) notifyGateway(ctx context.Context, workflowPlane *openchoreov1alpha1.WorkflowPlane, event string) error {
	logger := log.FromContext(ctx).WithValues("workflowplane", workflowPlane.Name)

	effectivePlaneID := workflowPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = workflowPlane.Name
	}

	notification := &gatewayClient.PlaneNotification{
		PlaneType: "workflowplane",
		PlaneID:   effectivePlaneID,
		Event:     event,
		Namespace: workflowPlane.Namespace,
		Name:      workflowPlane.Name,
	}

	logger.Info("notifying gateway of WorkflowPlane lifecycle event",
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
// and populates the WorkflowPlane status fields (without persisting to API server)
func (r *Reconciler) populateAgentConnectionStatus(ctx context.Context, workflowPlane *openchoreov1alpha1.WorkflowPlane) error {
	logger := log.FromContext(ctx).WithValues("workflowplane", workflowPlane.Name)

	// Skip if gateway client not configured
	if r.GatewayClient == nil {
		return nil
	}

	effectivePlaneID := workflowPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = workflowPlane.Name
	}

	// Query gateway for CR-specific authorization status
	// Pass namespace and name to get authorization status for this specific CR
	status, err := r.GatewayClient.GetPlaneStatus(ctx, "workflowplane", effectivePlaneID, workflowPlane.Namespace, workflowPlane.Name)
	if err != nil {
		// Log error but don't fail reconciliation
		// If gateway is unreachable, we'll try again on next requeue
		logger.Error(err, "failed to get plane connection status from gateway", "planeID", effectivePlaneID)
		return err
	}

	// Populate WorkflowPlane status fields (caller will persist to API server)
	now := metav1.Now()
	if workflowPlane.Status.AgentConnection == nil {
		workflowPlane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{}
	}

	// Track previous connection state to detect transitions
	previouslyConnected := workflowPlane.Status.AgentConnection.Connected

	workflowPlane.Status.AgentConnection.Connected = status.Connected
	workflowPlane.Status.AgentConnection.ConnectedAgents = status.ConnectedAgents

	if status.Connected {
		workflowPlane.Status.AgentConnection.LastHeartbeatTime = &metav1.Time{Time: status.LastSeen}

		// Only update LastConnectedTime on transition from disconnected to connected
		if !previouslyConnected {
			workflowPlane.Status.AgentConnection.LastConnectedTime = &now
		}

		if status.ConnectedAgents == 1 {
			workflowPlane.Status.AgentConnection.Message = "1 agent connected"
		} else {
			workflowPlane.Status.AgentConnection.Message = fmt.Sprintf("%d agents connected (HA mode)", status.ConnectedAgents)
		}
	} else {
		// Only update LastDisconnectedTime on transition from connected to disconnected
		if previouslyConnected {
			workflowPlane.Status.AgentConnection.LastDisconnectedTime = &now
		}
		workflowPlane.Status.AgentConnection.Message = "No agents connected"
	}

	logger.Info("populated agent connection status",
		"planeID", effectivePlaneID,
		"connected", status.Connected,
		"connectedAgents", status.ConnectedAgents,
	)

	return nil
}

// NewWorkflowPlaneCreatedCondition returns a condition indicating the workflow plane is created
func NewWorkflowPlaneCreatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(controller.TypeCreated),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "WorkflowPlaneCreated",
		Message:            "Workflowplane is created",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("workflowplane-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.WorkflowPlane{}).
		Named("workflowplane").
		Complete(r)
}
