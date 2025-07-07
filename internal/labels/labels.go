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

	// LabelKeyCreatedBy identifies which controller initially created a resource (audit trail).
	// Example: A namespace created by release-controller would have created-by=release-controller.
	// Note: For shared resources like namespaces, the creator and lifecycle manager may differ.
	LabelKeyCreatedBy = "core.choreo.dev/created-by"

	// LabelKeyManagedBy identifies which controller manages the lifecycle of a resource.
	// Example: Resources deployed by release-controller have managed-by=release-controller.
	LabelKeyManagedBy = "core.choreo.dev/managed-by"

	// LabelKeyReleaseResourceID identifies a specific resource within a release.
	LabelKeyReleaseResourceID = "core.choreo.dev/release-resource-id"

	// LabelKeyReleaseUID tracks which release UID owns/manages a resource.
	LabelKeyReleaseUID = "core.choreo.dev/release-uid"

	// LabelKeyReleaseName tracks the name of the release that manages a resource.
	LabelKeyReleaseName = "core.choreo.dev/release-name"

	// LabelKeyReleaseNamespace tracks the namespace of the release that manages a resource.
	LabelKeyReleaseNamespace = "core.choreo.dev/release-namespace"

	LabelValueManagedBy = "choreo-control-plane"
)
