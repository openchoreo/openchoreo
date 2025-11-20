// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReleaseSynced indicates that the Release resource has been created/updated
	// from the ReleaseBinding
	ConditionReleaseSynced controller.ConditionType = "ReleaseSynced"

	// ConditionResourcesReady indicates that all resources in the Release are ready
	// based on workload-type specific evaluation
	ConditionResourcesReady controller.ConditionType = "ResourcesReady"

	// ConditionReady indicates the overall readiness of the ReleaseBinding
	// This is the top-level condition that aggregates ReleaseSynced and ResourcesReady
	ConditionReady controller.ConditionType = "Ready"
)

// Constants for condition reasons

const (
	// Success states (Status=True)

	// ReasonReleaseCreated indicates the Release was successfully created
	ReasonReleaseCreated controller.ConditionReason = "ReleaseCreated"
	// ReasonReleaseSynced indicates the Release is up to date
	ReasonReleaseSynced controller.ConditionReason = "ReleaseSynced"
	// ReasonResourcesReady indicates all resources are ready
	ReasonResourcesReady controller.ConditionReason = "ResourcesReady"

	// Configuration issues (Status=False)

	// ReasonComponentReleaseNotFound indicates the referenced ComponentRelease doesn't exist
	ReasonComponentReleaseNotFound controller.ConditionReason = "ComponentReleaseNotFound"
	// ReasonEnvironmentNotFound indicates the referenced Environment doesn't exist
	ReasonEnvironmentNotFound controller.ConditionReason = "EnvironmentNotFound"
	// ReasonDataPlaneNotFound indicates the referenced DataPlane doesn't exist
	ReasonDataPlaneNotFound controller.ConditionReason = "DataPlaneNotFound"
	// ReasonDataPlaneNotConfigured indicates the Environment has no DataPlaneRef
	ReasonDataPlaneNotConfigured controller.ConditionReason = "DataPlaneNotConfigured"
	// ReasonComponentNotFound indicates the referenced Component doesn't exist
	ReasonComponentNotFound controller.ConditionReason = "ComponentNotFound"
	// ReasonProjectNotFound indicates the referenced Project doesn't exist
	ReasonProjectNotFound controller.ConditionReason = "ProjectNotFound"
	// ReasonInvalidReleaseConfiguration indicates the ComponentRelease configuration is invalid
	ReasonInvalidReleaseConfiguration controller.ConditionReason = "InvalidReleaseConfiguration"

	// Rendering issues (Status=False)

	// ReasonRenderingFailed indicates failure to render resources
	ReasonRenderingFailed controller.ConditionReason = "RenderingFailed"

	// Release management issues (Status=False)

	// ReasonReleaseOwnershipConflict indicates the Release exists but is owned by another resource
	ReasonReleaseOwnershipConflict controller.ConditionReason = "ReleaseOwnershipConflict"
	// ReasonReleaseUpdateFailed indicates failure to create/update the Release
	ReasonReleaseUpdateFailed controller.ConditionReason = "ReleaseUpdateFailed"

	// Resource readiness issues (Status=False)

	// ReasonResourcesNotReady indicates one or more resources are not ready
	ReasonResourcesNotReady controller.ConditionReason = "ResourcesNotReady"
	// ReasonResourcesProgressing indicates resources are being created/updated
	ReasonResourcesProgressing controller.ConditionReason = "ResourcesProgressing"
	// ReasonResourcesDegraded indicates one or more resources are degraded
	ReasonResourcesDegraded controller.ConditionReason = "ResourcesDegraded"
	// ReasonResourcesUnknown indicates resource status is unknown
	ReasonResourcesUnknown controller.ConditionReason = "ResourcesUnknown"

	// Ready condition reasons

	// ReasonReady indicates the ReleaseBinding is fully ready
	ReasonReady controller.ConditionReason = "Ready"
	// ReasonReadyWithSuspendedResources indicates ready with suspended workload (scaled to 0)
	ReasonReadyWithSuspendedResources controller.ConditionReason = "ReadyWithSuspendedResources"

	// Job-specific reasons

	// ReasonJobCompleted indicates Job completed successfully
	ReasonJobCompleted controller.ConditionReason = "JobCompleted"
	// ReasonJobRunning indicates Job is running
	ReasonJobRunning controller.ConditionReason = "JobRunning"
	// ReasonJobFailed indicates Job failed
	ReasonJobFailed controller.ConditionReason = "JobFailed"

	// CronJob-specific reasons

	// ReasonCronJobScheduled indicates CronJob is scheduled and ready
	ReasonCronJobScheduled controller.ConditionReason = "CronJobScheduled"
	// ReasonCronJobSuspended indicates CronJob is suspended
	ReasonCronJobSuspended controller.ConditionReason = "CronJobSuspended"
)
