// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowrun

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	ConditionWorkflowRunning   controller.ConditionType = "WorkflowRunning"
	ConditionWorkflowFailed    controller.ConditionType = "WorkflowFailed"
	ConditionWorkflowSucceeded controller.ConditionType = "WorkflowSucceeded"
	ConditionWorkflowCompleted controller.ConditionType = "WorkflowCompleted"
	ConditionWorkloadUpdated   controller.ConditionType = "WorkloadUpdated"
)

const (
	ReasonWorkflowPending            controller.ConditionReason = "WorkflowPending"
	ReasonWorkflowRunning            controller.ConditionReason = "WorkflowRunning"
	ReasonWorkflowSucceeded          controller.ConditionReason = "WorkflowSucceeded"
	ReasonWorkflowFailed             controller.ConditionReason = "WorkflowFailed"
	ReasonWorkloadUpdated            controller.ConditionReason = "WorkloadUpdated"
	ReasonWorkloadUpdateFailed       controller.ConditionReason = "WorkloadUpdateFailed"
	ReasonSecretResolutionError      controller.ConditionReason = "SecretResolutionError"
	ReasonWorkflowNotAllowed         controller.ConditionReason = "WorkflowNotAllowed"
	ReasonComponentWorkflowNotFound  controller.ConditionReason = "ComponentWorkflowNotFound"
	ReasonComponentTypeNotFound      controller.ConditionReason = "ComponentTypeNotFound"
	ReasonComponentNotFound          controller.ConditionReason = "ComponentNotFound"
	ReasonBuildPlaneNotFound         controller.ConditionReason = "BuildPlaneNotFound"
	ReasonBuildPlaneResolutionFailed controller.ConditionReason = "BuildPlaneResolutionFailed"
)

func setWorkflowPendingCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowPending),
		Message:            "Workflow has not completed yet",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setWorkflowRunningCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow is running",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setWorkflowSucceededCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow running has completed",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowSucceeded),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowSucceeded),
		Message:            "Workflow completed successfully",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowSucceeded),
		Message:            "Workflow has completed successfully",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setWorkflowFailedCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow running has completed",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowFailed),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow execution failed",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow has completed with failure",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setBuildPlaneNotFoundCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonBuildPlaneNotFound),
		Message:            "No build plane found for the project associated with this workflow run",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setBuildPlaneResolutionFailedCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun, err error) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonBuildPlaneResolutionFailed),
		Message:            "Failed to resolve build plane: " + err.Error(),
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setWorkflowNotFoundCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Workflow is not found in the cluster",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow has been deleted from the cluster",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setWorkloadUpdatedCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkloadUpdated),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkloadUpdated),
		Message:            "Workload CR created/updated successfully",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setWorkloadUpdateFailedCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkloadUpdated),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkloadUpdateFailed),
		Message:            "Failed to create/update workload CR",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setSecretResolutionFailedCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun, message string) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowFailed),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonSecretResolutionError),
		Message:            message,
		ObservedGeneration: componentWorkflowRun.Generation,
	})
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonSecretResolutionError),
		Message:            "Failed to resolve git secret",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func setWorkflowNotAllowedCondition(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun, message string) {
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowFailed),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowNotAllowed),
		Message:            message,
		ObservedGeneration: componentWorkflowRun.Generation,
	})
	meta.SetStatusCondition(&componentWorkflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowNotAllowed),
		Message:            "Workflow is not allowed by ComponentType",
		ObservedGeneration: componentWorkflowRun.Generation,
	})
}

func isWorkflowInitiated(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) bool {
	return meta.FindStatusCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowCompleted)) != nil
}

func isWorkflowCompleted(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) bool {
	return meta.IsStatusConditionTrue(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowCompleted))
}

func isWorkflowSucceeded(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) bool {
	return meta.IsStatusConditionTrue(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowSucceeded))
}

func isWorkloadUpdated(componentWorkflowRun *openchoreov1alpha1.ComponentWorkflowRun) bool {
	return meta.IsStatusConditionTrue(componentWorkflowRun.Status.Conditions, string(ConditionWorkloadUpdated))
}
