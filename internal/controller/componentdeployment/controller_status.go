// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentdeployment

import (
	"context"
	"fmt"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// setResourcesReadyStatus sets the ResourcesReady condition based on the Release resource status.
// It aggregates health information from all resources in the Release.
func (r *Reconciler) setResourcesReadyStatus(_ context.Context, componentDeployment *openchoreov1alpha1.ComponentDeployment, release *openchoreov1alpha1.Release) error { //nolint:unparam // Error return for future extensibility
	// Count resources by health status
	totalResources := len(release.Status.Resources)

	// Handle the case where there are no resources - this should not happen
	if totalResources == 0 {
		message := "No resources in Release - expected at least one workload resource"
		controller.MarkFalseCondition(componentDeployment, ConditionResourcesReady, ReasonResourcesDegraded, message)
		return nil
	}

	healthyCount := 0
	progressingCount := 0
	degradedCount := 0
	suspendedCount := 0

	// Check all resources using their health status
	for _, resource := range release.Status.Resources {
		switch resource.HealthStatus {
		case openchoreov1alpha1.HealthStatusHealthy:
			healthyCount++
		case openchoreov1alpha1.HealthStatusSuspended:
			suspendedCount++
		case openchoreov1alpha1.HealthStatusProgressing, openchoreov1alpha1.HealthStatusUnknown:
			// Treat both progressing and unknown as progressing
			progressingCount++
		case openchoreov1alpha1.HealthStatusDegraded:
			degradedCount++
		default:
			// Treat any unrecognized health status as progressing
			progressingCount++
		}
	}

	// Check if all resources are ready (only healthy counts as ready now)
	allResourcesReady := healthyCount == totalResources

	// Set the ResourcesReady condition based on resource health status
	if allResourcesReady {
		message := fmt.Sprintf("All %d resources are deployed and ready", totalResources)
		controller.MarkTrueCondition(componentDeployment, ConditionResourcesReady, ReasonResourcesReady, message)
	} else {
		// Build a status message with counts
		var statusParts []string

		if progressingCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d progressing", progressingCount, totalResources))
		}
		if degradedCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d degraded", degradedCount, totalResources))
		}
		if healthyCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d ready", healthyCount, totalResources))
		}
		if suspendedCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d suspended", suspendedCount, totalResources))
		}

		// Determine reason using priority: Progressing > Degraded
		var reason controller.ConditionReason
		var message string

		if progressingCount > 0 {
			// If any resource is progressing, the whole deployment is progressing
			reason = ReasonResourcesProgressing
		} else {
			// Only degraded resources
			reason = ReasonResourcesDegraded
		}

		message = fmt.Sprintf("Resources status: %s", strings.Join(statusParts, ", "))
		controller.MarkFalseCondition(componentDeployment, ConditionResourcesReady, reason, message)
	}

	return nil
}
