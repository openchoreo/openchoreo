// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

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
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a DataPlane object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientMgr     *kubernetesClient.KubeMultiClientManager
	GatewayClient *gatewayClient.Client // Client for notifying cluster-gateway
	CacheVersion  string                // Cache key version prefix (e.g., "v2")
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DataPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ClusterDataPlane instance
	clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{}
	if err := r.Get(ctx, req.NamespacedName, clusterDataPlane); err != nil {
		if apierrors.IsNotFound(err) {
			// The ClusterDataPlane resource may have been deleted since it triggered the reconcile
			logger.Info("ClusterDataPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get ClusterDataPlane")
		return ctrl.Result{}, err
	}

	// Handle the deletion of the clusterdataplane
	if !clusterDataPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing clusterdataplane", "name", clusterDataPlane.Name)
		return r.finalize(ctx, clusterDataPlane)
	}

	// Ensure the finalizer is added to the clusterdataplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, clusterDataPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the ClusterDataPlane CR (kubeconfig, credentials, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" && !r.shouldIgnoreReconcile(clusterDataPlane) {
		r.invalidateCache(ctx, clusterDataPlane)
	}

	// Handle create
	// Ignore reconcile if the ClusterDataplane is already available since this is a one-time create
	// However, we still want to update agent connection status periodically
	if r.shouldIgnoreReconcile(clusterDataPlane) {
		if err := r.populateAgentConnectionStatus(ctx, clusterDataPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		} else if err := r.Status().Update(ctx, clusterDataPlane); err != nil {
			logger.Error(err, "failed to update ClusterDataPlane status")
		}

		// Requeue to refresh agent connection status
		return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
	}

	// Set the observed generation
	clusterDataPlane.Status.ObservedGeneration = clusterDataPlane.Generation

	// Update the status condition to indicate the project is created/ready
	meta.SetStatusCondition(
		&clusterDataPlane.Status.Conditions,
		NewClusterDataPlaneCreatedCondition(clusterDataPlane.Generation),
	)

	// Notify gateway of ClusterDataPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, clusterDataPlane, "updated"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "ClusterDataPlane reconciliation"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Query agent connection status from gateway and add to clusterDataPlane status
	// This must be done BEFORE updating status to avoid conflicts
	if err := r.populateAgentConnectionStatus(ctx, clusterDataPlane); err != nil {
		logger.Error(err, "failed to get agent connection status")
		// Don't fail reconciliation for status query errors
	} else if err := r.Status().Update(ctx, clusterDataPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(clusterDataPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", clusterDataPlane.Name))

	// Requeue to refresh agent connection status
	return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(clusterDataPlane *openchoreov1alpha1.ClusterDataPlane) bool {
	return meta.FindStatusCondition(clusterDataPlane.Status.Conditions, string(controller.TypeCreated)) != nil
}

// invalidateCache invalidates the cached Kubernetes client for this ClusterDataPlane
func (r *Reconciler) invalidateCache(ctx context.Context, clusterDataPlane *openchoreov1alpha1.ClusterDataPlane) {
	logger := log.FromContext(ctx).WithValues("clusterdataplane", clusterDataPlane.Name)

	// Cache key format: v2/clusterdataplane/{planeID}/{name}
	cacheKey := fmt.Sprintf("%s/clusterdataplane/%s/%s", r.CacheVersion, clusterDataPlane.Spec.PlaneID, clusterDataPlane.Name)
	r.ClientMgr.RemoveClient(cacheKey)

	logger.Info("Invalidated cached Kubernetes client for ClusterDataPlane",
		"planeID", clusterDataPlane.Spec.PlaneID,
		"cacheKey", cacheKey,
	)
}

// notifyGateway sends a lifecycle notification to the cluster-gateway
func (r *Reconciler) notifyGateway(ctx context.Context, clusterDataPlane *openchoreov1alpha1.ClusterDataPlane, event string) error {
	logger := log.FromContext(ctx).WithValues("clusterdataplane", clusterDataPlane.Name)

	notification := &gatewayClient.PlaneNotification{
		PlaneType: "dataplane", // TODO: change to clusterdataplane once the gateway is updated
		PlaneID:   clusterDataPlane.Spec.PlaneID,
		Event:     event,
		Name:      clusterDataPlane.Name,
		// Namespace is intentionally empty for cluster-scoped resources
	}

	logger.Info("notifying gateway of ClusterDataPlane lifecycle event",
		"event", event,
		"planeID", clusterDataPlane.Spec.PlaneID,
	)

	resp, err := r.GatewayClient.NotifyPlaneLifecycle(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to notify gateway: %w", err)
	}

	logger.Info("gateway notification successful",
		"event", event,
		"planeID", clusterDataPlane.Spec.PlaneID,
		"disconnectedAgents", resp.DisconnectedAgents,
	)

	return nil
}

// populateAgentConnectionStatus queries the cluster-gateway for agent connection status
// and populates the ClusterDataPlane status fields (without persisting to API server)
func (r *Reconciler) populateAgentConnectionStatus(ctx context.Context, clusterDataPlane *openchoreov1alpha1.ClusterDataPlane) error {
	logger := log.FromContext(ctx).WithValues("clusterdataplane", clusterDataPlane.Name)

	// Skip if gateway client not configured
	if r.GatewayClient == nil {
		return nil
	}

	// Query gateway for connection status using the required planeID
	// For cluster-scoped resources, namespace is empty
	status, err := r.GatewayClient.GetPlaneStatus(ctx, "dataplane", clusterDataPlane.Spec.PlaneID, "", clusterDataPlane.Name)
	if err != nil {
		// Log error but don't fail reconciliation
		// If gateway is unreachable, we'll try again on next requeue
		logger.Error(err, "failed to get plane connection status from gateway", "planeID", clusterDataPlane.Spec.PlaneID, "name", clusterDataPlane.Name)
		return err
	}

	// Populate ClusterDataPlane status fields (caller will persist to API server)
	now := metav1.Now()
	if clusterDataPlane.Status.AgentConnection == nil {
		clusterDataPlane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{}
	}

	// Track previous connection state to detect transitions
	previouslyConnected := clusterDataPlane.Status.AgentConnection.Connected

	clusterDataPlane.Status.AgentConnection.Connected = status.Connected
	clusterDataPlane.Status.AgentConnection.ConnectedAgents = status.ConnectedAgents

	if status.Connected {
		clusterDataPlane.Status.AgentConnection.LastHeartbeatTime = &metav1.Time{Time: status.LastSeen}

		// Only update LastConnectedTime on transition from disconnected to connected
		if !previouslyConnected {
			clusterDataPlane.Status.AgentConnection.LastConnectedTime = &now
		}

		if status.ConnectedAgents == 1 {
			clusterDataPlane.Status.AgentConnection.Message = "1 agent connected"
		} else {
			clusterDataPlane.Status.AgentConnection.Message = fmt.Sprintf("%d agents connected (HA mode)", status.ConnectedAgents)
		}
	} else {
		// Only update LastDisconnectedTime on transition from connected to disconnected
		if previouslyConnected {
			clusterDataPlane.Status.AgentConnection.LastDisconnectedTime = &now
		}
		clusterDataPlane.Status.AgentConnection.Message = "No agents connected"
	}

	logger.Info("populated agent connection status",
		"planeID", clusterDataPlane.Spec.PlaneID,
		"connected", status.Connected,
		"connectedAgents", status.ConnectedAgents,
	)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("clusterdataplane-controller")
	}

	// Cluster-scoped planes don't watch namespace-scoped resources like Environments
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ClusterDataPlane{}).
		Named("clusterdataplane").
		Complete(r)
}
