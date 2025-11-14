// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package servicebinding

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionReady indicates that the ServiceBinding is ready and functioning
	ConditionReady controller.ConditionType = "Ready"
)

// Constants for condition reasons

const (
	// Reasons for the Ready condition based on ReleaseState

	// ReasonResourcesActive indicates all resources are deployed and actively running (ReleaseState=Active)
	ReasonResourcesActive controller.ConditionReason = "ResourcesActive"
	// ReasonResourcesSuspended indicates resources are intentionally suspended (ReleaseState=Suspend)
	ReasonResourcesSuspended controller.ConditionReason = "ResourcesSuspended"
	// ReasonResourcesUndeployed indicates resources are intentionally undeployed (ReleaseState=Undeploy)
	ReasonResourcesUndeployed controller.ConditionReason = "ResourcesUndeployed"

	// Reasons for the Ready condition type when status is False - Resource Health Issues

	// ReasonResourceHealthProgressing indicates one or more resources are being deployed/updated
	ReasonResourceHealthProgressing controller.ConditionReason = "ResourceHealthProgressing"
	// ReasonResourceHealthDegraded indicates one or more resources are in error state
	ReasonResourceHealthDegraded controller.ConditionReason = "ResourceHealthDegraded"

	// Reasons for the Ready condition type when status is False - Configuration Issues

	// ReasonServiceClassNotFound indicates the referenced ServiceClass doesn't exist
	ReasonServiceClassNotFound controller.ConditionReason = "ServiceClassNotFound"
	// ReasonAPIClassNotFound indicates a referenced APIClass doesn't exist
	ReasonAPIClassNotFound controller.ConditionReason = "APIClassNotFound"
	// ReasonEnvironmentNotFound indicates the referenced Environment doesn't exist
	ReasonEnvironmentNotFound controller.ConditionReason = "EnvironmentNotFound"
	// ReasonDataPlaneNotFound indicates the referenced DataPlane doesn't exist
	ReasonDataPlaneNotFound controller.ConditionReason = "DataPlaneNotFound"
	// ReasonSecretReferenceNotFound indicates a referenced SecretReference doesn't exist
	ReasonSecretReferenceNotFound controller.ConditionReason = "SecretReferenceNotFound"
	// ReasonInvalidConfiguration indicates the binding configuration is invalid
	ReasonInvalidConfiguration controller.ConditionReason = "InvalidConfiguration"

	// Reasons for the Ready condition type when status is False - Release Issues

	// ReasonReleaseCreationFailed indicates failure to create the Release
	ReasonReleaseCreationFailed controller.ConditionReason = "ReleaseCreationFailed"
	// ReasonReleaseUpdateFailed indicates failure to update the Release
	ReasonReleaseUpdateFailed controller.ConditionReason = "ReleaseUpdateFailed"
	// ReasonReleaseDeletionFailed indicates failure to delete the Release during undeploy
	ReasonReleaseDeletionFailed controller.ConditionReason = "ReleaseDeletionFailed"
)
