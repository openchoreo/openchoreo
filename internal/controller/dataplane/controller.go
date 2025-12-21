// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	if r.shouldIgnoreReconcile(dataPlane) {
		return ctrl.Result{}, nil
	}

	// Set the observed generation
	dataPlane.Status.ObservedGeneration = dataPlane.Generation

	// Update the status condition to indicate the project is created/ready
	meta.SetStatusCondition(
		&dataPlane.Status.Conditions,
		NewDataPlaneCreatedCondition(dataPlane.Generation),
	)

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, dataPlane); err != nil {
		return ctrl.Result{}, err
	}

	// Notify gateway of DataPlane reconciliation (create/update)
	// Using "updated" since Reconcile handles both CREATE and UPDATE events
	// and the gateway treats both identically (triggers agent reconnection)
	if r.GatewayClient != nil {
		if err := r.notifyGateway(ctx, dataPlane, "updated"); err != nil {
			// Don't fail reconciliation if gateway notification fails.
			// Rationale: Gateway notification is "best effort" - the system remains
			// eventually consistent through agent reconnection and cert verification.
			// Failing reconciliation would prevent CR status updates and requeue
			// indefinitely if gateway is temporarily unavailable.
			logger.Error(err, "Failed to notify gateway of DataPlane reconciliation")
		}
	}

	r.Recorder.Event(dataPlane, corev1.EventTypeNormal, "ReconcileComplete", fmt.Sprintf("Successfully created %s", dataPlane.Name))

	return ctrl.Result{}, nil
}

func (r *Reconciler) shouldIgnoreReconcile(dataPlane *openchoreov1alpha1.DataPlane) bool {
	return meta.FindStatusCondition(dataPlane.Status.Conditions, string(controller.TypeAvailable)) != nil
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
