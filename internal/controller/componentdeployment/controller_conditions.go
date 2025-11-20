// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentdeployment

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReleaseSynced indicates that the Release resource has been successfully created/updated
	// with the rendered resources. This does not indicate that the resources are ready yet.
	ConditionReleaseSynced controller.ConditionType = "ReleaseSynced"

	// ConditionResourcesReady indicates that all resources in the Release are applied and ready.
	// This is evaluated from the Release.Status.Resources health information.
	ConditionResourcesReady controller.ConditionType = "ResourcesReady"
)

// Constants for condition reasons

const (
	// Reasons for ReleaseSynced condition

	// ReasonReleaseCreated indicates the Release was successfully created
	ReasonReleaseCreated controller.ConditionReason = "ReleaseCreated"
	// ReasonReleaseUpdated indicates the Release was successfully updated
	ReasonReleaseUpdated controller.ConditionReason = "ReleaseUpdated"
	// ReasonReleaseSynced indicates the Release is up to date (no changes needed)
	ReasonReleaseSynced controller.ConditionReason = "ReleaseSynced"

	// Reasons for ResourcesReady condition

	// ReasonResourcesReady indicates all resources are deployed and ready
	ReasonResourcesReady controller.ConditionReason = "ResourcesReady"
	// ReasonResourcesProgressing indicates resources are being deployed or updated
	ReasonResourcesProgressing controller.ConditionReason = "ResourcesProgressing"
	// ReasonResourcesDegraded indicates one or more resources are in error state
	ReasonResourcesDegraded controller.ConditionReason = "ResourcesDegraded"

	// Configuration issues (Status=False)

	// ReasonComponentEnvSnapshotNotFound indicates the referenced ComponentEnvSnapshot doesn't exist
	ReasonComponentEnvSnapshotNotFound controller.ConditionReason = "ComponentEnvSnapshotNotFound"
	// ReasonComponentNotFound indicates the referenced Component doesn't exist
	ReasonComponentNotFound controller.ConditionReason = "ComponentNotFound"
	// ReasonProjectNotFound indicates the referenced Project doesn't exist
	ReasonProjectNotFound controller.ConditionReason = "ProjectNotFound"
	// ReasonEnvironmentNotFound indicates the referenced Environment doesn't exist
	ReasonEnvironmentNotFound controller.ConditionReason = "EnvironmentNotFound"
	// ReasonDataPlaneNotFound indicates the referenced DataPlane doesn't exist
	ReasonDataPlaneNotFound controller.ConditionReason = "DataPlaneNotFound"
	// ReasonInvalidConfiguration indicates the ComponentDeployment configuration is invalid
	ReasonInvalidConfiguration controller.ConditionReason = "InvalidConfiguration"
	// ReasonInvalidSnapshotConfiguration indicates the ComponentEnvSnapshot has invalid configuration
	ReasonInvalidSnapshotConfiguration controller.ConditionReason = "InvalidSnapshotConfiguration"
	// ReasonDataPlaneNotConfigured indicates the Environment has no DataPlaneRef configured
	ReasonDataPlaneNotConfigured controller.ConditionReason = "DataPlaneNotConfigured"

	// Rendering issues (Status=False)

	// ReasonRenderingFailed indicates failure to render resources from ComponentEnvSnapshot
	ReasonRenderingFailed controller.ConditionReason = "RenderingFailed"
	// ReasonValidationFailed indicates rendered resources failed validation
	ReasonValidationFailed controller.ConditionReason = "ValidationFailed"

	// Release management issues (Status=False)

	// ReasonReleaseUpdateFailed indicates failure to create or update the Release
	ReasonReleaseUpdateFailed controller.ConditionReason = "ReleaseUpdateFailed"
	// ReasonReleaseOwnershipConflict indicates Release exists but not owned by this ComponentDeployment
	ReasonReleaseOwnershipConflict controller.ConditionReason = "ReleaseOwnershipConflict"
)
