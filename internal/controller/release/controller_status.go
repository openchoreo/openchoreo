// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// updateStatus updates the Release status with applied resources
// Returns true if the status was updated, false if unchanged
func (r *Reconciler) updateStatus(ctx context.Context, old, release *choreov1.Release, appliedResources, liveResources []*unstructured.Unstructured) (bool, error) {
	logger := log.FromContext(ctx)

	// Build resource status from applied and live resources
	resourceStatuses := r.buildResourceStatus(ctx, old, appliedResources, liveResources)

	// Update the status
	release.Status.Resources = resourceStatuses

	// Check if the entire status actually changed and skip update if not
	if apiequality.Semantic.DeepEqual(old.Status, release.Status) {
		return false, nil
	}

	// Update the Release status
	if err := r.Status().Update(ctx, release); err != nil {
		logger.Error(err, "Failed to update Release status")
		return false, fmt.Errorf("failed to update status: %w", err)
	}

	return true, nil
}

// buildResourceStatus converts applied unstructured objects to ResourceStatus entries using live resources
func (r *Reconciler) buildResourceStatus(ctx context.Context, old *choreov1.Release, desiredResources, liveResources []*unstructured.Unstructured) []choreov1.ResourceStatus {
	logger := log.FromContext(ctx)
	// Build a map of live resources for quick lookup by resource ID
	liveResourceMap := make(map[string]*unstructured.Unstructured)
	for _, liveObj := range liveResources {
		if resourceID := liveObj.GetLabels()[labels.LabelKeyReleaseResourceID]; resourceID != "" {
			liveResourceMap[resourceID] = liveObj
		}
	}

	// Build a map of old resource statuses for quick lookup by resource ID
	oldResourceMap := make(map[string]choreov1.ResourceStatus)
	for _, oldResource := range old.Status.Resources {
		oldResourceMap[oldResource.ID] = oldResource
	}

	var resourceStatuses []choreov1.ResourceStatus

	for _, desiredObj := range desiredResources {
		gvk := desiredObj.GroupVersionKind()
		resourceID := desiredObj.GetLabels()[labels.LabelKeyReleaseResourceID]

		var resourceStatus *runtime.RawExtension
		var lastObservedTime *metav1.Time

		// Look up the live resource by ID
		if liveResource, found := liveResourceMap[resourceID]; found {
			// Extract status field if it exists
			if statusField, found, _ := unstructured.NestedFieldCopy(liveResource.Object, "status"); found && statusField != nil {
				// Convert status to RawExtension
				statusBytes, err := json.Marshal(statusField)
				if err == nil {
					resourceStatus = &runtime.RawExtension{Raw: statusBytes}
				} else {
					// Log marshalling error but continue
					logger.Error(err, "Failed to marshal resource status",
						"resourceID", resourceID,
						"gvk", gvk.String(),
						"namespace", liveResource.GetNamespace(),
						"name", liveResource.GetName())
				}
			}

			// Check if this resource existed before and if its status changed
			if oldResource, exists := oldResourceMap[resourceID]; exists {
				// Check if the resource status has actually changed
				if apiequality.Semantic.DeepEqual(oldResource.Status, resourceStatus) {
					// Status hasn't changed, preserve the existing timestamp
					lastObservedTime = oldResource.LastObservedTime
				}
			}

			// Set current time if:
			// 1. This is a new resource (not in oldResourceMap), or
			// 2. The status changed (lastObservedTime is still nil)
			if lastObservedTime == nil {
				lastObservedTime = ptr.To(metav1.Now())
			}
		}

		status := choreov1.ResourceStatus{
			ID:               resourceID,
			Group:            gvk.Group,
			Version:          gvk.Version,
			Kind:             gvk.Kind,
			Name:             desiredObj.GetName(),
			Namespace:        desiredObj.GetNamespace(),
			Status:           resourceStatus,
			LastObservedTime: lastObservedTime,
		}

		resourceStatuses = append(resourceStatuses, status)
	}

	return resourceStatuses
}

// hasTransitioningResources checks if any resources are in a transitioning state
func (r *Reconciler) hasTransitioningResources(resources []choreov1.ResourceStatus) bool {
	for _, resource := range resources {
		// Skip resources without status (ConfigMaps, Secrets, etc.)
		if resource.Status == nil {
			continue
		}

		// Check if this specific resource is transitioning
		if r.isResourceTransitioning(resource) {
			return true
		}
	}
	return false
}

// isResourceTransitioning checks if a specific resource is in a transitioning state
func (r *Reconciler) isResourceTransitioning(resource choreov1.ResourceStatus) bool {
	switch {
	case resource.Group == "apps" && resource.Kind == "Deployment":
		return r.isDeploymentTransitioning(resource.Status)
	case resource.Group == "apps" && resource.Kind == "StatefulSet":
		return r.isStatefulSetTransitioning(resource.Status)
	case resource.Group == "" && resource.Kind == "Pod":
		return r.isPodTransitioning(resource.Status)
	default:
		// For unknown resource types, we don't consider the transitioning states
		// This includes ConfigMaps, Secrets, ServiceAccounts, etc.
		return false
	}
}

// isDeploymentTransitioning checks if a Deployment is rolling out
func (r *Reconciler) isDeploymentTransitioning(statusRaw *runtime.RawExtension) bool {
	if statusRaw == nil {
		return false
	}

	var deploymentStatus appsv1.DeploymentStatus
	if err := json.Unmarshal(statusRaw.Raw, &deploymentStatus); err != nil {
		return true // assume transitioning if we can't parse
	}

	// Transitioning if:
	// - unavailableReplicas > 0: pods are not yet available
	// - replicas != readyReplicas: pods exist but aren't ready
	// - replicas != updatedReplicas: rolling update in progress
	return deploymentStatus.UnavailableReplicas > 0 ||
		deploymentStatus.Replicas != deploymentStatus.ReadyReplicas ||
		deploymentStatus.Replicas != deploymentStatus.UpdatedReplicas
}

// isStatefulSetTransitioning checks if a StatefulSet is updating
func (r *Reconciler) isStatefulSetTransitioning(statusRaw *runtime.RawExtension) bool {
	if statusRaw == nil {
		return false
	}

	var statefulSetStatus appsv1.StatefulSetStatus
	if err := json.Unmarshal(statusRaw.Raw, &statefulSetStatus); err != nil {
		return true // assume transitioning if we can't parse
	}

	// Transitioning if:
	// - replicas != readyReplicas: pods exist but aren't ready
	// - replicas != availableReplicas: pods aren't available (ready for minReadySeconds)
	// - currentReplicas != updatedReplicas: rolling update in progress
	// - replicas != updatedReplicas: not all pods updated to latest version
	return statefulSetStatus.Replicas != statefulSetStatus.ReadyReplicas ||
		statefulSetStatus.Replicas != statefulSetStatus.AvailableReplicas ||
		statefulSetStatus.CurrentReplicas != statefulSetStatus.UpdatedReplicas ||
		statefulSetStatus.Replicas != statefulSetStatus.UpdatedReplicas
}

// isPodTransitioning checks if a Pod is in a transitioning phase
func (r *Reconciler) isPodTransitioning(statusRaw *runtime.RawExtension) bool {
	if statusRaw == nil {
		return false
	}

	var podStatus corev1.PodStatus
	if err := json.Unmarshal(statusRaw.Raw, &podStatus); err != nil {
		return true // assume transitioning if we can't parse
	}

	// Pod phases: Pending, Running, Succeeded, Failed, Unknown
	// - Pending: Pod accepted but containers not created yet (transitioning)
	// - Unknown: Pod state couldn't be obtained (transitioning)
	// - Running: Pod is stable and running (unless it has deletionTimestamp)
	// - Succeeded/Failed: Terminal states, not transitioning
	//
	// Note: "Terminating" is not a phase but indicated by metadata.deletionTimestamp
	return podStatus.Phase == corev1.PodPending || podStatus.Phase == corev1.PodUnknown
}
