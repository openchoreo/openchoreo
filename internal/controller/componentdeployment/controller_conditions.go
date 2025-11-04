// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentdeployment

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReady indicates that the ComponentDeployment has successfully created/updated
	// the Release and the deployment is ready.
	ConditionReady controller.ConditionType = "Ready"
)

// Constants for condition reasons

const (
	// Success states (Status=True)

	// ReasonReleaseReady indicates the Release is successfully deployed and ready
	ReasonReleaseReady controller.ConditionReason = "ReleaseReady"

	// Configuration issues (Status=False)

	// ReasonComponentEnvSnapshotNotFound indicates the referenced ComponentEnvSnapshot doesn't exist
	ReasonComponentEnvSnapshotNotFound controller.ConditionReason = "ComponentEnvSnapshotNotFound"
	// ReasonEnvironmentNotFound indicates the referenced Environment doesn't exist
	ReasonEnvironmentNotFound controller.ConditionReason = "EnvironmentNotFound"
	// ReasonDataPlaneNotFound indicates the referenced DataPlane doesn't exist
	ReasonDataPlaneNotFound controller.ConditionReason = "DataPlaneNotFound"
	// ReasonInvalidConfiguration indicates the ComponentDeployment configuration is invalid
	ReasonInvalidConfiguration controller.ConditionReason = "InvalidConfiguration"
	// ReasonInvalidSnapshotConfiguration indicates the ComponentEnvSnapshot has invalid configuration
	ReasonInvalidSnapshotConfiguration controller.ConditionReason = "InvalidSnapshotConfiguration"

	// Rendering issues (Status=False)

	// ReasonRenderingFailed indicates failure to render resources from ComponentEnvSnapshot
	ReasonRenderingFailed controller.ConditionReason = "RenderingFailed"
	// ReasonValidationFailed indicates rendered resources failed validation
	ReasonValidationFailed controller.ConditionReason = "ValidationFailed"

	// Release management issues (Status=False)

	// ReasonReleaseCreationFailed indicates failure to create the Release
	ReasonReleaseCreationFailed controller.ConditionReason = "ReleaseCreationFailed"
	// ReasonReleaseUpdateFailed indicates failure to update the Release
	ReasonReleaseUpdateFailed controller.ConditionReason = "ReleaseUpdateFailed"
	// ReasonReleaseOwnershipConflict indicates Release exists but not owned by this ComponentDeployment
	ReasonReleaseOwnershipConflict controller.ConditionReason = "ReleaseOwnershipConflict"

	// Resource health issues (Status=False/Unknown)

	// ReasonResourcesProgressing indicates resources are being deployed or updated
	ReasonResourcesProgressing controller.ConditionReason = "ResourcesProgressing"
	// ReasonResourcesDegraded indicates one or more resources are in error state
	ReasonResourcesDegraded controller.ConditionReason = "ResourcesDegraded"
)
