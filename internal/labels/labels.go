// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labels

// This file contains the all the labels that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	LabelKeyOrganizationName       = "openchoreo.dev/organization"
	LabelKeyProjectName            = "openchoreo.dev/project"
	LabelKeyComponentName          = "openchoreo.dev/component"
	LabelKeyDeploymentTrackName    = "openchoreo.dev/deployment-track"
	LabelKeyBuildName              = "openchoreo.dev/build"
	LabelKeyEnvironmentName        = "openchoreo.dev/environment"
	LabelKeyName                   = "openchoreo.dev/name"
	LabelKeyDeployableArtifactName = "openchoreo.dev/deployable-artifact"
	LabelKeyDeploymentName         = "openchoreo.dev/deployment"
	LabelKeyDataPlaneName          = "openchoreo.dev/dataplane"
	LabelKeyBuildPlane             = "openchoreo.dev/build-plane"

	// LabelKeyCreatedBy identifies which controller initially created a resource (audit trail).
	// Example: A namespace created by release-controller would have created-by=release-controller.
	// Note: For shared resources like namespaces, the creator and lifecycle manager may differ.
	LabelKeyCreatedBy = "openchoreo.dev/created-by"

	// LabelKeyManagedBy identifies which controller manages the lifecycle of a resource.
	// Example: Resources deployed by release-controller have managed-by=release-controller.
	LabelKeyManagedBy = "openchoreo.dev/managed-by"

	// LabelKeyReleaseResourceID identifies a specific resource within a release.
	LabelKeyReleaseResourceID = "openchoreo.dev/release-resource-id"

	// LabelKeyReleaseUID tracks which release UID owns/manages a resource.
	LabelKeyReleaseUID = "openchoreo.dev/release-uid"

	// LabelKeyReleaseName tracks the name of the release that manages a resource.
	LabelKeyReleaseName = "openchoreo.dev/release-name"

	// LabelKeyReleaseNamespace tracks the namespace of the release that manages a resource.
	LabelKeyReleaseNamespace = "openchoreo.dev/release-namespace"

	// LabelKeyTarget identifies which logical target a resource belongs to
	// Allowed values: build | runtime | gateway | <future‑targets>
	LabelKeyTarget = "openchoreo.dev/target"

	// Predefined values for LabelKeyTarget.
	LabelValueBuildTarget   = "build"
	LabelValueRuntimeTarget = "runtime"
	LabelValueGatewayTarget = "gateway"

	// LabelKeyUUID stores the Kubernetes UID (metadata.uid) of the resource.
	LabelKeyUUID = "openchoreo.dev/uuid"

	LabelValueManagedBy = "openchoreo-control-plane"
)
