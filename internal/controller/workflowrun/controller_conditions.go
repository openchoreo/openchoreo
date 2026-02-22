// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

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
)

const (
	ReasonWorkflowPending            controller.ConditionReason = "WorkflowPending"
	ReasonWorkflowRunning            controller.ConditionReason = "WorkflowRunning"
	ReasonWorkflowSucceeded          controller.ConditionReason = "WorkflowSucceeded"
	ReasonWorkflowFailed             controller.ConditionReason = "WorkflowFailed"
	ReasonBuildPlaneNotFound         controller.ConditionReason = "BuildPlaneNotFound"
	ReasonBuildPlaneResolutionFailed controller.ConditionReason = "BuildPlaneResolutionFailed"
)

func setWorkflowPendingCondition(workflowRun *openchoreov1alpha1.WorkflowRun) {
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowPending),
		Message:            "Workflow has not completed yet",
		ObservedGeneration: workflowRun.Generation,
	})
}

func setWorkflowRunningCondition(workflowRun *openchoreov1alpha1.WorkflowRun) {
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow is running",
		ObservedGeneration: workflowRun.Generation,
	})
}

func setWorkflowSucceededCondition(workflowRun *openchoreov1alpha1.WorkflowRun) {
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow running has completed",
		ObservedGeneration: workflowRun.Generation,
	})
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowSucceeded),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowSucceeded),
		Message:            "Workflow completed successfully",
		ObservedGeneration: workflowRun.Generation,
	})
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowSucceeded),
		Message:            "Workflow has completed successfully",
		ObservedGeneration: workflowRun.Generation,
	})
}

func setWorkflowFailedCondition(workflowRun *openchoreov1alpha1.WorkflowRun) {
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow running has completed",
		ObservedGeneration: workflowRun.Generation,
	})
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowFailed),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow execution failed",
		ObservedGeneration: workflowRun.Generation,
	})
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow has completed with failure",
		ObservedGeneration: workflowRun.Generation,
	})
}

func setBuildPlaneNotFoundCondition(workflowRun *openchoreov1alpha1.WorkflowRun) {
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonBuildPlaneNotFound),
		Message:            "No build plane found for the project associated with this workflow run",
		ObservedGeneration: workflowRun.Generation,
	})
}

func setBuildPlaneResolutionFailedCondition(workflowRun *openchoreov1alpha1.WorkflowRun, err error) {
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonBuildPlaneResolutionFailed),
		Message:            "Failed to resolve build plane: " + err.Error(),
		ObservedGeneration: workflowRun.Generation,
	})
}

func setWorkflowNotFoundCondition(workflowRun *openchoreov1alpha1.WorkflowRun) {
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Workflow is not found in the cluster",
		ObservedGeneration: workflowRun.Generation,
	})
	meta.SetStatusCondition(&workflowRun.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow has been deleted from the cluster",
		ObservedGeneration: workflowRun.Generation,
	})
}

func isWorkflowInitiated(workflowRun *openchoreov1alpha1.WorkflowRun) bool {
	return meta.FindStatusCondition(workflowRun.Status.Conditions, string(ConditionWorkflowCompleted)) != nil
}

func isWorkflowCompleted(workflowRun *openchoreov1alpha1.WorkflowRun) bool {
	return meta.IsStatusConditionTrue(workflowRun.Status.Conditions, string(ConditionWorkflowCompleted))
}

func isWorkflowSucceeded(workflowRun *openchoreov1alpha1.WorkflowRun) bool {
	return meta.IsStatusConditionTrue(workflowRun.Status.Conditions, string(ConditionWorkflowSucceeded))
}
