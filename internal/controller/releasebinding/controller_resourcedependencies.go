// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// buildResourceDependencyTargets extracts ResourceDependencyTarget entries from the
// workload's resource dependencies. Pure function with no API calls. v1.1 is project-bound:
// each target inherits the consumer ReleaseBinding's namespace, project, and environment.
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
