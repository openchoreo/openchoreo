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
	// Reasons for the Ready condition type when status is True

	// ReasonAllResourcesReady indicates all resources are deployed and healthy
	ReasonAllResourcesReady controller.ConditionReason = "AllResourcesReady"
	// ReasonResourcesReadyWithSuspended indicates all resources are ready (some intentionally suspended)
	ReasonResourcesReadyWithSuspended controller.ConditionReason = "ResourcesReadyWithSuspended"

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
	// ReasonInvalidConfiguration indicates the binding configuration is invalid
	ReasonInvalidConfiguration controller.ConditionReason = "InvalidConfiguration"

	// Reasons for the Ready condition type when status is False - Release Issues

	// ReasonReleaseCreationFailed indicates failure to create the Release
	ReasonReleaseCreationFailed controller.ConditionReason = "ReleaseCreationFailed"
	// ReasonReleaseUpdateFailed indicates failure to update the Release
	ReasonReleaseUpdateFailed controller.ConditionReason = "ReleaseUpdateFailed"
)
