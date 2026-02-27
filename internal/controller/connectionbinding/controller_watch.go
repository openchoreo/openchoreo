// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package connectionbinding

import (
	"context"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// connectionTargetsIndex indexes ConnectionBindings by their connection target
// (namespace/project/component/environment) for efficient reverse lookup when a
// dependency ReleaseBinding changes.
const connectionTargetsIndex = "spec.connections.targets"

// makeConnectionTargetKey creates an index key for a connection target.
func makeConnectionTargetKey(namespace, project, component, environment string) string {
	return namespace + "/" + project + "/" + component + "/" + environment
}

// setupConnectionTargetsIndex registers a field index that extracts unique
// namespace/project/component/environment keys from each ConnectionBinding's connections list.
func (r *Reconciler) setupConnectionTargetsIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ConnectionBinding{},
		connectionTargetsIndex, func(obj client.Object) []string {
			cb := obj.(*openchoreov1alpha1.ConnectionBinding)
			seen := make(map[string]struct{})
			var keys []string
			for _, conn := range cb.Spec.Connections {
				key := makeConnectionTargetKey(conn.Namespace, conn.Project, conn.Component, cb.Spec.Environment)
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					keys = append(keys, key)
				}
			}
			return keys
		})
}

// endpointStatusChangedPredicate returns a predicate that only passes when
// status.endpoints changes on a ReleaseBinding, or when a ReleaseBinding's
// spec.state changes (Active/Undeploy). This avoids unnecessary reconciliations.
func endpointStatusChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldRB, ok := e.ObjectOld.(*openchoreov1alpha1.ReleaseBinding)
			if !ok {
				return false
			}
			newRB, ok := e.ObjectNew.(*openchoreov1alpha1.ReleaseBinding)
			if !ok {
				return false
			}
			if oldRB.Spec.State != newRB.Spec.State {
				return true
			}
			return !apiequality.Semantic.DeepEqual(oldRB.Status.Endpoints, newRB.Status.Endpoints)
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

// findConnectionBindingsForReleaseBinding returns reconcile requests for all ConnectionBindings
// that depend on the changed ReleaseBinding. It uses the connectionTargetsIndex (which includes
// environment) to find matching ConnectionBindings efficiently without post-filtering.
func (r *Reconciler) findConnectionBindingsForReleaseBinding(ctx context.Context, obj client.Object) []reconcile.Request {
	rb, ok := obj.(*openchoreov1alpha1.ReleaseBinding)
	if !ok {
		return nil
	}

	targetKey := makeConnectionTargetKey(rb.Namespace, rb.Spec.Owner.ProjectName, rb.Spec.Owner.ComponentName, rb.Spec.Environment)

	var connectionBindings openchoreov1alpha1.ConnectionBindingList
	if err := r.List(ctx, &connectionBindings,
		client.MatchingFields{connectionTargetsIndex: targetKey}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list ConnectionBindings for ReleaseBinding", "releaseBinding", rb.Name)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(connectionBindings.Items))
	for _, cb := range connectionBindings.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      cb.Name,
				Namespace: cb.Namespace,
			},
		})
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.setupConnectionTargetsIndex(context.Background(), mgr); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ConnectionBinding{}).
		Watches(&openchoreov1alpha1.ReleaseBinding{},
			handler.EnqueueRequestsFromMapFunc(r.findConnectionBindingsForReleaseBinding),
			builder.WithPredicates(endpointStatusChangedPredicate()),
		).
		Named("connectionbinding").
		Complete(r)
}
