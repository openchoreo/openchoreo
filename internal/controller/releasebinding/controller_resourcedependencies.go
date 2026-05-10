// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/resourcereleasebinding"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

// buildResourceDependencyTargets extracts ResourceDependencyTarget entries from the
// workload's resource dependencies. Pure function with no API calls. Resource
// dependencies are project-bound: each target inherits the consumer ReleaseBinding's
// namespace, project, and environment.
func buildResourceDependencyTargets(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	deps []openchoreov1alpha1.WorkloadResourceDependency,
) []openchoreov1alpha1.ResourceDependencyTarget {
	if len(deps) == 0 {
		return nil
	}
	targets := make([]openchoreov1alpha1.ResourceDependencyTarget, 0, len(deps))
	for _, dep := range deps {
		targets = append(targets, openchoreov1alpha1.ResourceDependencyTarget{
			Namespace:    releaseBinding.Namespace,
			Project:      releaseBinding.Spec.Owner.ProjectName,
			ResourceName: dep.Ref,
			Environment:  releaseBinding.Spec.Environment,
		})
	}
	return targets
}

// allResourceDependenciesResolved reports whether every declared resource dependency has
// been resolved. The resolver populates Status.PendingResourceDependencies on failure;
// emptiness of that list with at least one declared dep means all resolved.
func allResourceDependenciesResolved(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	deps []openchoreov1alpha1.WorkloadResourceDependency,
) bool {
	if len(deps) == 0 {
		return true
	}
	return len(releaseBinding.Status.PendingResourceDependencies) == 0
}

// setResourceDependenciesCondition sets the ResourceDependenciesReady condition on the
// ReleaseBinding based on how many declared dependencies have been resolved.
func setResourceDependenciesCondition(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	allResolved bool,
) {
	totalCount := len(releaseBinding.Status.ResourceDependencyTargets)
	if totalCount == 0 {
		controller.MarkTrueCondition(releaseBinding, ConditionResourceDependenciesReady,
			ReasonNoResourceDependencies, "No resource dependencies to resolve")
		return
	}

	if allResolved {
		controller.MarkTrueCondition(releaseBinding, ConditionResourceDependenciesReady,
			ReasonAllResourceDependenciesReady,
			fmt.Sprintf("All %d resource dependencies resolved", totalCount))
		return
	}

	pendingCount := len(releaseBinding.Status.PendingResourceDependencies)
	resolvedCount := totalCount - pendingCount
	controller.MarkFalseCondition(releaseBinding, ConditionResourceDependenciesReady,
		ReasonResourceDependenciesPending,
		fmt.Sprintf("%d resource dependencies pending, %d resolved", pendingCount, resolvedCount))
}

// populateResourceDependencyStatus is the reconcile-chain entry point for resource
// dependencies. It writes Status.ResourceDependencyTargets and Status.PendingResourceDependencies
// in one place, returning the resolved per-dep items for the pipeline. Wraps the resolver
// orchestrator so callers don't have to remember the three-step status-write dance.
func (r *Reconciler) populateResourceDependencyStatus(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	deps []openchoreov1alpha1.WorkloadResourceDependency,
) ([]pipelinecontext.ResourceDependencyItem, error) {
	releaseBinding.Status.ResourceDependencyTargets = buildResourceDependencyTargets(releaseBinding, deps)
	items, pending, err := r.resolveResourceDependencies(ctx, releaseBinding, deps)
	if err != nil {
		return nil, err
	}
	releaseBinding.Status.PendingResourceDependencies = pending
	return items, nil
}

// resolveResourceDependencies resolves every workload resource dependency for the consumer
// ReleaseBinding. For each dep it looks up the matching provider ResourceReleaseBinding via
// the (project, resource, environment) field index, reads its status.outputs, and dispatches
// them through BuildResourceDependencyItem to produce a ResourceDependencyItem. Failure
// modes (provider missing, provider not Ready, output not yet resolved) populate
// PendingResourceDependency entries instead.
//
// Transient API errors abort the entire orchestrator so the caller requeues without acting
// on a partial view.
func (r *Reconciler) resolveResourceDependencies(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	deps []openchoreov1alpha1.WorkloadResourceDependency,
) ([]pipelinecontext.ResourceDependencyItem, []openchoreov1alpha1.PendingResourceDependency, error) {
	if len(deps) == 0 {
		return nil, nil, nil
	}

	var items []pipelinecontext.ResourceDependencyItem
	var pending []openchoreov1alpha1.PendingResourceDependency

	for _, dep := range deps {
		item, p, err := r.resolveResourceDependency(ctx, releaseBinding, dep)
		if err != nil {
			return nil, nil, err
		}
		if p != nil {
			pending = append(pending, *p)
		} else if item != nil {
			items = append(items, *item)
		}
	}

	return items, pending, nil
}

// resolveResourceDependency resolves a single resource dependency. Returns either a
// ResourceDependencyItem (success) or a PendingResourceDependency (deferred to a later
// reconcile). A non-nil error indicates a transient API failure the caller should requeue
// on; in that case both returned pointers are nil.
func (r *Reconciler) resolveResourceDependency(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	dep openchoreov1alpha1.WorkloadResourceDependency,
) (*pipelinecontext.ResourceDependencyItem, *openchoreov1alpha1.PendingResourceDependency, error) {
	indexKey := controller.MakeResourceReleaseBindingOwnerEnvKey(
		releaseBinding.Spec.Owner.ProjectName,
		dep.Ref,
		releaseBinding.Spec.Environment,
	)

	var rrbList openchoreov1alpha1.ResourceReleaseBindingList
	if err := r.List(ctx, &rrbList,
		client.InNamespace(releaseBinding.Namespace),
		client.MatchingFields{controller.IndexKeyResourceReleaseBindingOwnerEnv: indexKey}); err != nil {
		return nil, nil, fmt.Errorf("failed to list ResourceReleaseBindings for %s/%s in %s: %w",
			releaseBinding.Spec.Owner.ProjectName, dep.Ref, releaseBinding.Spec.Environment, err)
	}

	if len(rrbList.Items) == 0 {
		return nil, &openchoreov1alpha1.PendingResourceDependency{
			Namespace:    releaseBinding.Namespace,
			Project:      releaseBinding.Spec.Owner.ProjectName,
			ResourceName: dep.Ref,
			Reason: fmt.Sprintf("ResourceReleaseBinding not found for %s/%s in environment %s",
				releaseBinding.Spec.Owner.ProjectName, dep.Ref, releaseBinding.Spec.Environment),
		}, nil
	}

	if len(rrbList.Items) > 1 {
		return nil, &openchoreov1alpha1.PendingResourceDependency{
			Namespace:    releaseBinding.Namespace,
			Project:      releaseBinding.Spec.Owner.ProjectName,
			ResourceName: dep.Ref,
			Reason: fmt.Sprintf("multiple ResourceReleaseBindings found for %s/%s in environment %s",
				releaseBinding.Spec.Owner.ProjectName, dep.Ref, releaseBinding.Spec.Environment),
		}, nil
	}

	rrb := &rrbList.Items[0]

	if !isResourceReleaseBindingReady(rrb) {
		return nil, &openchoreov1alpha1.PendingResourceDependency{
			Namespace:    releaseBinding.Namespace,
			Project:      releaseBinding.Spec.Owner.ProjectName,
			ResourceName: dep.Ref,
			Reason:       fmt.Sprintf("ResourceReleaseBinding %q not ready", rrb.Name),
		}, nil
	}

	item, err := pipelinecontext.BuildResourceDependencyItem(dep, rrb.Status.Outputs)
	if err != nil {
		// ErrOutputNotResolved is transient — surface as pending so the consumer waits for
		// the provider to populate the output. ErrInvalidFileBinding is a workload-spec
		// misconfiguration; surface it the same way so the developer sees the reason on
		// kubectl describe (a future webhook will reject these at admission instead).
		if errors.Is(err, pipelinecontext.ErrOutputNotResolved) ||
			errors.Is(err, pipelinecontext.ErrInvalidFileBinding) {
			return nil, &openchoreov1alpha1.PendingResourceDependency{
				Namespace:    releaseBinding.Namespace,
				Project:      releaseBinding.Spec.Owner.ProjectName,
				ResourceName: dep.Ref,
				Reason:       err.Error(),
			}, nil
		}
		return nil, nil, fmt.Errorf("failed to build resource dependency item for %s: %w", dep.Ref, err)
	}

	return &item, nil, nil
}

// isResourceReleaseBindingReady reports whether the given binding's Ready condition is True
// for the current generation. Ready aggregates Synced + ResourcesReady + OutputsResolved,
// so consumers wait for full steady-state on the provider rather than wiring against a
// half-applied state. The ObservedGeneration check ensures we don't accept a stale Ready
// from a prior reconcile when the provider's spec has just changed.
func isResourceReleaseBindingReady(rrb *openchoreov1alpha1.ResourceReleaseBinding) bool {
	cond := meta.FindStatusCondition(rrb.Status.Conditions, string(resourcereleasebinding.ConditionReady))
	if cond == nil {
		return false
	}
	return cond.Status == metav1.ConditionTrue && cond.ObservedGeneration == rrb.Generation
}
