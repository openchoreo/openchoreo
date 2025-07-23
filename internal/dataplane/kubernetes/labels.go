// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

const (
	LabelKeyOrganizationName    = "organization-name"
	LabelKeyEnvironmentName     = "environment-name"
	LabelKeyEnvironmentID       = "environment-id"
	LabelKeyProjectName         = "project-name"
	LabelKeyProjectID           = "project-id"
	LabelKeyComponentName       = "component-name"
	LabelKeyComponentID         = "component-id"
	LabelKeyDeploymentTrackName = "deployment-track-name"
	LabelKeyDeploymentTrackID   = "deployment-track-id"
	LabelKeyBuildName           = "build-name"
	LabelKeyDeploymentName      = "deployment-name"
	LabelKeyDeploymentID        = "deployment-id"
	LabelKeyManagedBy           = "managed-by"
	LabelKeyBelongTo            = "belong-to"
	LabelKeyComponentType       = "component-type"
	LabelKeyVisibility          = "gateway-visibility"
	
	// LabelKeyUUID stores the Kubernetes UID (metadata.uid) of the resource.
	LabelKeyUUID = "uuid"

	// LabelKeyTarget identifies which logical target a resource belongs to
	// Allowed values: build | runtime | gateway | <future‑targets>
	LabelKeyTarget = "target"

	// Predefined values for LabelKeyTarget.
	LabelValueBuildTarget   = "build"
	LabelValueRuntimeTarget = "runtime"
	LabelValueGatewayTarget = "gateway"

	LabelValueManagedBy = "choreo-deployment-controller"
	LabelValueBelongTo  = "user-workloads"
)
