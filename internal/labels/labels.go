// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labels

// This file contains the all the labels that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	LabelKeyOrganizationName       = "core.choreo.dev/organization"
	LabelKeyProjectName            = "core.choreo.dev/project"
	LabelKeyComponentName          = "core.choreo.dev/component"
	LabelKeyDeploymentTrackName    = "core.choreo.dev/deployment-track"
	LabelKeyBuildName              = "core.choreo.dev/build"
	LabelKeyEnvironmentName        = "core.choreo.dev/environment"
	LabelKeyName                   = "core.choreo.dev/name"
	LabelKeyDeployableArtifactName = "core.choreo.dev/deployable-artifact"
	LabelKeyDeploymentName         = "core.choreo.dev/deployment"
	LabelKeyDataPlaneName          = "core.choreo.dev/dataplane"
	LabelKeyBuildPlane             = "core.choreo.dev/build-plane"

	LabelKeyManagedBy = "core.choreo.dev/managed-by"

	// Release controller specific labels
	LabelKeyReleaseResourceID = "core.choreo.dev/release-resource-id"
	LabelKeyReleaseUID        = "core.choreo.dev/release-uid"

	LabelValueManagedBy = "choreo-control-plane"
)
