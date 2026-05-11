// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Interface defines all API client methods used by occ commands.
// The concrete *Client type satisfies this interface.
type Interface interface {
	ListNamespaces(ctx context.Context, params *gen.ListNamespacesParams) (*gen.NamespaceList, error)
	GetNamespace(ctx context.Context, namespaceName string) (*gen.Namespace, error)
	DeleteNamespace(ctx context.Context, namespaceName string) error

	ListProjects(ctx context.Context, namespaceName string, params *gen.ListProjectsParams) (*gen.ProjectList, error)
	GetProject(ctx context.Context, namespaceName, projectName string) (*gen.Project, error)
	DeleteProject(ctx context.Context, namespaceName, projectName string) error
	GetProjectDeploymentPipeline(ctx context.Context, namespaceName, projectName string) (*gen.DeploymentPipeline, error)

	ListComponents(ctx context.Context, namespaceName, projectName string, params *gen.ListComponentsParams) (*gen.ComponentList, error)
	GetComponent(ctx context.Context, namespaceName, componentName string) (*gen.Component, error)
	DeleteComponent(ctx context.Context, namespaceName, componentName string) error

	ListEnvironments(ctx context.Context, namespaceName string, params *gen.ListEnvironmentsParams) (*gen.EnvironmentList, error)
	GetEnvironment(ctx context.Context, namespaceName, envName string) (*gen.Environment, error)
	DeleteEnvironment(ctx context.Context, namespaceName, envName string) error

	ListDataPlanes(ctx context.Context, namespaceName string, params *gen.ListDataPlanesParams) (*gen.DataPlaneList, error)
	GetDataPlane(ctx context.Context, namespaceName, dpName string) (*gen.DataPlane, error)
	DeleteDataPlane(ctx context.Context, namespaceName, dpName string) error

	ListClusterDataPlanes(ctx context.Context, params *gen.ListClusterDataPlanesParams) (*gen.ClusterDataPlaneList, error)
	GetClusterDataPlane(ctx context.Context, cdpName string) (*gen.ClusterDataPlane, error)
	DeleteClusterDataPlane(ctx context.Context, cdpName string) error

	ListWorkflowPlanes(ctx context.Context, namespaceName string, params *gen.ListWorkflowPlanesParams) (*gen.WorkflowPlaneList, error)
	GetWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) (*gen.WorkflowPlane, error)
	DeleteWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) error

	ListClusterWorkflowPlanes(ctx context.Context, params *gen.ListClusterWorkflowPlanesParams) (*gen.ClusterWorkflowPlaneList, error)
	GetClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) (*gen.ClusterWorkflowPlane, error)
	DeleteClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) error

	ListObservabilityPlanes(ctx context.Context, namespaceName string, params *gen.ListObservabilityPlanesParams) (*gen.ObservabilityPlaneList, error)
	GetObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) (*gen.ObservabilityPlane, error)
	DeleteObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) error

	ListClusterObservabilityPlanes(ctx context.Context, params *gen.ListClusterObservabilityPlanesParams) (*gen.ClusterObservabilityPlaneList, error)
	GetClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) (*gen.ClusterObservabilityPlane, error)
	DeleteClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) error

	ListComponentTypes(ctx context.Context, namespaceName string, params *gen.ListComponentTypesParams) (*gen.ComponentTypeList, error)
	GetComponentType(ctx context.Context, namespaceName, ctName string) (*gen.ComponentType, error)
	CreateComponentType(ctx context.Context, namespaceName string, ct gen.ComponentType) (*gen.ComponentType, error)
	UpdateComponentType(ctx context.Context, namespaceName, ctName string, ct gen.ComponentType) (*gen.ComponentType, error)
	DeleteComponentType(ctx context.Context, namespaceName, ctName string) error
	GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (*json.RawMessage, error)

	ListClusterComponentTypes(ctx context.Context, params *gen.ListClusterComponentTypesParams) (*gen.ClusterComponentTypeList, error)
	GetClusterComponentType(ctx context.Context, cctName string) (*gen.ClusterComponentType, error)
	DeleteClusterComponentType(ctx context.Context, cctName string) error
	GetClusterComponentTypeSchema(ctx context.Context, cctName string) (*json.RawMessage, error)

	ListTraits(ctx context.Context, namespaceName string, params *gen.ListTraitsParams) (*gen.TraitList, error)
	GetTrait(ctx context.Context, namespaceName, traitName string) (*gen.Trait, error)
	CreateTrait(ctx context.Context, namespaceName string, t gen.Trait) (*gen.Trait, error)
	UpdateTrait(ctx context.Context, namespaceName, traitName string, t gen.Trait) (*gen.Trait, error)
	DeleteTrait(ctx context.Context, namespaceName, traitName string) error
	GetTraitSchema(ctx context.Context, namespaceName, traitName string) (*json.RawMessage, error)

	ListClusterTraits(ctx context.Context, params *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error)
	GetClusterTrait(ctx context.Context, clusterTraitName string) (*gen.ClusterTrait, error)
	DeleteClusterTrait(ctx context.Context, clusterTraitName string) error
	GetClusterTraitSchema(ctx context.Context, clusterTraitName string) (*json.RawMessage, error)

	ListWorkflows(ctx context.Context, namespaceName string, params *gen.ListWorkflowsParams) (*gen.WorkflowList, error)
	GetWorkflow(ctx context.Context, namespaceName, workflowName string) (*gen.Workflow, error)
	DeleteWorkflow(ctx context.Context, namespaceName, workflowName string) error
	GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (*json.RawMessage, error)

	ListClusterWorkflows(ctx context.Context, params *gen.ListClusterWorkflowsParams) (*gen.ClusterWorkflowList, error)
	GetClusterWorkflow(ctx context.Context, clusterWorkflowName string) (*gen.ClusterWorkflow, error)
	DeleteClusterWorkflow(ctx context.Context, clusterWorkflowName string) error
	GetClusterWorkflowSchema(ctx context.Context, clusterWorkflowName string) (*json.RawMessage, error)

	ListWorkflowRuns(ctx context.Context, namespaceName string, params *gen.ListWorkflowRunsParams) (*gen.WorkflowRunList, error)
	GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*gen.WorkflowRun, error)
	CreateWorkflowRun(ctx context.Context, namespace string, body gen.CreateWorkflowRunJSONRequestBody) (*gen.WorkflowRun, error)
	GetWorkflowRunStatus(ctx context.Context, namespaceName, runName string) (*gen.WorkflowRunStatusResponse, error)
	GetWorkflowRunLogs(ctx context.Context, namespaceName, runName string, params *gen.GetWorkflowRunLogsParams) ([]gen.WorkflowRunLogEntry, error)

	ListComponentReleases(ctx context.Context, namespaceName string, params *gen.ListComponentReleasesParams) (*gen.ComponentReleaseList, error)
	GetComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) (*gen.ComponentRelease, error)
	CreateComponentRelease(ctx context.Context, namespaceName string, cr gen.ComponentRelease) (*gen.ComponentRelease, error)
	DeleteComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) error
	GenerateRelease(ctx context.Context, namespaceName, componentName string, req gen.GenerateReleaseRequest) (*gen.ComponentRelease, error)

	ListReleaseBindings(ctx context.Context, namespaceName string, params *gen.ListReleaseBindingsParams) (*gen.ReleaseBindingList, error)
	GetReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) (*gen.ReleaseBinding, error)
	CreateReleaseBinding(ctx context.Context, namespaceName string, req gen.ReleaseBinding) (*gen.ReleaseBinding, error)
	UpdateReleaseBinding(ctx context.Context, namespaceName, bindingName string, req gen.ReleaseBinding) (*gen.ReleaseBinding, error)
	DeleteReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) error

	ListResourceTypes(ctx context.Context, namespaceName string, params *gen.ListResourceTypesParams) (*gen.ResourceTypeList, error)
	GetResourceType(ctx context.Context, namespaceName, rtName string) (*gen.ResourceType, error)
	CreateResourceType(ctx context.Context, namespaceName string, rt gen.ResourceType) (*gen.ResourceType, error)
	UpdateResourceType(ctx context.Context, namespaceName, rtName string, rt gen.ResourceType) (*gen.ResourceType, error)
	DeleteResourceType(ctx context.Context, namespaceName, rtName string) error
	GetResourceTypeSchema(ctx context.Context, namespaceName, rtName string) (*json.RawMessage, error)

	ListClusterResourceTypes(ctx context.Context, params *gen.ListClusterResourceTypesParams) (*gen.ClusterResourceTypeList, error)
	GetClusterResourceType(ctx context.Context, crtName string) (*gen.ClusterResourceType, error)
	CreateClusterResourceType(ctx context.Context, crt gen.ClusterResourceType) (*gen.ClusterResourceType, error)
	UpdateClusterResourceType(ctx context.Context, crtName string, crt gen.ClusterResourceType) (*gen.ClusterResourceType, error)
	DeleteClusterResourceType(ctx context.Context, crtName string) error
	GetClusterResourceTypeSchema(ctx context.Context, crtName string) (*json.RawMessage, error)

	ListResources(ctx context.Context, namespaceName string, params *gen.ListResourcesParams) (*gen.ResourceInstanceList, error)
	GetResource(ctx context.Context, namespaceName, resourceName string) (*gen.ResourceInstance, error)
	CreateResource(ctx context.Context, namespaceName string, r gen.ResourceInstance) (*gen.ResourceInstance, error)
	UpdateResource(ctx context.Context, namespaceName, resourceName string, r gen.ResourceInstance) (*gen.ResourceInstance, error)
	DeleteResource(ctx context.Context, namespaceName, resourceName string) error

	ListResourceReleases(ctx context.Context, namespaceName string, params *gen.ListResourceReleasesParams) (*gen.ResourceReleaseList, error)
	GetResourceRelease(ctx context.Context, namespaceName, resourceReleaseName string) (*gen.ResourceRelease, error)
	CreateResourceRelease(ctx context.Context, namespaceName string, rr gen.ResourceRelease) (*gen.ResourceRelease, error)
	DeleteResourceRelease(ctx context.Context, namespaceName, resourceReleaseName string) error

	ListResourceReleaseBindings(ctx context.Context, namespaceName string, params *gen.ListResourceReleaseBindingsParams) (*gen.ResourceReleaseBindingList, error)
	GetResourceReleaseBinding(ctx context.Context, namespaceName, bindingName string) (*gen.ResourceReleaseBinding, error)
	CreateResourceReleaseBinding(ctx context.Context, namespaceName string, rrb gen.ResourceReleaseBinding) (*gen.ResourceReleaseBinding, error)
	UpdateResourceReleaseBinding(ctx context.Context, namespaceName, bindingName string, rrb gen.ResourceReleaseBinding) (*gen.ResourceReleaseBinding, error)
	DeleteResourceReleaseBinding(ctx context.Context, namespaceName, bindingName string) error

	ListDeploymentPipelines(ctx context.Context, namespaceName string, params *gen.ListDeploymentPipelinesParams) (*gen.DeploymentPipelineList, error)
	GetDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) (*gen.DeploymentPipeline, error)
	DeleteDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) error

	ListSecretReferences(ctx context.Context, namespaceName string, params *gen.ListSecretReferencesParams) (*gen.SecretReferenceList, error)
	GetSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (*gen.SecretReference, error)
	DeleteSecretReference(ctx context.Context, namespaceName, secretReferenceName string) error

	ListWorkloads(ctx context.Context, namespaceName string, params *gen.ListWorkloadsParams) (*gen.WorkloadList, error)
	GetWorkload(ctx context.Context, namespaceName, workloadName string) (*gen.Workload, error)
	DeleteWorkload(ctx context.Context, namespaceName, workloadName string) error

	ListObservabilityAlertsNotificationChannels(ctx context.Context, namespaceName string, params *gen.ListObservabilityAlertsNotificationChannelsParams) (*gen.ObservabilityAlertsNotificationChannelList, error)
	GetObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) (*gen.ObservabilityAlertsNotificationChannel, error)
	DeleteObservabilityAlertsNotificationChannel(ctx context.Context, namespaceName, channelName string) error

	ListClusterRoles(ctx context.Context, params *gen.ListClusterRolesParams) (*gen.ClusterAuthzRoleList, error)
	GetClusterRole(ctx context.Context, name string) (*gen.ClusterAuthzRole, error)
	DeleteClusterRole(ctx context.Context, name string) error

	ListClusterRoleBindings(ctx context.Context, params *gen.ListClusterRoleBindingsParams) (*gen.ClusterAuthzRoleBindingList, error)
	GetClusterRoleBinding(ctx context.Context, name string) (*gen.ClusterAuthzRoleBinding, error)
	DeleteClusterRoleBinding(ctx context.Context, name string) error

	ListNamespaceRoles(ctx context.Context, namespaceName string, params *gen.ListNamespaceRolesParams) (*gen.AuthzRoleList, error)
	GetNamespaceRole(ctx context.Context, namespaceName, name string) (*gen.AuthzRole, error)
	DeleteNamespaceRole(ctx context.Context, namespaceName, name string) error

	ListNamespaceRoleBindings(ctx context.Context, namespaceName string, params *gen.ListNamespaceRoleBindingsParams) (*gen.AuthzRoleBindingList, error)
	GetNamespaceRoleBinding(ctx context.Context, namespaceName, name string) (*gen.AuthzRoleBinding, error)
	DeleteNamespaceRoleBinding(ctx context.Context, namespaceName, name string) error
}

// compile-time check that *Client satisfies Interface.
var _ Interface = (*Client)(nil)

// NewClientFunc is a factory function that creates a new API client.
// Commands receive this at construction time instead of calling NewClient() directly.
type NewClientFunc func() (Interface, error)
