// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

const (
	// secretReferencesIndex is the field index name for SecretReference names used in ReleaseBinding
	// This index tracks all SecretReferences from both ComponentRelease workload and ReleaseBinding workloadOverrides
	secretReferencesIndex = "spec.secretReferences"
)

// setupSecretReferencesIndex sets up the field index for SecretReference names used by ReleaseBinding.
// This index extracts all SecretReference names from the merged workload
// (ComponentRelease.Spec.Workload merged with ReleaseBinding.Spec.WorkloadOverrides).
func (r *Reconciler) setupSecretReferencesIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.ReleaseBinding{},
		secretReferencesIndex, func(obj client.Object) []string {
			releaseBinding := obj.(*openchoreov1alpha1.ReleaseBinding)
			logger := log.FromContext(ctx)

			// Fetch ComponentRelease to get the base workload
			if releaseBinding.Spec.ReleaseName == "" {
				return []string{}
			}

			componentRelease := &openchoreov1alpha1.ComponentRelease{}
			if err := r.Get(ctx, types.NamespacedName{
				Name:      releaseBinding.Spec.ReleaseName,
				Namespace: releaseBinding.Namespace,
			}, componentRelease); err != nil {
				logger.Info("Failed to get ComponentRelease for index",
					"releaseBinding", releaseBinding.Name,
					"componentRelease", releaseBinding.Spec.ReleaseName,
					"error", err)
				return []string{}
			}

			// Build workload from ComponentRelease
			baseWorkload := &openchoreov1alpha1.Workload{
				Spec: openchoreov1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: componentRelease.Spec.Workload,
				},
			}

			// Merge workload with overrides using the shared function
			mergedWorkload := pipelinecontext.MergeWorkloadOverrides(baseWorkload, releaseBinding.Spec.WorkloadOverrides)
			if mergedWorkload == nil {
				return []string{}
			}

			// Extract SecretReferences from the merged workload
			var secretRefNames []string
			container := mergedWorkload.Spec.Container
			// Extract from Env variables
			for _, env := range container.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil && env.ValueFrom.SecretRef.Name != "" {
					secretRefNames = append(secretRefNames, env.ValueFrom.SecretRef.Name)
				}
			}

			// Extract from Files
			for _, file := range container.Files {
				if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil && file.ValueFrom.SecretRef.Name != "" {
					secretRefNames = append(secretRefNames, file.ValueFrom.SecretRef.Name)
				}
			}

			return secretRefNames
		})
}

// listReleaseBindingsForSecretReference returns reconcile requests for all ReleaseBindings
// that use the changed SecretReference (either in ComponentRelease's workload or ReleaseBinding's workloadOverrides)
func (r *Reconciler) listReleaseBindingsForSecretReference(ctx context.Context, obj client.Object) []reconcile.Request {
	secretRef := obj.(*openchoreov1alpha1.SecretReference)
	logger := log.FromContext(ctx)

	logger.Info("SecretReference changed, finding affected ReleaseBindings",
		"secretReference", secretRef.Name,
		"namespace", secretRef.Namespace)

	// Find all ReleaseBindings in the same namespace that use this SecretReference
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

	// List all ReleaseBindings that reference this Component
	var bindings openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &bindings,
		client.InNamespace(component.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: component.Name}); err != nil {
		return nil
	}

	// Create reconcile requests for each ReleaseBinding
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
