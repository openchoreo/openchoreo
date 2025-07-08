// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildv2

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Build condition types
const (
	ConditionBuildInitiated controller.ConditionType = "BuildInitiated"
	ConditionBuildTriggered controller.ConditionType = "BuildTriggered"
	ConditionBuildCompleted controller.ConditionType = "BuildCompleted"
)

// Build condition reasons
const (
	ReasonBuildInitiated          controller.ConditionReason = "BuildInitiated"
	ReasonBuildTriggered          controller.ConditionReason = "BuildTriggered"
	ReasonBuildCompleted          controller.ConditionReason = "BuildCompleted"
	ReasonBuildFailed             controller.ConditionReason = "BuildFailed"
	ReasonBuildInProgress         controller.ConditionReason = "BuildInProgress"
	ReasonWorkflowCreated         controller.ConditionReason = "WorkflowCreated"
	ReasonWorkflowCreationFailed  controller.ConditionReason = "WorkflowCreationFailed"
	ReasonNamespaceCreationFailed controller.ConditionReason = "NamespaceCreationFailed"
	ReasonRBACCreationFailed      controller.ConditionReason = "RBACCreationFailed"
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

// NewWorkflowCreatedCondition creates a new WorkflowCreated condition
func NewWorkflowCreatedCondition(generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildTriggered),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowCreated),
		Message:            "Build workflow created successfully",
		ObservedGeneration: generation,
	}
}

// NewWorkflowCreationFailedCondition creates a new WorkflowCreationFailed condition
func NewWorkflowCreationFailedCondition(generation int64, message string) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowCreationFailed),
		Message:            message,
		ObservedGeneration: generation,
	}
}

// NewNamespaceCreationFailedCondition creates a new NamespaceCreationFailed condition
func NewNamespaceCreationFailedCondition(generation int64, message string) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonNamespaceCreationFailed),
		Message:            message,
		ObservedGeneration: generation,
	}
}

// NewRBACCreationFailedCondition creates a new RBACCreationFailed condition
func NewRBACCreationFailedCondition(generation int64, message string) metav1.Condition {
	return metav1.Condition{
		Type:               string(ConditionBuildCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonRBACCreationFailed),
		Message:            message,
		ObservedGeneration: generation,
	}
}

// setBuildInitiatedCondition sets the BuildInitiated condition
func setBuildInitiatedCondition(build *choreov1.BuildV2) {
	meta.SetStatusCondition(&build.Status.Conditions, NewBuildInitiatedCondition(build.Generation))
}

// setBuildTriggeredCondition sets the BuildTriggered condition
func setBuildTriggeredCondition(build *choreov1.BuildV2) {
	meta.SetStatusCondition(&build.Status.Conditions, NewBuildTriggeredCondition(build.Generation))
}

// setBuildCompletedCondition sets the BuildCompleted condition
func setBuildCompletedCondition(build *choreov1.BuildV2, message string) {
	condition := NewBuildCompletedCondition(build.Generation)
	if message != "" {
		condition.Message = message
	}
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}

// setBuildFailedCondition sets the BuildFailed condition
func setBuildFailedCondition(build *choreov1.BuildV2, reason controller.ConditionReason, message string) {
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
func setBuildInProgressCondition(build *choreov1.BuildV2) {
	meta.SetStatusCondition(&build.Status.Conditions, NewBuildInProgressCondition(build.Generation))
}

// isBuildInitiated checks if the build is initiated
func isBuildInitiated(build *choreov1.BuildV2) bool {
	return meta.IsStatusConditionTrue(build.Status.Conditions, string(ConditionBuildInitiated))
}

// isBuildCompleted returns true when the Build has **reached a terminal state**
// (either Succeeded or Failed).  Any “in-progress” or unknown condition returns false.
func isBuildCompleted(build *choreov1.BuildV2) bool {
	cond := meta.FindStatusCondition(build.Status.Conditions, string(ConditionBuildCompleted))
	if cond == nil {
		return false
	}

	switch cond.Reason {
	case string(ReasonBuildCompleted):
		// success → the controller set Status=True + Completed reason
		return cond.Status == metav1.ConditionTrue

	case string(ReasonBuildFailed):
		// failure → the controller set Status=False + Failed reason
		return cond.Status == metav1.ConditionFalse
	}

	// “InProgress” or any other reason → still running
	return false
}

// isBuildTriggered checks if the build is triggered
func isBuildTriggered(build *choreov1.BuildV2) bool {
	return meta.IsStatusConditionTrue(build.Status.Conditions, string(ConditionBuildTriggered))
}

// shouldIgnoreReconcile checks whether the reconcile loop should be continued
func shouldIgnoreReconcile(build *choreov1.BuildV2) bool {
	// Skip reconciliation if build is already completed (success or failure)
	if isBuildCompleted(build) {
		return true
	}
	return false
}
