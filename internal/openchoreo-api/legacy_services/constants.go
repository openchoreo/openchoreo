// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacy_services

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

	SystemActionCreateNamespace systemAction = "namespace:create"
	SystemActionViewNamespace   systemAction = "namespace:view"

	SystemActionCreateRole        systemAction = "role:create"
	SystemActionViewRole          systemAction = "role:view"
	SystemActionDeleteRole        systemAction = "role:delete"
	SystemActionUpdateRole        systemAction = "role:update"
	SystemActionCreateRoleMapping systemAction = "rolemapping:create"
	SystemActionViewRoleMapping   systemAction = "rolemapping:view"
	SystemActionDeleteRoleMapping systemAction = "rolemapping:delete"
	SystemActionUpdateRoleMapping systemAction = "rolemapping:update"

	SystemActionViewComponentType   systemAction = "componenttype:view"
	SystemActionCreateComponentType systemAction = "componenttype:create"
	SystemActionViewTrait           systemAction = "trait:view"
	SystemActionCreateTrait         systemAction = "trait:create"

	SystemActionCreateDataPlane systemAction = "dataplane:create"
	SystemActionViewDataPlane   systemAction = "dataplane:view"

	SystemActionViewBuildPlane systemAction = "buildplane:view"

	SystemActionViewObservabilityPlane systemAction = "observabilityplane:view"

	SystemActionCreateClusterDataPlane        systemAction = "clusterdataplane:create"
	SystemActionViewClusterDataPlane          systemAction = "clusterdataplane:view"
	SystemActionViewClusterBuildPlane         systemAction = "clusterbuildplane:view"
	SystemActionViewClusterObservabilityPlane systemAction = "clusterobservabilityplane:view"

	SystemActionCreateEnvironment systemAction = "environment:create"
	SystemActionViewEnvironment   systemAction = "environment:view"

	SystemActionViewDeploymentPipeline systemAction = "deploymentpipeline:view"

	SystemActionViewWorkflow systemAction = "workflow:view"

	SystemActionCreateWorkflowRun systemAction = "workflowrun:create"
	SystemActionViewWorkflowRun   systemAction = "workflowrun:view"

	SystemActionViewComponentWorkflow    systemAction = "componentworkflow:view"
	SystemActionCreateComponentWorkflow  systemAction = "componentworkflow:create"
	SystemActionViewComponentWorkflowRun systemAction = "componentworkflowrun:view"

	SystemActionCreateSecretReference systemAction = "secretreference:create"
	SystemActionViewSecretReference   systemAction = "secretreference:view"
	SystemActionDeleteSecretReference systemAction = "secretreference:delete"
)

type ResourceType string

const (
	ResourceTypeProject                   ResourceType = "project"
	ResourceTypeComponent                 ResourceType = "component"
	ResourceTypeComponentRelease          ResourceType = "componentRelease"
	ResourceTypeReleaseBinding            ResourceType = "releaseBinding"
	ResourceTypeWorkload                  ResourceType = "workload"
	ResourceTypeNamespace                 ResourceType = "namespace"
	ResourceTypeRole                      ResourceType = "role"
	ResourceTypeRoleMapping               ResourceType = "roleMapping"
	ResourceTypeComponentType             ResourceType = "componentType"
	ResourceTypeTrait                     ResourceType = "trait"
	ResourceTypeDataPlane                 ResourceType = "dataPlane"
	ResourceTypeBuildPlane                ResourceType = "buildPlane"
	ResourceTypeObservabilityPlane        ResourceType = "observabilityPlane"
	ResourceTypeClusterDataPlane          ResourceType = "clusterDataPlane"
	ResourceTypeClusterBuildPlane         ResourceType = "clusterBuildPlane"
	ResourceTypeClusterObservabilityPlane ResourceType = "clusterObservabilityPlane"
	ResourceTypeEnvironment               ResourceType = "environment"
	ResourceTypeDeploymentPipeline        ResourceType = "deploymentPipeline"
	ResourceTypeWorkflow                  ResourceType = "workflow"
	ResourceTypeWorkflowRun               ResourceType = "workflowRun"
	ResourceTypeComponentWorkflow         ResourceType = "componentWorkflow"
	ResourceTypeComponentWorkflowRun      ResourceType = "componentWorkflowRun"
	ResourceTypeSecretReference           ResourceType = "secretReference"
)

// Workflow run status constants
const (
	WorkflowRunStatusPending   = "Pending"
	WorkflowRunStatusCompleted = "Completed"
)
