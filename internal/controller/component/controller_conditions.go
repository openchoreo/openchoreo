// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReady indicates that the Component has successfully created/updated
	// the ComponentEnvSnapshot and is ready for deployment.
	ConditionReady controller.ConditionType = "Ready"
)

// Constants for condition reasons

const (
	// Success states (Status=True)

	// ReasonSnapshotReady indicates the ComponentEnvSnapshot is successfully created/updated
	ReasonSnapshotReady controller.ConditionReason = "SnapshotReady"

	// Configuration issues (Status=False)

	// ReasonWorkloadNotFound indicates the referenced Workload doesn't exist
	ReasonWorkloadNotFound controller.ConditionReason = "WorkloadNotFound"
	// ReasonComponentTypeNotFound indicates the referenced ComponentType doesn't exist
	ReasonComponentTypeNotFound controller.ConditionReason = "ComponentTypeNotFound"
	// ReasonAddonNotFound indicates one or more referenced Addons don't exist
	ReasonAddonNotFound controller.ConditionReason = "AddonNotFound"
	// ReasonProjectNotFound indicates the referenced Project doesn't exist
	ReasonProjectNotFound controller.ConditionReason = "ProjectNotFound"
	// ReasonDeploymentPipelineNotFound indicates the deployment pipeline is not found
	ReasonDeploymentPipelineNotFound controller.ConditionReason = "DeploymentPipelineNotFound"
	// ReasonInvalidConfiguration indicates the Component configuration is invalid
	ReasonInvalidConfiguration controller.ConditionReason = "InvalidConfiguration"

	// Snapshot management issues (Status=False)

	// ReasonSnapshotCreationFailed indicates failure to create the ComponentEnvSnapshot
	ReasonSnapshotCreationFailed controller.ConditionReason = "SnapshotCreationFailed"
	// ReasonSnapshotUpdateFailed indicates failure to update the ComponentEnvSnapshot
	ReasonSnapshotUpdateFailed controller.ConditionReason = "SnapshotUpdateFailed"
)
