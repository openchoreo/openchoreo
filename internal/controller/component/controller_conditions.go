// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReady indicates that the Component has been successfully reconciled.
	// When autoDeploy is enabled, this means ComponentRelease and ReleaseBinding are created/updated.
	// When autoDeploy is disabled, this means the Component has been validated.
	ConditionReady controller.ConditionType = "Ready"

	// ConditionFinalizing indicates that the Component is being finalized (deleted).
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

// Constants for condition reasons

const (
	// Success states (Status=True)

	// ReasonReconciled indicates the Component has been successfully validated
	// Used when autoDeploy is disabled - only validation is performed
	ReasonReconciled controller.ConditionReason = "Reconciled"

	// ReasonComponentReleaseReady indicates ComponentRelease and ReleaseBinding are successfully created/updated
	// Used when autoDeploy is enabled
	ReasonComponentReleaseReady controller.ConditionReason = "ComponentReleaseReady"

	// Configuration issues (Status=False)

	// ReasonWorkloadNotFound indicates the referenced Workload doesn't exist
	ReasonWorkloadNotFound controller.ConditionReason = "WorkloadNotFound"
	// ReasonComponentTypeNotFound indicates the referenced ComponentType doesn't exist
	ReasonComponentTypeNotFound controller.ConditionReason = "ComponentTypeNotFound"
	// ReasonTraitNotFound indicates one or more referenced Traits don't exist
	ReasonTraitNotFound controller.ConditionReason = "TraitNotFound"
	// ReasonProjectNotFound indicates the referenced Project doesn't exist
	ReasonProjectNotFound controller.ConditionReason = "ProjectNotFound"
	// ReasonDeploymentPipelineNotFound indicates the deployment pipeline is not found
	ReasonDeploymentPipelineNotFound controller.ConditionReason = "DeploymentPipelineNotFound"
	// ReasonInvalidConfiguration indicates the Component configuration is invalid
	ReasonInvalidConfiguration controller.ConditionReason = "InvalidConfiguration"
	// ReasonComponentWorkflowNotAllowed indicates the referenced ComponentWorkflow is not in allowedWorkflows
	ReasonComponentWorkflowNotAllowed controller.ConditionReason = "ComponentWorkflowNotAllowed"
	// ReasonComponentWorkflowNotFound indicates the referenced ComponentWorkflow doesn't exist
	ReasonComponentWorkflowNotFound controller.ConditionReason = "ComponentWorkflowNotFound"

	// AutoDeploy issues (Status=False)

	// ReasonAutoDeployFailed indicates failure to handle autoDeploy (ComponentRelease/ReleaseBinding creation)
	ReasonAutoDeployFailed controller.ConditionReason = "AutoDeployFailed"

	// ReasonFinalizing indicates the Component is being finalized
	ReasonFinalizing controller.ConditionReason = "Finalizing"
)

// NewComponentFinalizingCondition creates a condition indicating the Component is being finalized.
func NewComponentFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonFinalizing,
		"Component is finalizing",
		generation,
	)
}
