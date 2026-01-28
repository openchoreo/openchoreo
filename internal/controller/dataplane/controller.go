// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

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
	"sigs.k8s.io/controller-runtime/pkg/handler"
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

	// Fetch the DataPlane instance
	dataPlane := &openchoreov1alpha1.DataPlane{}
	if err := r.Get(ctx, req.NamespacedName, dataPlane); err != nil {
		if apierrors.IsNotFound(err) {
			// The DataPlane resource may have been deleted since it triggered the reconcile
			logger.Info("DataPlane resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get DataPlane")
		return ctrl.Result{}, err
	}

	// Keep a copy of the old DataPlane object
	old := dataPlane.DeepCopy()

	// Handle the deletion of the dataplane
	if !dataPlane.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing dataplane")
		return r.finalize(ctx, old, dataPlane)
	}

	// Ensure the finalizer is added to the dataplane
	if finalizerAdded, err := r.ensureFinalizer(ctx, dataPlane); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Invalidate cached Kubernetes client on UPDATE
	// This ensures that any changes to the DataPlane CR (kubeconfig, credentials, etc.)
	// trigger a new client to be created with the updated configuration
	if r.ClientMgr != nil && r.CacheVersion != "" && !r.shouldIgnoreReconcile(dataPlane) {
		r.invalidateCache(ctx, dataPlane)
	}

	// Handle create
	// Ignore reconcile if the Dataplane is already available since this is a one-time create
	// However, we still want to update agent connection status periodically
	if r.shouldIgnoreReconcile(dataPlane) {
		// Check if spec has changed (generation > observedGeneration)
		// If so, notify gateway to re-validate agent certificates with updated CA
		specChanged := dataPlane.Status.ObservedGeneration < dataPlane.Generation
		if specChanged && r.GatewayClient != nil {
			logger.Info("detected spec change, notifying gateway for certificate re-validation",
				"generation", dataPlane.Generation,
				"observedGeneration", dataPlane.Status.ObservedGeneration,
			)
			if err := r.notifyGateway(ctx, dataPlane, "updated"); err != nil {
				if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "DataPlane spec update"); shouldRetry {
					return result, retryErr
				}
			}
			// Update observedGeneration to track that we processed this change
			dataPlane.Status.ObservedGeneration = dataPlane.Generation
		}

		if err := r.populateAgentConnectionStatus(ctx, dataPlane); err != nil {
			logger.Error(err, "failed to get agent connection status")
			// Don't fail reconciliation for status query errors
		} else if err := r.Status().Update(ctx, dataPlane); err != nil {
			logger.Error(err, "failed to update DataPlane status")
		}

		// Requeue to refresh agent connection status
		return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
	}

	// Set the observed generation
	dataPlane.Status.ObservedGeneration = dataPlane.Generation

	// Update the status condition to indicate the project is created/ready
	meta.SetStatusCondition(
		&dataPlane.Status.Conditions,
		NewDataPlaneCreatedCondition(dataPlane.Generation),
	)

	// Notify gateway of DataPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, dataPlane, "updated"); err != nil {
			if shouldRetry, result, retryErr := gatewayClient.HandleGatewayError(logger, err, "DataPlane reconciliation"); shouldRetry {
				return result, retryErr
			}
		}
	}

	// Query agent connection status from gateway and add to dataPlane status
	// This must be done BEFORE updating status to avoid conflicts
	if err := r.populateAgentConnectionStatus(ctx, dataPlane); err != nil {
		logger.Error(err, "failed to get agent connection status")
		// Don't fail reconciliation for status query errors
	} else if err := r.Status().Update(ctx, dataPlane); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(dataPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", dataPlane.Name))

	// Requeue to refresh agent connection status
	return ctrl.Result{RequeueAfter: controller.StatusUpdateInterval}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(dataPlane *openchoreov1alpha1.DataPlane) bool {
	return meta.FindStatusCondition(dataPlane.Status.Conditions, string(controller.TypeCreated)) != nil
}

// invalidateCache invalidates the cached Kubernetes client for this DataPlane
func (r *Reconciler) invalidateCache(ctx context.Context, dataPlane *openchoreov1alpha1.DataPlane) {
	logger := log.FromContext(ctx).WithValues("dataplane", dataPlane.Name)

	effectivePlaneID := dataPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = dataPlane.Name
	}

	// Cache key format: v2/dataplane/{planeID}/{namespace}/{name}
	cacheKey := fmt.Sprintf("%s/dataplane/%s/%s/%s", r.CacheVersion, effectivePlaneID, dataPlane.Namespace, dataPlane.Name)
	r.ClientMgr.RemoveClient(cacheKey)

	// Also try invalidating using CR name as planeID (in case planeID was changed FROM default)
	if effectivePlaneID != dataPlane.Name {
		fallbackKey := fmt.Sprintf("%s/dataplane/%s/%s/%s", r.CacheVersion, dataPlane.Name, dataPlane.Namespace, dataPlane.Name)
		r.ClientMgr.RemoveClient(fallbackKey)
	}

	logger.Info("Invalidated cached Kubernetes client for DataPlane",
		"planeID", effectivePlaneID,
		"cacheKey", cacheKey,
	)
}

// notifyGateway sends a lifecycle notification to the cluster-gateway
func (r *Reconciler) notifyGateway(ctx context.Context, dataPlane *openchoreov1alpha1.DataPlane, event string) error {
	logger := log.FromContext(ctx).WithValues("dataplane", dataPlane.Name)

	effectivePlaneID := dataPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = dataPlane.Name
	}

	notification := &gatewayClient.PlaneNotification{
		PlaneType: "dataplane",
		PlaneID:   effectivePlaneID,
		Event:     event,
		Namespace: dataPlane.Namespace,
		Name:      dataPlane.Name,
	}

	logger.Info("notifying gateway of DataPlane lifecycle event",
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
// and populates the DataPlane status fields (without persisting to API server)
func (r *Reconciler) populateAgentConnectionStatus(ctx context.Context, dataPlane *openchoreov1alpha1.DataPlane) error {
	logger := log.FromContext(ctx).WithValues("dataplane", dataPlane.Name)

	// Skip if gateway client not configured
	if r.GatewayClient == nil {
		return nil
	}

	effectivePlaneID := dataPlane.Spec.PlaneID
	if effectivePlaneID == "" {
		effectivePlaneID = dataPlane.Name
	}

	// Query gateway for CR-specific authorization status
	// Pass namespace and name to get authorization status for this specific CR
	status, err := r.GatewayClient.GetPlaneStatus(ctx, "dataplane", effectivePlaneID, dataPlane.Namespace, dataPlane.Name)
	if err != nil {
		// Log error but don't fail reconciliation
		// If gateway is unreachable, we'll try again on next requeue
		logger.Error(err, "failed to get plane connection status from gateway", "planeID", effectivePlaneID)
		return err
	}

	// Populate DataPlane status fields (caller will persist to API server)
	now := metav1.Now()
	if dataPlane.Status.AgentConnection == nil {
		dataPlane.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{}
	}

	// Track previous connection state to detect transitions
	previouslyConnected := dataPlane.Status.AgentConnection.Connected

	dataPlane.Status.AgentConnection.Connected = status.Connected
	dataPlane.Status.AgentConnection.ConnectedAgents = status.ConnectedAgents

	if status.Connected {
		dataPlane.Status.AgentConnection.LastHeartbeatTime = &metav1.Time{Time: status.LastSeen}

		// Only update LastConnectedTime on transition from disconnected to connected
		if !previouslyConnected {
			dataPlane.Status.AgentConnection.LastConnectedTime = &now
		}

		if status.ConnectedAgents == 1 {
			dataPlane.Status.AgentConnection.Message = "1 agent connected"
		} else {
			dataPlane.Status.AgentConnection.Message = fmt.Sprintf("%d agents connected (HA mode)", status.ConnectedAgents)
		}
	} else {
		// Only update LastDisconnectedTime on transition from connected to disconnected
		if previouslyConnected {
			dataPlane.Status.AgentConnection.LastDisconnectedTime = &now
		}
		dataPlane.Status.AgentConnection.Message = "No agents connected"
	}

	logger.Info("populated agent connection status",
		"planeID", effectivePlaneID,
		"connected", status.Connected,
		"connectedAgents", status.ConnectedAgents,
	)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("dataplane-controller")
	}

	// Set up the index for the environment reference
	if err := r.setupDataPlaneRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup dataPlane reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.DataPlane{}).
		Named("dataplane").
		// Watch for Environment changes to reconcile the dataplane
		Watches(
			&openchoreov1alpha1.Environment{},
			handler.EnqueueRequestsFromMapFunc(r.GetDataPlaneForEnvironment),
		).
		Complete(r)
}
