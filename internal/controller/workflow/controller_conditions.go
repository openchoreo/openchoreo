// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

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
	ReasonWorkflowPending      controller.ConditionReason = "WorkflowPending"
	ReasonWorkflowRunning      controller.ConditionReason = "WorkflowRunning"
	ReasonWorkflowSucceeded    controller.ConditionReason = "WorkflowSucceeded"
	ReasonWorkflowFailed       controller.ConditionReason = "WorkflowFailed"
	ReasonWorkloadUpdated      controller.ConditionReason = "WorkloadUpdated"
	ReasonWorkloadUpdateFailed controller.ConditionReason = "WorkloadUpdateFailed"
)

func setWorkflowPendingCondition(workflow *openchoreov1alpha1.Workflow) {
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowPending),
		Message:            "Workflow has not completed yet",
		ObservedGeneration: workflow.Generation,
	})
}

func setWorkflowRunningCondition(workflow *openchoreov1alpha1.Workflow) {
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow is running",
		ObservedGeneration: workflow.Generation,
	})
}

func setWorkflowSucceededCondition(workflow *openchoreov1alpha1.Workflow) {
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow running has completed",
		ObservedGeneration: workflow.Generation,
	})
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowSucceeded),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowSucceeded),
		Message:            "Workflow completed successfully",
		ObservedGeneration: workflow.Generation,
	})
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowSucceeded),
		Message:            "Workflow has completed successfully",
		ObservedGeneration: workflow.Generation,
	})
}

func setWorkflowFailedCondition(workflow *openchoreov1alpha1.Workflow) {
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Argo Workflow running has completed",
		ObservedGeneration: workflow.Generation,
	})
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowFailed),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow execution failed",
		ObservedGeneration: workflow.Generation,
	})
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow has completed with failure",
		ObservedGeneration: workflow.Generation,
	})
}

func setWorkflowNotFoundCondition(workflow *openchoreov1alpha1.Workflow) {
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowRunning),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkflowRunning),
		Message:            "Workflow is not found in the cluster",
		ObservedGeneration: workflow.Generation,
	})
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkflowCompleted),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkflowFailed),
		Message:            "Workflow has been deleted from the cluster",
		ObservedGeneration: workflow.Generation,
	})
}

func setWorkloadUpdatedCondition(workflow *openchoreov1alpha1.Workflow) {
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkloadUpdated),
		Status:             metav1.ConditionTrue,
		Reason:             string(ReasonWorkloadUpdated),
		Message:            "Workload CR created/updated successfully",
		ObservedGeneration: workflow.Generation,
	})
}

func setWorkloadUpdateFailedCondition(workflow *openchoreov1alpha1.Workflow) {
	meta.SetStatusCondition(&workflow.Status.Conditions, metav1.Condition{
		Type:               string(ConditionWorkloadUpdated),
		Status:             metav1.ConditionFalse,
		Reason:             string(ReasonWorkloadUpdateFailed),
		Message:            "Failed to create/update workload CR",
		ObservedGeneration: workflow.Generation,
	})
}

func isWorkflowInitiated(workflow *openchoreov1alpha1.Workflow) bool {
	return meta.FindStatusCondition(workflow.Status.Conditions, string(ConditionWorkflowCompleted)) != nil
}

func isWorkflowCompleted(workflow *openchoreov1alpha1.Workflow) bool {
	return meta.IsStatusConditionTrue(workflow.Status.Conditions, string(ConditionWorkflowCompleted))
}

func isWorkflowSucceeded(workflow *openchoreov1alpha1.Workflow) bool {
	return meta.IsStatusConditionTrue(workflow.Status.Conditions, string(ConditionWorkflowSucceeded))
}

func isWorkloadUpdated(workflow *openchoreov1alpha1.Workflow) bool {
	return meta.IsStatusConditionTrue(workflow.Status.Conditions, string(ConditionWorkloadUpdated))
}
