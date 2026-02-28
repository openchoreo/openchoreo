// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

const (
	// secretReferencesIndex is the field index name for SecretReference names used in ReleaseBinding.
	// This index reads from status.secretReferenceNames (a pure function of the indexed object).
	secretReferencesIndex = "status.secretReferenceNames"

	// connectionTargetsIndex indexes ReleaseBindings by their connection targets
	// (namespace/project/component/environment) for efficient reverse lookup when a
	// dependency ReleaseBinding's endpoints change.
	connectionTargetsIndex = "status.connectionTargets"
)

// makeConnectionTargetKey creates an index key for a connection target.
func makeConnectionTargetKey(namespace, project, component, environment string) string {
	return namespace + "/" + project + "/" + component + "/" + environment
}

// setupSecretReferencesIndex sets up the field index for SecretReference names used by ReleaseBinding.
// This index reads from status.secretReferenceNames which is populated during reconciliation,
// making it a pure function of the indexed object with no external API calls.
func (r *Reconciler) setupSecretReferencesIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ReleaseBinding{},
		secretReferencesIndex, func(obj client.Object) []string {
			releaseBinding := obj.(*openchoreov1alpha1.ReleaseBinding)
			return releaseBinding.Status.SecretReferenceNames
		})
}

// setupConnectionTargetsIndex registers a field index that extracts unique
// namespace/project/component/environment keys from each ReleaseBinding's status.connectionTargets.
func (r *Reconciler) setupConnectionTargetsIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ReleaseBinding{},
		connectionTargetsIndex, func(obj client.Object) []string {
			rb := obj.(*openchoreov1alpha1.ReleaseBinding)
			if len(rb.Status.ConnectionTargets) == 0 {
				return nil
			}
			seen := make(map[string]struct{})
			var keys []string
			for _, conn := range rb.Status.ConnectionTargets {
				key := makeConnectionTargetKey(conn.Namespace, conn.Project, conn.Component, rb.Spec.Environment)
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					keys = append(keys, key)
				}
			}
			return keys
		})
}

// endpointStatusChangedPredicate returns a predicate that only passes when
// status.endpoints changes on a ReleaseBinding, or when spec.state changes
// (Active/Undeploy). This avoids enqueuing consumers for unrelated status changes
// (e.g., resolvedConnections updates), preventing infinite loops.
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

// findConsumerReleaseBindings returns reconcile requests for all ReleaseBindings
// that depend on the changed ReleaseBinding (i.e., have it as a connection target).
func (r *Reconciler) findConsumerReleaseBindings(ctx context.Context, obj client.Object) []reconcile.Request {
	rb, ok := obj.(*openchoreov1alpha1.ReleaseBinding)
	if !ok {
		return nil
	}

	targetKey := makeConnectionTargetKey(rb.Namespace, rb.Spec.Owner.ProjectName, rb.Spec.Owner.ComponentName, rb.Spec.Environment)

	var consumers openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &consumers,
		client.InNamespace(rb.Namespace),
		client.MatchingFields{connectionTargetsIndex: targetKey}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list consumer ReleaseBindings", "releaseBinding", rb.Name)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(consumers.Items))
	for _, consumer := range consumers.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      consumer.Name,
				Namespace: consumer.Namespace,
			},
		})
	}
	return requests
}

// listReleaseBindingsForSecretReference returns reconcile requests for all ReleaseBindings
// that use the changed SecretReference (via status.secretReferenceNames index).
func (r *Reconciler) listReleaseBindingsForSecretReference(ctx context.Context, obj client.Object) []reconcile.Request {
	secretRef := obj.(*openchoreov1alpha1.SecretReference)
	logger := log.FromContext(ctx)

	logger.Info("SecretReference changed, finding affected ReleaseBindings",
		"secretReference", secretRef.Name,
		"namespace", secretRef.Namespace)

	var releaseBindings openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &releaseBindings,
		client.InNamespace(secretRef.Namespace),
		client.MatchingFields{secretReferencesIndex: secretRef.Name}); err != nil {
		logger.Error(err, "Failed to list ReleaseBindings for SecretReference",
			"secretReference", secretRef.Name)
		return nil
	}

	if len(releaseBindings.Items) == 0 {
		logger.Info("No ReleaseBindings found using this SecretReference",
			"secretReference", secretRef.Name)
		return nil
	}

	logger.Info("Found ReleaseBindings using SecretReference",
		"secretReference", secretRef.Name,
		"count", len(releaseBindings.Items))

	requests := make([]reconcile.Request, len(releaseBindings.Items))
	for i, rb := range releaseBindings.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      rb.Name,
				Namespace: rb.Namespace,
			},
		}
		logger.Info("Enqueuing ReleaseBinding for reconciliation due to SecretReference change",
			"releaseBinding", rb.Name,
			"secretReference", secretRef.Name)
	}

	return requests
}

// findReleaseBindingsForComponent maps a Component to its owned ReleaseBindings.
// Note: Uses the shared index key controller.IndexKeyReleaseBindingOwnerComponentName
// which is registered by the Component controller.
func (r *Reconciler) findReleaseBindingsForComponent(ctx context.Context, obj client.Object) []ctrl.Request {
	component := obj.(*openchoreov1alpha1.Component)

	var bindings openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &bindings,
		client.InNamespace(component.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: component.Name}); err != nil {
		return nil
	}

	requests := make([]ctrl.Request, len(bindings.Items))
	for i, binding := range bindings.Items {
		requests[i] = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      binding.Name,
				Namespace: binding.Namespace,
			},
		}
	}
	return requests
}

// collectSecretReferenceNames extracts SecretReference names from a merged workload.
// This is used to populate status.secretReferenceNames for the field index.
func collectSecretReferenceNames(workload *openchoreov1alpha1.Workload, releaseBinding *openchoreov1alpha1.ReleaseBinding) []string {
	if workload == nil {
		return nil
	}

	mergedWorkload := pipelinecontext.MergeWorkloadOverrides(workload, releaseBinding.Spec.WorkloadOverrides)
	if mergedWorkload == nil {
		return nil
	}

	seen := make(map[string]struct{})
	var names []string
	container := mergedWorkload.Spec.Container
	for _, env := range container.Env {
		if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil && env.ValueFrom.SecretRef.Name != "" {
			if _, dup := seen[env.ValueFrom.SecretRef.Name]; !dup {
				seen[env.ValueFrom.SecretRef.Name] = struct{}{}
				names = append(names, env.ValueFrom.SecretRef.Name)
			}
		}
	}
	for _, file := range container.Files {
		if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil && file.ValueFrom.SecretRef.Name != "" {
			if _, dup := seen[file.ValueFrom.SecretRef.Name]; !dup {
				seen[file.ValueFrom.SecretRef.Name] = struct{}{}
				names = append(names, file.ValueFrom.SecretRef.Name)
			}
		}
	}
	return names
}
