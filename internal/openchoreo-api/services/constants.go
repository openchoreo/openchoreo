// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

type systemAction string

const (
	SystemActionCreateProject systemAction = "project:create"
	SystemActionViewProject   systemAction = "project:view"
	SystemActionDeleteProject systemAction = "project:delete"

	SystemActionCreateComponent        systemAction = "component:create"
	SystemActionViewComponent          systemAction = "component:view"
	SystemActionUpdateComponent        systemAction = "component:update"
	SystemActionDeployComponent        systemAction = "component:deploy"
	SystemActionCreateComponentRelease systemAction = "componentrelease:create"
	SystemActionViewComponentRelease   systemAction = "componentrelease:view"
)

type ResourceType string

const (
	ResourceTypeProject          ResourceType = "project"
	ResourceTypeComponent        ResourceType = "component"
	ResourceTypeComponentRelease ResourceType = "componentRelease"
)
