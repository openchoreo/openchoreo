// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Build condition types
const (
	ConditionBuildInitiated  controller.ConditionType = "BuildInitiated"
	ConditionBuildTriggered  controller.ConditionType = "BuildTriggered"
	ConditionBuildCompleted  controller.ConditionType = "BuildCompleted"
	ConditionWorkloadUpdated controller.ConditionType = "WorkloadUpdated"
)

// Build condition reasons
const (
	ReasonBuildInitiated       controller.ConditionReason = "BuildInitiated"
	ReasonBuildTriggered       controller.ConditionReason = "BuildTriggered"
	ReasonBuildCompleted       controller.ConditionReason = "BuildCompleted"
	ReasonBuildFailed          controller.ConditionReason = "BuildFailed"
	ReasonBuildInProgress      controller.ConditionReason = "BuildInProgress"
	ReasonWorkloadUpdated      controller.ConditionReason = "WorkloadUpdated"
	ReasonWorkloadUpdateFailed controller.ConditionReason = "WorkloadUpdateFailed"
)

// NewBuildInitiatedCondition creates a new BuildInitiated condition
func NewBuildInitiatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildInitiated),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonBuildInitiated),
		Message:            "Build initialization started",
		ObservedGeneration: generation,
	}
}

// NewBuildTriggeredCondition creates a new BuildTriggered condition
func NewBuildTriggeredCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildTriggered),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonBuildTriggered),
		Message:            "Build has been triggered",
		ObservedGeneration: generation,
	}
}

// NewBuildCompletedCondition creates a new BuildCompleted condition
func NewBuildCompletedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonBuildCompleted),
		Message:            "Build completed successfully",
		ObservedGeneration: generation,
	}
}

// NewBuildFailedCondition creates a new BuildFailed condition
func NewBuildFailedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonBuildFailed),
		Message:            "Build failed",
		ObservedGeneration: generation,
	}
}

// NewBuildInProgressCondition creates a new BuildInProgress condition
func NewBuildInProgressCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonBuildInProgress),
		Message:            "Build is in progress",
		ObservedGeneration: generation,
	}
}

// NewWorkloadUpdatedCondition creates a new WorkloadUpdated condition
func NewWorkloadUpdatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionWorkloadUpdated),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkloadUpdated),
		Message:            "Workload updated successfully",
		ObservedGeneration: generation,
	}
}

// NewWorkloadUpdateFailedCondition creates a new WorkloadUpdateFailed condition
func NewWorkloadUpdateFailedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionWorkloadUpdated),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkloadUpdateFailed),
		Message:            "Failed to update workload with the new built image",
		ObservedGeneration: generation,
	}
}

// setBuildInitiatedCondition sets the BuildInitiated condition
func setBuildInitiatedCondition(build *openchoreov1alpha1.Build) {
	meta.SetStatusCondition(&build.Status.Conditions, NewBuildInitiatedCondition(build.Generation))
}

// setBuildTriggeredCondition sets the BuildTriggered condition
func setBuildTriggeredCondition(build *openchoreov1alpha1.Build) {
	meta.SetStatusCondition(&build.Status.Conditions, NewBuildTriggeredCondition(build.Generation))
}

// setBuildCompletedCondition sets the BuildCompleted condition
func setBuildCompletedCondition(build *openchoreov1alpha1.Build, message string) {
	condition := NewBuildCompletedCondition(build.Generation)
	if message != "" {
		condition.Message = message
	}
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}

// setBuildFailedCondition sets the BuildFailed condition
func setBuildFailedCondition(build *openchoreov1alpha1.Build, reason controller.ConditionReason, message string) {
	condition := NewBuildFailedCondition(build.Generation)
	if reason != "" {
		condition.Reason = string(reason)
	}
	if message != "" {
		condition.Message = message
	}
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}

// setBuildInProgressCondition sets the BuildInProgress condition
func setBuildInProgressCondition(build *openchoreov1alpha1.Build) {
	meta.SetStatusCondition(&build.Status.Conditions, NewBuildInProgressCondition(build.Generation))
}

// isBuildInitiated checks if the build is initiated
func isBuildInitiated(build *openchoreov1alpha1.Build) bool {
	return meta.IsStatusConditionTrue(build.Status.Conditions, string(ConditionBuildInitiated))
}

// isBuildCompleted returns true when the Build has **reached a terminal state**
// (either Succeeded or Failed).  Any “in-progress” or unknown condition returns false.
func isBuildCompleted(build *openchoreov1alpha1.Build) bool {
	cond := meta.FindStatusCondition(build.Status.Conditions, string(ConditionWorkloadUpdated))
	if cond == nil {
		return false
	}

	if cond.Reason == string(ReasonWorkloadUpdated) {
		return cond.Status == metav1.ConditionTrue
	}

	return false
}

func isBuildWorkflowSucceeded(build *openchoreov1alpha1.Build) bool {
	cond := meta.FindStatusCondition(build.Status.Conditions, string(ConditionBuildCompleted))
	if cond == nil {
		return false
	}

	if cond.Reason == string(ReasonBuildCompleted) {
		return cond.Status == metav1.ConditionTrue
	}
	return false
}

// shouldIgnoreReconcile checks whether the reconcile loop should be continued
func shouldIgnoreReconcile(build *openchoreov1alpha1.Build) bool {
	// Skip reconciliation if build is already completed (success or failure)
	if isBuildCompleted(build) {
		return true
	}
	return false
}
