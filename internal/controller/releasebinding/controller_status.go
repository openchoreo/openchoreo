// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// ResourceCategory defines the category of a resource for status evaluation.
// Different categories have different readiness evaluation criteria.
type ResourceCategory string

const (
	// CategoryPrimaryWorkload - The main workload resource (Deployment, StatefulSet, Job, CronJob)
	// This is the most important resource for determining component readiness
	CategoryPrimaryWorkload ResourceCategory = "primary-workload"

	// CategorySupporting - Supporting infrastructure resources (Service, PVC, etc.)
	// These should exist but have simple status checks
	CategorySupporting ResourceCategory = "supporting"

	// CategoryOperational - Operational resources that may have complex states (HPA, HTTPRoute, etc.)
	// These can be progressing without blocking readiness
	CategoryOperational ResourceCategory = "operational"

	// CategoryNoStatus - Resources without meaningful status semantics (ConfigMap, Secret, etc.)
	// These are skipped in status evaluation (following ArgoCD's approach)
	CategoryNoStatus ResourceCategory = "no-status"

	// appsAPIGroup is the API group for core Kubernetes workload resources
	appsAPIGroup = "apps"
	// batchAPIGroup is the API group for Kubernetes batch workload resources
	batchAPIGroup = "batch"
)

// ResourceStatusSummary aggregates health status counts for resources
type ResourceStatusSummary struct {
	Total       int
	Healthy     int
	Progressing int
	Degraded    int
	Suspended   int
	Unknown     int
}

// setResourcesReadyStatus evaluates resource status from the Release
// and sets the ResourcesReady condition with workload-type specific logic.
//
// nolint:unparam // ctx and error return kept for consistency with other status methods
func (r *Reconciler) setResourcesReadyStatus(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	release *openchoreov1alpha1.Release,
	component *openchoreov1alpha1.Component,
) error {
	logger := log.FromContext(ctx)

	// Extract workload type from Component's ComponentType field
	componentTypeName := component.Spec.ComponentType.Name
	workloadType := extractWorkloadType(componentTypeName)

	logger.Info("Evaluating resource status",
		"componentType", componentTypeName,
		"workloadType", workloadType,
		"resourceCount", len(release.Status.Resources))

	// If Release has no resources yet, mark as Progressing
	if len(release.Status.Resources) == 0 {
		msg := fmt.Sprintf("Release %q has no resources yet", release.Name)
		controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady,
			ReasonResourcesProgressing, msg)
		return nil
	}

	// Evaluate readiness based on workload type
	var ready bool
	var reason, message string

	switch workloadType {
	case WorkloadTypeDeployment:
		ready, reason, message = evaluateDeploymentStatus(release.Status.Resources, workloadType)

	case WorkloadTypeStatefulSet:
		ready, reason, message = evaluateStatefulSetStatus(release.Status.Resources, workloadType)

	case WorkloadTypeCronJob:
		ready, reason, message = evaluateCronJobStatus(release.Status.Resources, workloadType)

	case WorkloadTypeJob:
		ready, reason, message = evaluateJobStatus(release.Status.Resources, workloadType)

	case WorkloadTypeProxy:
		// Proxy components are generic resources without traditional workload semantics
		ready, reason, message = evaluateGenericStatus(release.Status.Resources)

	case WorkloadTypeUnknown:
		// Fallback for unknown workload types or legacy components
		ready, reason, message = evaluateGenericStatus(release.Status.Resources)
		logger.Info("Using generic status evaluation for unknown workload type",
			"componentType", componentTypeName)
	}

	// Set the ResourcesReady condition
	if ready {
		controller.MarkTrueCondition(releaseBinding, ConditionResourcesReady,
			controller.ConditionReason(reason), message)
	} else {
		controller.MarkFalseCondition(releaseBinding, ConditionResourcesReady,
			controller.ConditionReason(reason), message)
	}

	return nil
}

// setReadyCondition sets the top-level Ready condition based on
// ReleaseSynced and ResourcesReady conditions.
func (r *Reconciler) setReadyCondition(releaseBinding *openchoreov1alpha1.ReleaseBinding) {
	// Find ReleaseSynced condition
	var releaseSynced *metav1.Condition
	for i := range releaseBinding.Status.Conditions {
		if releaseBinding.Status.Conditions[i].Type == string(ConditionReleaseSynced) {
			releaseSynced = &releaseBinding.Status.Conditions[i]
			break
		}
	}

	// Find ResourcesReady condition
	var resourcesReady *metav1.Condition
	for i := range releaseBinding.Status.Conditions {
		if releaseBinding.Status.Conditions[i].Type == string(ConditionResourcesReady) {
			resourcesReady = &releaseBinding.Status.Conditions[i]
			break
		}
	}

	// Both must be True for Ready to be True
	if releaseSynced != nil && releaseSynced.Status == metav1.ConditionTrue &&
		resourcesReady != nil && resourcesReady.Status == metav1.ConditionTrue {
		controller.MarkTrueCondition(releaseBinding, ConditionReady,
			ReasonReady, "ReleaseBinding is ready")
		return
	}

	// If ReleaseSynced is not True, use its reason
	if releaseSynced == nil || releaseSynced.Status != metav1.ConditionTrue {
		reason := ReasonReleaseSynced
		message := "Release is not synced"
		if releaseSynced != nil {
			reason = controller.ConditionReason(releaseSynced.Reason)
			message = releaseSynced.Message
		}
		controller.MarkFalseCondition(releaseBinding, ConditionReady, reason, message)
		return
	}

	// If ResourcesReady is not True, use its reason
	if resourcesReady != nil {
		controller.MarkFalseCondition(releaseBinding, ConditionReady,
			controller.ConditionReason(resourcesReady.Reason), resourcesReady.Message)
	} else {
		controller.MarkFalseCondition(releaseBinding, ConditionReady,
			ReasonResourcesProgressing, "Resources are being evaluated")
	}
}

// categorizeResource determines the category of a resource based on its GVK and workload type.
// nolint:gocyclo
func categorizeResource(gvk schema.GroupVersionKind, workloadType WorkloadType) ResourceCategory {
	// Check if this is the primary workload based on ComponentType
	if isPrimaryWorkload(gvk, workloadType) {
		return CategoryPrimaryWorkload
	}

	// Categorize based on GVK
	switch {
	// Workload resources (if not primary, these are secondary workloads)
	case gvk.Group == appsAPIGroup && (gvk.Kind == "Deployment" || gvk.Kind == "StatefulSet" || gvk.Kind == "DaemonSet"):
		return CategoryPrimaryWorkload
	case gvk.Group == batchAPIGroup && (gvk.Kind == "Job" || gvk.Kind == "CronJob"):
		return CategoryPrimaryWorkload
	case gvk.Group == "" && gvk.Kind == "Pod":
		return CategoryOperational

	// Supporting infrastructure
	case gvk.Group == "" && gvk.Kind == "Service":
		return CategorySupporting
	case gvk.Group == "" && gvk.Kind == "PersistentVolumeClaim":
		return CategorySupporting
	case gvk.Group == "" && gvk.Kind == "ServiceAccount":
		return CategoryNoStatus // ServiceAccounts don't have meaningful status

	// Operational resources (routing, scaling, policies)
	case gvk.Group == "autoscaling" && gvk.Kind == "HorizontalPodAutoscaler":
		return CategoryOperational
	case gvk.Group == "gateway.networking.k8s.io" && gvk.Kind == "HTTPRoute":
		return CategoryOperational
	case gvk.Group == "gateway.networking.k8s.io" && gvk.Kind == "Gateway":
		return CategoryOperational
	case gvk.Group == "networking.k8s.io" && gvk.Kind == "Ingress":
		return CategoryOperational

	// Resources without meaningful status semantics
	case gvk.Group == "" && gvk.Kind == "ConfigMap":
		return CategoryNoStatus
	case gvk.Group == "" && gvk.Kind == "Secret":
		return CategoryNoStatus
	case gvk.Group == "policy" && gvk.Kind == "NetworkPolicy":
		return CategoryNoStatus
	case gvk.Group == "cilium.io" && gvk.Kind == "CiliumNetworkPolicy":
		return CategoryNoStatus
	case gvk.Group == "gateway.envoyproxy.io" && gvk.Kind == "SecurityPolicy":
		return CategoryNoStatus
	case gvk.Group == "rbac.authorization.k8s.io":
		return CategoryNoStatus // RBAC resources don't have meaningful status

	// Unknown resources - treat as operational and let status determine impact
	default:
		return CategoryOperational
	}
}

// isPrimaryWorkload checks if the given GVK matches the expected primary workload for the ComponentType.
func isPrimaryWorkload(gvk schema.GroupVersionKind, workloadType WorkloadType) bool {
	switch workloadType {
	case WorkloadTypeDeployment:
		return gvk.Group == appsAPIGroup && gvk.Kind == "Deployment"
	case WorkloadTypeStatefulSet:
		return gvk.Group == appsAPIGroup && gvk.Kind == "StatefulSet"
	case WorkloadTypeCronJob:
		return gvk.Group == batchAPIGroup && gvk.Kind == "CronJob"
	case WorkloadTypeJob:
		return gvk.Group == batchAPIGroup && gvk.Kind == "Job"
	default:
		return false
	}
}

// aggregateResourceStatus counts resources by their health status.
func aggregateResourceStatus(resources []openchoreov1alpha1.ResourceStatus) ResourceStatusSummary {
	summary := ResourceStatusSummary{
		Total: len(resources),
	}

	for i := range resources {
		switch resources[i].HealthStatus {
		case openchoreov1alpha1.HealthStatusHealthy:
			summary.Healthy++
		case openchoreov1alpha1.HealthStatusProgressing:
			summary.Progressing++
		case openchoreov1alpha1.HealthStatusDegraded:
			summary.Degraded++
		case openchoreov1alpha1.HealthStatusSuspended:
			summary.Suspended++
		case openchoreov1alpha1.HealthStatusUnknown:
			summary.Unknown++
		}
	}

	return summary
}

// evaluateDeploymentStatus evaluates status for Deployment workload type.
// Focuses on the primary Deployment resource and only fails if critical resources are degraded.
func evaluateDeploymentStatus(resources []openchoreov1alpha1.ResourceStatus, workloadType WorkloadType) (ready bool, reason, message string) {
	// Separate resources by category
	var primaryWorkload *openchoreov1alpha1.ResourceStatus
	var supportingResources []openchoreov1alpha1.ResourceStatus
	var operationalResources []openchoreov1alpha1.ResourceStatus

	for i := range resources {
		res := &resources[i]
		gvk := schema.GroupVersionKind{
			Group:   res.Group,
			Version: res.Version,
			Kind:    res.Kind,
		}

		category := categorizeResource(gvk, workloadType)

		switch category {
		case CategoryPrimaryWorkload:
			if isPrimaryWorkload(gvk, workloadType) {
				primaryWorkload = res
			}
		case CategorySupporting:
			supportingResources = append(supportingResources, *res)
		case CategoryOperational:
			operationalResources = append(operationalResources, *res)
		case CategoryNoStatus:
			// Skip resources without status semantics (ConfigMaps, Secrets, etc.)
			continue
		}
	}

	// PRIMARY WORKLOAD is the key indicator
	if primaryWorkload == nil {
		return false, string(ReasonResourcesProgressing), "Primary workload not found"
	}

	// Check primary workload status
	switch primaryWorkload.HealthStatus {
	case openchoreov1alpha1.HealthStatusDegraded:
		return false, string(ReasonResourcesDegraded),
			fmt.Sprintf("Primary workload %s/%s is degraded", primaryWorkload.Kind, primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusProgressing:
		return false, string(ReasonResourcesProgressing),
			fmt.Sprintf("Primary workload %s/%s is progressing", primaryWorkload.Kind, primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusUnknown:
		return false, string(ReasonResourcesUnknown),
			fmt.Sprintf("Primary workload %s/%s status unknown", primaryWorkload.Kind, primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusSuspended:
		// Suspended deployment is considered ready (scaled to 0)
		return true, string(ReasonReadyWithSuspendedResources),
			fmt.Sprintf("Primary workload %s is suspended (scaled to 0)", primaryWorkload.Kind)

	case openchoreov1alpha1.HealthStatusHealthy:
		// Primary is healthy - check if any other critical resources are degraded
		for i := range supportingResources {
			if supportingResources[i].HealthStatus == openchoreov1alpha1.HealthStatusDegraded {
				return false, string(ReasonResourcesDegraded),
					fmt.Sprintf("Supporting resource %s/%s is degraded", supportingResources[i].Kind, supportingResources[i].Name)
			}
		}

		for i := range operationalResources {
			if operationalResources[i].HealthStatus == openchoreov1alpha1.HealthStatusDegraded {
				return false, string(ReasonResourcesDegraded),
					fmt.Sprintf("Operational resource %s/%s is degraded", operationalResources[i].Kind, operationalResources[i].Name)
			}
		}

		// All good!
		return true, string(ReasonReady),
			fmt.Sprintf("Primary workload %s and all supporting resources are ready", primaryWorkload.Kind)
	}

	return false, string(ReasonResourcesUnknown), "Unable to determine status"
}

// evaluateStatefulSetStatus evaluates status for StatefulSet workload type.
// Similar to Deployment - all replicas must be ready in order.
func evaluateStatefulSetStatus(resources []openchoreov1alpha1.ResourceStatus, workloadType WorkloadType) (ready bool, reason, message string) {
	// StatefulSet status logic is similar to Deployment
	// All replicas must be ready and in order
	return evaluateDeploymentStatus(resources, workloadType)
}

// evaluateCronJobStatus evaluates status for CronJob workload type.
// CronJobs have different ready criteria - Suspended and Progressing are valid states.
func evaluateCronJobStatus(resources []openchoreov1alpha1.ResourceStatus, workloadType WorkloadType) (ready bool, reason, message string) {
	// Find primary CronJob
	var primaryWorkload *openchoreov1alpha1.ResourceStatus

	for i := range resources {
		res := &resources[i]
		gvk := schema.GroupVersionKind{
			Group:   res.Group,
			Version: res.Version,
			Kind:    res.Kind,
		}

		if isPrimaryWorkload(gvk, workloadType) {
			primaryWorkload = res
			break
		}
	}

	if primaryWorkload == nil {
		return false, string(ReasonResourcesProgressing), "Primary CronJob not found"
	}

	// Check primary CronJob status
	switch primaryWorkload.HealthStatus {
	case openchoreov1alpha1.HealthStatusDegraded:
		return false, string(ReasonResourcesDegraded),
			fmt.Sprintf("CronJob %s is degraded", primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusUnknown:
		return false, string(ReasonResourcesUnknown),
			fmt.Sprintf("CronJob %s status unknown", primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusSuspended:
		// Suspended CronJob is a valid ready state
		return true, string(ReasonCronJobSuspended),
			fmt.Sprintf("CronJob %s is suspended", primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusProgressing:
		// Progressing means a job execution is running - this is valid for CronJobs
		return true, string(ReasonCronJobScheduled),
			fmt.Sprintf("CronJob %s is scheduled, job execution running", primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusHealthy:
		// Healthy CronJob (scheduled and ready)
		return true, string(ReasonCronJobScheduled),
			fmt.Sprintf("CronJob %s is scheduled and ready", primaryWorkload.Name)
	}

	return false, string(ReasonResourcesUnknown), "Unable to determine CronJob status"
}

// evaluateJobStatus evaluates status for Job workload type.
// Jobs must complete (Healthy) to be considered ready. Progressing is expected until completion.
func evaluateJobStatus(resources []openchoreov1alpha1.ResourceStatus, workloadType WorkloadType) (ready bool, reason, message string) {
	// Find primary Job
	var primaryWorkload *openchoreov1alpha1.ResourceStatus

	for i := range resources {
		res := &resources[i]
		gvk := schema.GroupVersionKind{
			Group:   res.Group,
			Version: res.Version,
			Kind:    res.Kind,
		}

		if isPrimaryWorkload(gvk, workloadType) {
			primaryWorkload = res
			break
		}
	}

	if primaryWorkload == nil {
		return false, string(ReasonResourcesProgressing), "Primary Job not found"
	}

	// Check primary Job status
	switch primaryWorkload.HealthStatus {
	case openchoreov1alpha1.HealthStatusDegraded:
		return false, string(ReasonJobFailed),
			fmt.Sprintf("Job %s failed", primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusUnknown:
		return false, string(ReasonResourcesUnknown),
			fmt.Sprintf("Job %s status unknown", primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusProgressing:
		// Progressing is expected for Jobs until completion
		return false, string(ReasonJobRunning),
			fmt.Sprintf("Job %s is running", primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusSuspended:
		// Suspended Job - not ready yet
		return false, string(ReasonJobRunning),
			fmt.Sprintf("Job %s is suspended", primaryWorkload.Name)

	case openchoreov1alpha1.HealthStatusHealthy:
		// Healthy means Job completed successfully
		return true, string(ReasonJobCompleted),
			fmt.Sprintf("Job %s completed successfully", primaryWorkload.Name)
	}

	return false, string(ReasonResourcesUnknown), "Unable to determine Job status"
}

// evaluateGenericStatus evaluates status for unknown/generic workload types.
// Uses simple status check - ready if all resources are healthy or suspended.
func evaluateGenericStatus(resources []openchoreov1alpha1.ResourceStatus) (ready bool, reason, message string) {
	summary := aggregateResourceStatus(resources)

	if summary.Total == 0 {
		return true, string(ReasonReady), "No resources to track"
	}

	// Any degraded resources = not ready
	if summary.Degraded > 0 {
		return false, string(ReasonResourcesDegraded),
			fmt.Sprintf("%d/%d resources degraded", summary.Degraded, summary.Total)
	}

	// Any progressing resources = not ready
	if summary.Progressing > 0 {
		return false, string(ReasonResourcesProgressing),
			fmt.Sprintf("%d/%d resources progressing", summary.Progressing, summary.Total)
	}

	// Any unknown resources = not ready
	if summary.Unknown > 0 {
		return false, string(ReasonResourcesUnknown),
			fmt.Sprintf("%d/%d resources status unknown", summary.Unknown, summary.Total)
	}

	// All resources are either healthy or suspended
	if summary.Healthy+summary.Suspended == summary.Total {
		return true, string(ReasonReady),
			fmt.Sprintf("All %d resources ready", summary.Total)
	}

	return false, string(ReasonResourcesUnknown), "Unable to determine resource status"
}
