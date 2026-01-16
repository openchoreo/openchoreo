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
	SystemActionDeleteComponent        systemAction = "component:delete"
	SystemActionDeployComponent        systemAction = "component:deploy"
	SystemActionCreateComponentRelease systemAction = "componentrelease:create"
	SystemActionViewComponentRelease   systemAction = "componentrelease:view"

	SystemActionUpdateReleaseBinding systemAction = "releasebinding:update"
	SystemActionViewReleaseBinding   systemAction = "releasebinding:view"

	SystemActionCreateWorkload systemAction = "workload:create"
	SystemActionViewWorkload   systemAction = "workload:view"

	SystemActionViewOrganization systemAction = "organization:view"

	SystemActionCreateRole        systemAction = "role:create"
	SystemActionViewRole          systemAction = "role:view"
	SystemActionDeleteRole        systemAction = "role:delete"
	SystemActionUpdateRole        systemAction = "role:update"
	SystemActionCreateRoleMapping systemAction = "rolemapping:create"
	SystemActionViewRoleMapping   systemAction = "rolemapping:view"
	SystemActionDeleteRoleMapping systemAction = "rolemapping:delete"
	SystemActionUpdateRoleMapping systemAction = "rolemapping:update"

	SystemActionViewComponentType systemAction = "componenttype:view"
	SystemActionViewTrait         systemAction = "trait:view"

	SystemActionCreateDataPlane systemAction = "dataplane:create"
	SystemActionViewDataPlane   systemAction = "dataplane:view"

	SystemActionViewBuildPlane systemAction = "buildplane:view"

	SystemActionCreateEnvironment systemAction = "environment:create"
	SystemActionViewEnvironment   systemAction = "environment:view"

	SystemActionViewDeploymentPipeline systemAction = "deploymentpipeline:view"

	SystemActionViewWorkflow systemAction = "workflow:view"

	SystemActionViewComponentWorkflow    systemAction = "componentworkflow:view"
	SystemActionCreateComponentWorkflow  systemAction = "componentworkflow:create"
	SystemActionViewComponentWorkflowRun systemAction = "componentworkflowrun:view"

	SystemActionViewSecretReference systemAction = "secretreference:view"
)

type ResourceType string

const (
	ResourceTypeProject              ResourceType = "project"
	ResourceTypeComponent            ResourceType = "component"
	ResourceTypeComponentRelease     ResourceType = "componentRelease"
	ResourceTypeReleaseBinding       ResourceType = "releaseBinding"
	ResourceTypeWorkload             ResourceType = "workload"
	ResourceTypeOrganization         ResourceType = "organization"
	ResourceTypeRole                 ResourceType = "role"
	ResourceTypeRoleMapping          ResourceType = "roleMapping"
	ResourceTypeComponentType        ResourceType = "componentType"
	ResourceTypeTrait                ResourceType = "trait"
	ResourceTypeDataPlane            ResourceType = "dataPlane"
	ResourceTypeBuildPlane           ResourceType = "buildPlane"
	ResourceTypeEnvironment          ResourceType = "environment"
	ResourceTypeDeploymentPipeline   ResourceType = "deploymentPipeline"
	ResourceTypeWorkflow             ResourceType = "workflow"
	ResourceTypeComponentWorkflow    ResourceType = "componentWorkflow"
	ResourceTypeComponentWorkflowRun ResourceType = "componentWorkflowRun"
	ResourceTypeSecretReference      ResourceType = "secretReference"
)
