// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controlplanenamespace

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// CleanupFinalizer blocks control-plane namespace deletion until the
	// shared workflows-* namespace on the workflow plane is removed.
	CleanupFinalizer = "openchoreo.dev/control-plane-namespace-cleanup"

	// workflowPlaneClientTimeout bounds remote Get/Delete during finalization so a
	// stalled plane cannot pin a reconcile worker indefinitely.
	workflowPlaneClientTimeout = 30 * time.Second
)

// Reconciler manages cleanup of workflow-plane resources owned by an
// OpenChoreo control-plane namespace (labeled openchoreo.dev/control-plane=true).
type Reconciler struct {
	client.Client
	PlaneClientProvider kubernetesClient.WorkflowPlaneClientProvider
	Scheme              *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterworkflowplanes,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespace", req.Name)

	ns := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get namespace %s: %w", req.Name, err)
	}

	// Finalize even if the control-plane label was removed after the finalizer
	// was added, so deletion cannot strand forever.
	if !ns.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, ns)
	}

	if ns.Labels[labels.LabelKeyControlPlaneNamespace] != labels.LabelValueTrue {
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(ns, CleanupFinalizer) {
		logger.Info("Adding control-plane namespace cleanup finalizer")
		if err := r.Update(ctx, ns); err != nil {
			return ctrl.Result{}, fmt.Errorf("add cleanup finalizer on namespace %s: %w", ns.Name, err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) finalize(ctx context.Context, ns *corev1.Namespace) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespace", ns.Name)

	if !controllerutil.ContainsFinalizer(ns, CleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	pending, err := r.deleteWorkflowsNamespace(ctx, ns.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pending {
		logger.Info("Waiting for workflows namespace deletion", "workflowsNamespace", workflowsNamespaceName(ns.Name))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if controllerutil.RemoveFinalizer(ns, CleanupFinalizer) {
		if err := r.Update(ctx, ns); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove cleanup finalizer on namespace %s: %w", ns.Name, err)
		}
	}
	return ctrl.Result{}, nil
}

// deleteWorkflowsNamespace deletes workflows-<orgNS> on the resolved workflow plane.
// Returns pending=true while the remote namespace still exists.
func (r *Reconciler) deleteWorkflowsNamespace(ctx context.Context, orgNS string) (bool, error) {
	logger := log.FromContext(ctx)

	wpResult, err := controller.GetWorkflowPlaneFromRef(ctx, r.Client, orgNS, nil)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// No workflow plane configured — nothing to clean up.
			logger.Info("No workflow plane found during namespace finalization, skipping workflows cleanup")
			return false, nil
		}
		return false, fmt.Errorf("resolve workflow plane for namespace %s: %w", orgNS, err)
	}

	wpClient, err := wpResult.GetK8sClient(r.PlaneClientProvider)
	if err != nil {
		return false, fmt.Errorf("workflow plane client for namespace %s: %w", orgNS, err)
	}

	remoteCtx, cancel := context.WithTimeout(ctx, workflowPlaneClientTimeout)
	defer cancel()

	wfNSName := workflowsNamespaceName(orgNS)
	wfNS := &corev1.Namespace{}
	if err := wpClient.Get(remoteCtx, client.ObjectKey{Name: wfNSName}, wfNS); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get workflows namespace %s: %w", wfNSName, err)
	}

	if wfNS.DeletionTimestamp.IsZero() {
		if err := wpClient.Delete(remoteCtx, wfNS); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("delete workflows namespace %s: %w", wfNSName, err)
		}
	}
	return true, nil
}

func workflowsNamespaceName(orgNS string) string {
	return "workflows-" + orgNS
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			if obj.GetLabels()[labels.LabelKeyControlPlaneNamespace] == labels.LabelValueTrue {
				return true
			}
			// Keep reconciling while our finalizer is present even if the label was removed.
			return controllerutil.ContainsFinalizer(obj, CleanupFinalizer)
		})).
		Named("controlplanenamespace").
		Complete(r)
}
