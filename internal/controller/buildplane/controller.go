// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

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
	BuildPlaneCleanupFinalizer = "openchoreo.dev/buildplane-cleanup"
)

// Reconciler reconciles a BuildPlane object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client // Client for notifying cluster-gateway
	CacheVersion  string                // Cache key version prefix (e.g., "v2")
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=buildplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=buildplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=buildplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the BuildPlane instance
	buildPlane := &openchoreov1alpha1.BuildPlane{}
	if err := r.Get(ctx, req.NamespacedName, buildPlane); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("BuildPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get BuildPlane")
		return ctrl.Result{}, err
	}

	// Keep a copy of the old BuildPlane object
	old := buildPlane.DeepCopy()

	// Handle the deletion of the buildplane
	if !buildPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing buildplane")
		return r.finalize(ctx, old, buildPlane)
	}

	// Ensure the finalizer is added to the buildplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, buildPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the BuildPlane CR (kubeconfig, credentials, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" && !r.shouldIgnoreReconcile(buildPlane) {
		r.invalidateCache(ctx, buildPlane)
	}

	// Handle create
	// Ignore reconcile if the BuildPlane is already available since this is a one-time create
	// However, we still want to update agent connection status periodically
	if r.shouldIgnoreReconcile(buildPlane) {
		if err := r.populateAgentConnectionStatus(ctx, buildPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		}

		if err := r.Status().Update(ctx, buildPlane); err != nil {
			logger.Error(err, "failed to update BuildPlane status")
		}

		// Requeue to refresh agent connection status
		return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
	}

	// Set the observed generation
	buildPlane.Status.ObservedGeneration = buildPlane.Generation

	// Update the status condition to indicate the build plane is created/ready
	meta.SetStatusCondition(
		&buildPlane.Status.Conditions,
		NewBuildPlaneCreatedCondition(buildPlane.Generation),
	)

	// Notify gateway of BuildPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, buildPlane, "updated"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "BuildPlane reconciliation"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Query agent connection status from gateway and add to buildPlane status
	// This must be done BEFORE updating status to avoid conflicts
	if err := r.populateAgentConnectionStatus(ctx, buildPlane); err != nil {
		logger.Error(err, "failed to get agent connection status")
		// Don't fail reconciliation for status query errors
	}

	// Update status with both conditions and agent connection status in a single update
	// We use Status().Update() directly instead of UpdateStatusConditions to preserve agentConnection field
	if err := r.Status().Update(ctx, buildPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(buildPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", buildPlane.Name))

	// Requeue to refresh agent connection status
	return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(buildPlane *openchoreov1alpha1.BuildPlane) bool {
	return meta.FindStatusCondition(buildPlane.Status.Conditions, string(controller.TypeCreated)) != nil
}

// ensureFinalizer ensures that the finalizer is added to the buildplane.
// The first return value indicates whether the finalizer was added to the buildplane.
func (r *Reconciler) ensureFinalizer(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane) (bool, error) {
	if !buildPlane.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(buildPlane, BuildPlaneCleanupFinalizer) {
		return true, r.Update(ctx, buildPlane)
	}

	return false, nil
}

func (r *Reconciler) finalize(ctx context.Context, _, buildPlane *openchoreov1alpha1.BuildPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("buildplane", buildPlane.Name)

	if !controllerutil.ContainsFinalizer(buildPlane, BuildPlaneCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Notify gateway of BuildPlane deletion before removing finalizer
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, buildPlane, "deleted"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "BuildPlane deletion"); shouldRetry {
				return result, retryErr
			}
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
func (r *Reconciler) invalidateCache(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane) {
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
func (r *Reconciler) notifyGateway(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane, event string) error {
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

// populateAgentConnectionStatus queries the cluster-gateway for agent connection status
// and populates the BuildPlane status fields (without persisting to API server)
func (r *Reconciler) populateAgentConnectionStatus(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane) error {
	logger := log.FromContext(ctx).WithValues("buildplane", buildPlane.Name)

	// Skip if gateway client not configured
	if r.GatewayClient == nil {
		return nil
	}

	effectivePlaneID := buildPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = buildPlane.Name
	}

	// Query gateway for connection status
	status, err := r.GatewayClient.GetPlaneStatus(ctx, "buildplane", effectivePlaneID)
	if err != nil {
		// Log error but don't fail reconciliation
		// If gateway is unreachable, we'll try again on next requeue
		logger.Error(err, "failed to get plane connection status from gateway", "planeID", effectivePlaneID)
		return err
	}

	// Populate BuildPlane status fields (caller will persist to API server)
	now := metav1.Now()
	if buildPlane.Status.AgentConnection == nil {
		buildPlane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{}
	}

	// Track previous connection state to detect transitions
	previouslyConnected := buildPlane.Status.AgentConnection.Connected

	buildPlane.Status.AgentConnection.Connected = status.Connected
	buildPlane.Status.AgentConnection.ConnectedAgents = status.ConnectedAgents

	if status.Connected {
		buildPlane.Status.AgentConnection.LastHeartbeatTime = &metav1.Time{Time: status.LastSeen}

		// Only update LastConnectedTime on transition from disconnected to connected
		if !previouslyConnected {
			buildPlane.Status.AgentConnection.LastConnectedTime = &now
		}

		if status.ConnectedAgents == 1 {
			buildPlane.Status.AgentConnection.Message = "1 agent connected"
		} else {
			buildPlane.Status.AgentConnection.Message = fmt.Sprintf("%d agents connected (HA mode)", status.ConnectedAgents)
		}
	} else {
		// Only update LastDisconnectedTime on transition from connected to disconnected
		if previouslyConnected {
			buildPlane.Status.AgentConnection.LastDisconnectedTime = &now
		}
		buildPlane.Status.AgentConnection.Message = "No agents connected"
	}

	logger.Info("populated agent connection status",
		"planeID", effectivePlaneID,
		"connected", status.Connected,
		"connectedAgents", status.ConnectedAgents,
	)

	return nil
}

// NewBuildPlaneCreatedCondition returns a condition indicating the build plane is created
func NewBuildPlaneCreatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(controller.TypeCreated),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "BuildPlaneCreated",
		Message:            "Buildplane is created",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("buildplane-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.BuildPlane{}).
		Named("buildplane").
		Complete(r)
}
