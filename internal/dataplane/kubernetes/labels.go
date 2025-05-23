/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

const (
	LabelKeyOrganizationName    = "organization-name"
	LabelKeyProjectName         = "project-name"
	LabelKeyProjectID           = "project-id"
	LabelKeyComponentName       = "component-name"
	LabelKeyComponentID         = "component-id"
	LabelKeyDeploymentTrackName = "deployment-track-name"
	LabelKeyDeploymentTrackID   = "deployment-track-id"
	LabelKeyEnvironmentName     = "environment-name"
	LabelKeyEnvironmentID       = "environment-id"
	LabelKeyDeploymentName      = "deployment-name"
	LabelKeyDeploymentID        = "deployment-id"
	LabelKeyManagedBy           = "managed-by"
	LabelKeyBelongTo            = "belong-to"
	LabelKeyComponentType       = "component-type"

	LabelValueManagedBy = "choreo-deployment-controller"
	LabelValueBelongTo  = "user-workloads"

	LabelBuildControllerCreated = "choreo-build-controller"
)
