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

	// resourceDependencyTargetsIndex indexes ReleaseBindings by their resource dependency
	// targets (namespace/project/resourceName/environment) for efficient reverse lookup
	// when a provider ResourceReleaseBinding's outputs or Ready condition change.
	resourceDependencyTargetsIndex = "status.resourceDependencyTargets"
)

// makeConnectionTargetKey creates an index key for a connection target.
func makeConnectionTargetKey(namespace, project, component, environment string) string {
	return namespace + "/" + project + "/" + component + "/" + environment
}

// makeResourceDependencyTargetKey creates an index key for a resource dependency target.
// Same shape as makeConnectionTargetKey but with resourceName in the third slot.
func makeResourceDependencyTargetKey(namespace, project, resourceName, environment string) string {
	return namespace + "/" + project + "/" + resourceName + "/" + environment
}

// indexResourceDependencyTargets is the field-indexer function for resourceDependencyTargetsIndex.
// Exported helper kept package-private so unit tests can register the same indexer the
// production setup uses on the manager's cache.
func indexResourceDependencyTargets(rb *openchoreov1alpha1.ReleaseBinding) []string {
	if len(rb.Status.ResourceDependencyTargets) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var keys []string
	for _, t := range rb.Status.ResourceDependencyTargets {
		key := makeResourceDependencyTargetKey(t.Namespace, t.Project, t.ResourceName, t.Environment)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return keys
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
				key := makeConnectionTargetKey(conn.Namespace, conn.Project, conn.Component, conn.Environment)
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

// setupResourceDependencyTargetsIndex registers a field index that extracts unique
// namespace/project/resourceName/environment keys from each ReleaseBinding's
// status.resourceDependencyTargets.
func (r *Reconciler) setupResourceDependencyTargetsIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ReleaseBinding{},
		resourceDependencyTargetsIndex, func(obj client.Object) []string {
			return indexResourceDependencyTargets(obj.(*openchoreov1alpha1.ReleaseBinding))
		})
}

// resourceReleaseBindingOutputsChangedPredicate returns a predicate that passes when a
// ResourceReleaseBinding update affects what consumers see: outputs change, generation
// advances (spec edit by PE), or the Ready condition's Status / ObservedGeneration shift.
// Other status changes (e.g., Synced reason updates) are filtered out. Tracking generation
// and ObservedGeneration matches the consumer-side gate in isResourceReleaseBindingReady,
// so a provider mid-reconcile re-enqueues consumers when it catches up.
func resourceReleaseBindingOutputsChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldRRB, ok := e.ObjectOld.(*openchoreov1alpha1.ResourceReleaseBinding)
			if !ok {
				return false
			}
			newRRB, ok := e.ObjectNew.(*openchoreov1alpha1.ResourceReleaseBinding)
			if !ok {
				return false
			}
			if oldRRB.Generation != newRRB.Generation {
				return true
			}
			if !apiequality.Semantic.DeepEqual(oldRRB.Status.Outputs, newRRB.Status.Outputs) {
				return true
			}
			return readyConditionChanged(oldRRB, newRRB)
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

// readyConditionChanged reports whether the Ready condition's Status or ObservedGeneration
// differs between two ResourceReleaseBindings. Absence on either side maps to "" / 0, so a
// True ↔ absent transition still counts as a flip. The ObservedGeneration check matches the
// consumer-side gate in isResourceReleaseBindingReady — a provider that updates Ready in
// place to track a new generation must re-enqueue consumers even if Status stays True.
func readyConditionChanged(oldRRB, newRRB *openchoreov1alpha1.ResourceReleaseBinding) bool {
	const readyType = "Ready"
	var oldStatus, newStatus string
	var oldObserved, newObserved int64
	for i := range oldRRB.Status.Conditions {
		if oldRRB.Status.Conditions[i].Type == readyType {
			oldStatus = string(oldRRB.Status.Conditions[i].Status)
			oldObserved = oldRRB.Status.Conditions[i].ObservedGeneration
			break
		}
	}
	for i := range newRRB.Status.Conditions {
		if newRRB.Status.Conditions[i].Type == readyType {
			newStatus = string(newRRB.Status.Conditions[i].Status)
			newObserved = newRRB.Status.Conditions[i].ObservedGeneration
			break
		}
	}
	return oldStatus != newStatus || oldObserved != newObserved
}

// findConsumerReleaseBindingsForResourceReleaseBinding returns reconcile requests for all
// ReleaseBindings that depend on the changed ResourceReleaseBinding via their workload's
// dependencies.resources[]. Used by Watches(&ResourceReleaseBinding{}, ...) to propagate
// provider output changes to consumers.
func (r *Reconciler) findConsumerReleaseBindingsForResourceReleaseBinding(ctx context.Context, obj client.Object) []reconcile.Request {
	rrb, ok := obj.(*openchoreov1alpha1.ResourceReleaseBinding)
	if !ok {
		return nil
	}
	// Bail early on a malformed RRB so we never produce a partial key like "ns///" that
	// would over-list consumers carrying any zero-value targets. Mirrors the same guard
	// in IndexResourceReleaseBindingOwnerEnv (internal/controller/watch.go).
	if rrb.Spec.Owner.ProjectName == "" || rrb.Spec.Owner.ResourceName == "" || rrb.Spec.Environment == "" {
		return nil
	}

	targetKey := makeResourceDependencyTargetKey(
		rrb.Namespace,
		rrb.Spec.Owner.ProjectName,
		rrb.Spec.Owner.ResourceName,
		rrb.Spec.Environment,
	)

	var consumers openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &consumers,
		client.MatchingFields{resourceDependencyTargetsIndex: targetKey}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list consumer ReleaseBindings for ResourceReleaseBinding",
			"resourceReleaseBinding", rrb.Name)
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
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name != "" {
			if _, dup := seen[env.ValueFrom.SecretKeyRef.Name]; !dup {
				seen[env.ValueFrom.SecretKeyRef.Name] = struct{}{}
				names = append(names, env.ValueFrom.SecretKeyRef.Name)
			}
		}
	}
	for _, file := range container.Files {
		if file.ValueFrom != nil && file.ValueFrom.SecretKeyRef != nil && file.ValueFrom.SecretKeyRef.Name != "" {
			if _, dup := seen[file.ValueFrom.SecretKeyRef.Name]; !dup {
				seen[file.ValueFrom.SecretKeyRef.Name] = struct{}{}
				names = append(names, file.ValueFrom.SecretKeyRef.Name)
			}
		}
	}
	return names
}
