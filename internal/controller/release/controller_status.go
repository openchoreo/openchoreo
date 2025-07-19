// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// updateStatus updates the Release status with applied resources
// Returns true if the status was updated, false if unchanged
func (r *Reconciler) updateStatus(ctx context.Context, old, release *openchoreov1alpha1.Release, appliedResources, liveResources []*unstructured.Unstructured) (bool, error) {
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
func (r *Reconciler) buildResourceStatus(ctx context.Context, old *openchoreov1alpha1.Release, desiredResources, liveResources []*unstructured.Unstructured) []openchoreov1alpha1.ResourceStatus {
	logger := log.FromContext(ctx)
	// Build a map of live resources for quick lookup by resource ID
	liveResourceMap := make(map[string]*unstructured.Unstructured)
	for _, liveObj := range liveResources {
		if resourceID := liveObj.GetLabels()[labels.LabelKeyReleaseResourceID]; resourceID != "" {
			liveResourceMap[resourceID] = liveObj
		}
	}

	// Build a map of old resource statuses for quick lookup by resource ID
	oldResourceMap := make(map[string]openchoreov1alpha1.ResourceStatus)
	for _, oldResource := range old.Status.Resources {
		oldResourceMap[oldResource.ID] = oldResource
	}

	resourceStatuses := make([]openchoreov1alpha1.ResourceStatus, 0, len(desiredResources))

	for _, desiredObj := range desiredResources {
		gvk := desiredObj.GroupVersionKind()
		resourceID := desiredObj.GetLabels()[labels.LabelKeyReleaseResourceID]

		var resourceStatus *runtime.RawExtension
		var lastObservedTime *metav1.Time
		healthStatus := openchoreov1alpha1.HealthStatusUnknown

		// Look up the live resource by ID
		if liveResource, found := liveResourceMap[resourceID]; found {
			// Extract status field if it exists
			if statusField, found, _ := unstructured.NestedFieldCopy(liveResource.Object, "status"); found && statusField != nil {
				// Convert status to RawExtension
				statusBytes, err := json.Marshal(statusField)
				if err == nil {
					resourceStatus = &runtime.RawExtension{Raw: statusBytes}
				} else {
					// Log marshaling error but continue
					logger.Error(err, "Failed to marshal resource status",
						"resourceID", resourceID,
						"gvk", gvk.String(),
						"namespace", liveResource.GetNamespace(),
						"name", liveResource.GetName())
				}
			}

			// Get health check function for this resource type
			healthCheckFunc := GetHealthCheckFunc(gvk)
			if healthCheckFunc != nil {
				health, err := healthCheckFunc(liveResource)
				if err != nil {
					logger.Error(err, "Failed to check resource health",
						"resourceID", resourceID,
						"gvk", gvk.String(),
						"namespace", liveResource.GetNamespace(),
						"name", liveResource.GetName())
					healthStatus = openchoreov1alpha1.HealthStatusUnknown
				} else {
					healthStatus = health
				}
			} else {
				// No health check function available, default to Unknown
				healthStatus = openchoreov1alpha1.HealthStatusUnknown
			}

			// Check if this resource existed before and if its status changed
			if oldResource, exists := oldResourceMap[resourceID]; exists {
				// Check if the resource status has actually changed
				if apiequality.Semantic.DeepEqual(oldResource.Status, resourceStatus) && oldResource.HealthStatus == healthStatus {
					// Status and health haven't changed, preserve the existing timestamp
					lastObservedTime = oldResource.LastObservedTime
				}
			}

			// Set current time if:
			// 1. This is a new resource (not in oldResourceMap), or
			// 2. The status or health changed (lastObservedTime is still nil)
			if lastObservedTime == nil {
				lastObservedTime = ptr.To(metav1.Now())
			}
		}

		status := openchoreov1alpha1.ResourceStatus{
			ID:               resourceID,
			Group:            gvk.Group,
			Version:          gvk.Version,
			Kind:             gvk.Kind,
			Name:             desiredObj.GetName(),
			Namespace:        desiredObj.GetNamespace(),
			Status:           resourceStatus,
			HealthStatus:     healthStatus,
			LastObservedTime: lastObservedTime,
		}

		resourceStatuses = append(resourceStatuses, status)
	}

	return resourceStatuses
}

// hasTransitioningResources checks if any resources are in a transitioning state
func (r *Reconciler) hasTransitioningResources(resources []openchoreov1alpha1.ResourceStatus) bool {
	for _, resource := range resources {
		// Check health status to determine if resource is transitioning
		// - Progressing: actively changing state (rolling update, scaling, etc.)
		// - Unknown: can't determine state (could be transitioning)
		// - Degraded: in error state but Kubernetes may be retrying (CrashLoopBackOff, ImagePullBackOff, etc.)
		if resource.HealthStatus == openchoreov1alpha1.HealthStatusProgressing ||
			resource.HealthStatus == openchoreov1alpha1.HealthStatusUnknown ||
			resource.HealthStatus == openchoreov1alpha1.HealthStatusDegraded {
			return true
		}
	}
	return false
}

func GetHealthCheckFunc(gvk schema.GroupVersionKind) func(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	switch {
	case gvk.Group == "apps" && gvk.Kind == "Deployment":
		return getDeploymentHealth
	case gvk.Group == "apps" && gvk.Kind == "StatefulSet":
		return getStatefulSetHealth
	case gvk.Group == "" && gvk.Kind == "Pod":
		return getPodHealth
	case gvk.Group == "batch" && gvk.Kind == "CronJob":
		return getCronJobHealth
		// TODO: Add gateway http route health check, and other resources as needed
	}
	return getUnknownResourceHealth
}

func getDeploymentHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// Convert unstructured object to Deployment
	var deployment appsv1.Deployment
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &deployment); err != nil {
		return openchoreov1alpha1.HealthStatusUnknown, fmt.Errorf("failed to convert to deployment: %w", err)
	}

	// Check if deployment is paused (suspended)
	if deployment.Spec.Paused {
		return openchoreov1alpha1.HealthStatusSuspended, nil
	}

	// If status is not populated yet, it's progressing
	if deployment.Status.ObservedGeneration == 0 || deployment.Generation > deployment.Status.ObservedGeneration {
		return openchoreov1alpha1.HealthStatusProgressing, nil
	}

	// Check deployment conditions for health status
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			// Check if progress deadline exceeded
			if condition.Reason == "ProgressDeadlineExceeded" {
				return openchoreov1alpha1.HealthStatusDegraded, nil
			}
		}
		if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
			return openchoreov1alpha1.HealthStatusDegraded, nil
		}
	}

	// Get desired replicas (default to 1 if not specified)
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}

	// Determine health based on replica counts
	updatedReplicas := deployment.Status.UpdatedReplicas
	readyReplicas := deployment.Status.ReadyReplicas
	availableReplicas := deployment.Status.AvailableReplicas

	// All replicas are up-to-date, ready, and available
	if desiredReplicas == updatedReplicas && desiredReplicas == readyReplicas && desiredReplicas == availableReplicas {
		return openchoreov1alpha1.HealthStatusHealthy, nil
	}

	// Deployment is in progress if:
	// - Not all replicas are updated
	// - Not all replicas are ready
	// - Not all replicas are available
	return openchoreov1alpha1.HealthStatusProgressing, nil
}

func getStatefulSetHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// Convert an unstructured object to StatefulSet
	var statefulSet appsv1.StatefulSet
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &statefulSet); err != nil {
		return openchoreov1alpha1.HealthStatusUnknown, fmt.Errorf("failed to convert to statefulset: %w", err)
	}

	// If status is not populated yet, it's progressing
	if statefulSet.Status.ObservedGeneration == 0 || statefulSet.Generation > statefulSet.Status.ObservedGeneration {
		return openchoreov1alpha1.HealthStatusProgressing, nil
	}

	// Get desired replicas (default to 1 if not specified)
	desiredReplicas := int32(1)
	if statefulSet.Spec.Replicas != nil {
		desiredReplicas = *statefulSet.Spec.Replicas
	}

	// Check for update in progress
	if statefulSet.Status.CurrentRevision != statefulSet.Status.UpdateRevision {
		return openchoreov1alpha1.HealthStatusProgressing, nil
	}

	// Determine health based on replica counts
	readyReplicas := statefulSet.Status.ReadyReplicas
	availableReplicas := statefulSet.Status.AvailableReplicas
	updatedReplicas := statefulSet.Status.UpdatedReplicas

	// All replicas are ready, available, and updated
	if desiredReplicas == readyReplicas && desiredReplicas == availableReplicas && desiredReplicas == updatedReplicas {
		return openchoreov1alpha1.HealthStatusHealthy, nil
	}

	// If we have some ready replicas but not all, it's progressing
	return openchoreov1alpha1.HealthStatusProgressing, nil
}

func getPodHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// Convert an unstructured object to Pod
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil {
		return openchoreov1alpha1.HealthStatusUnknown, fmt.Errorf("failed to convert to pod: %w", err)
	}

	// Check pod phase
	switch pod.Status.Phase {
	case corev1.PodPending:
		return openchoreov1alpha1.HealthStatusProgressing, nil
	case corev1.PodRunning:
		// Check if all containers are ready
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if !containerStatus.Ready {
				return openchoreov1alpha1.HealthStatusProgressing, nil
			}
			// Check if container is in a waiting state with error
			if containerStatus.State.Waiting != nil {
				if containerStatus.State.Waiting.Reason == "CrashLoopBackOff" ||
					containerStatus.State.Waiting.Reason == "ImagePullBackOff" ||
					containerStatus.State.Waiting.Reason == "ErrImagePull" {
					return openchoreov1alpha1.HealthStatusDegraded, nil
				}
			}
		}
		return openchoreov1alpha1.HealthStatusHealthy, nil
	case corev1.PodSucceeded:
		return openchoreov1alpha1.HealthStatusHealthy, nil
	case corev1.PodFailed:
		return openchoreov1alpha1.HealthStatusDegraded, nil
	case corev1.PodUnknown:
		return openchoreov1alpha1.HealthStatusUnknown, nil
	default:
		return openchoreov1alpha1.HealthStatusUnknown, nil
	}
}

func getCronJobHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// Convert unstructured object to CronJob
	var cronJob batchv1.CronJob
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cronJob); err != nil {
		return openchoreov1alpha1.HealthStatusUnknown, fmt.Errorf("failed to convert to cronjob: %w", err)
	}

	// Check if CronJob is suspended
	if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
		return openchoreov1alpha1.HealthStatusSuspended, nil
	}

	// Check active jobs
	activeJobs := len(cronJob.Status.Active)

	// TODO: Fine tune - what makes a CronJob healthy in OpenChoreo?

	// If there's an active job, it's progressing
	if activeJobs > 0 {
		return openchoreov1alpha1.HealthStatusProgressing, nil
	}

	// Check last schedule time
	if cronJob.Status.LastScheduleTime != nil {
		// CronJob has run at least once, consider it healthy
		return openchoreov1alpha1.HealthStatusHealthy, nil
	}

	// CronJob hasn't run yet - could be newly created or waiting for schedule
	return openchoreov1alpha1.HealthStatusProgressing, nil
}

func getUnknownResourceHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// For unknown resources, we can't determine health status reliably
	// Resources like ConfigMaps, Secrets, Services, etc. don't have meaningful health states
	// They are either present or not, so if we got here, they exist
	return openchoreov1alpha1.HealthStatusHealthy, nil
}
