// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"sort"
	"strings"
)

// ActionScope represents the resource hierarchy level at which an action is evaluated.
type ActionScope string

const (
	// ScopeCluster indicates the action is evaluated at the cluster level (no hierarchy).
	ScopeCluster ActionScope = "cluster"
	// ScopeNamespace indicates the action is evaluated at the namespace level.
	ScopeNamespace ActionScope = "namespace"
	// ScopeProject indicates the action is evaluated at the project level.
	ScopeProject ActionScope = "project"
	// ScopeComponent indicates the action is evaluated at the component level.
	ScopeComponent ActionScope = "component"
)

// Action name constants for use in authorization checks.
const (
	// Namespace actions
	ActionCreateNamespace = "namespace:create"
	ActionViewNamespace   = "namespace:view"
	ActionUpdateNamespace = "namespace:update"
	ActionDeleteNamespace = "namespace:delete"

	// Project actions
	ActionCreateProject = "project:create"
	ActionViewProject   = "project:view"
	ActionUpdateProject = "project:update"
	ActionDeleteProject = "project:delete"

	// Component actions
	ActionCreateComponent = "component:create"
	ActionViewComponent   = "component:view"
	ActionUpdateComponent = "component:update"
	ActionDeleteComponent = "component:delete"

	// ComponentRelease actions
	ActionCreateComponentRelease = "componentrelease:create"
	ActionViewComponentRelease   = "componentrelease:view"
	ActionDeleteComponentRelease = "componentrelease:delete"

	// ReleaseBinding actions
	ActionCreateReleaseBinding = "releasebinding:create"
	ActionViewReleaseBinding   = "releasebinding:view"
	ActionUpdateReleaseBinding = "releasebinding:update"
	ActionDeleteReleaseBinding = "releasebinding:delete"

	// ComponentType actions
	ActionCreateComponentType = "componenttype:create"
	ActionViewComponentType   = "componenttype:view"
	ActionUpdateComponentType = "componenttype:update"
	ActionDeleteComponentType = "componenttype:delete"

	// Workflow actions
	ActionCreateWorkflow = "workflow:create"
	ActionViewWorkflow   = "workflow:view"
	ActionUpdateWorkflow = "workflow:update"
	ActionDeleteWorkflow = "workflow:delete"

	// WorkflowRun actions
	ActionCreateWorkflowRun = "workflowrun:create"
	ActionViewWorkflowRun   = "workflowrun:view"
	ActionUpdateWorkflowRun = "workflowrun:update"

	// Trait actions
	ActionCreateTrait = "trait:create"
	ActionViewTrait   = "trait:view"
	ActionUpdateTrait = "trait:update"
	ActionDeleteTrait = "trait:delete"

	// Environment actions
	ActionCreateEnvironment = "environment:create"
	ActionViewEnvironment   = "environment:view"
	ActionUpdateEnvironment = "environment:update"
	ActionDeleteEnvironment = "environment:delete"

	// DataPlane actions
	ActionCreateDataPlane = "dataplane:create"
	ActionViewDataPlane   = "dataplane:view"
	ActionUpdateDataPlane = "dataplane:update"
	ActionDeleteDataPlane = "dataplane:delete"

	// WorkflowPlane actions
	ActionCreateWorkflowPlane = "workflowplane:create"
	ActionViewWorkflowPlane   = "workflowplane:view"
	ActionUpdateWorkflowPlane = "workflowplane:update"
	ActionDeleteWorkflowPlane = "workflowplane:delete"

	// ObservabilityPlane actions
	ActionCreateObservabilityPlane = "observabilityplane:create"
	ActionViewObservabilityPlane   = "observabilityplane:view"
	ActionUpdateObservabilityPlane = "observabilityplane:update"
	ActionDeleteObservabilityPlane = "observabilityplane:delete"

	// ClusterComponentType actions
	ActionCreateClusterComponentType = "clustercomponenttype:create"
	ActionViewClusterComponentType   = "clustercomponenttype:view"
	ActionUpdateClusterComponentType = "clustercomponenttype:update"
	ActionDeleteClusterComponentType = "clustercomponenttype:delete"

	// ClusterTrait actions
	ActionCreateClusterTrait = "clustertrait:create"
	ActionViewClusterTrait   = "clustertrait:view"
	ActionUpdateClusterTrait = "clustertrait:update"
	ActionDeleteClusterTrait = "clustertrait:delete"

	// ClusterWorkflow actions
	ActionCreateClusterWorkflow = "clusterworkflow:create"
	ActionViewClusterWorkflow   = "clusterworkflow:view"
	ActionUpdateClusterWorkflow = "clusterworkflow:update"
	ActionDeleteClusterWorkflow = "clusterworkflow:delete"

	// ClusterDataPlane actions
	ActionCreateClusterDataPlane = "clusterdataplane:create"
	ActionViewClusterDataPlane   = "clusterdataplane:view"
	ActionUpdateClusterDataPlane = "clusterdataplane:update"
	ActionDeleteClusterDataPlane = "clusterdataplane:delete"

	// ClusterWorkflowPlane actions
	ActionCreateClusterWorkflowPlane = "clusterworkflowplane:create"
	ActionViewClusterWorkflowPlane   = "clusterworkflowplane:view"
	ActionUpdateClusterWorkflowPlane = "clusterworkflowplane:update"
	ActionDeleteClusterWorkflowPlane = "clusterworkflowplane:delete"

	// ClusterObservabilityPlane actions
	ActionCreateClusterObservabilityPlane = "clusterobservabilityplane:create"
	ActionViewClusterObservabilityPlane   = "clusterobservabilityplane:view"
	ActionUpdateClusterObservabilityPlane = "clusterobservabilityplane:update"
	ActionDeleteClusterObservabilityPlane = "clusterobservabilityplane:delete"

	// DeploymentPipeline actions
	ActionCreateDeploymentPipeline = "deploymentpipeline:create"
	ActionViewDeploymentPipeline   = "deploymentpipeline:view"
	ActionUpdateDeploymentPipeline = "deploymentpipeline:update"
	ActionDeleteDeploymentPipeline = "deploymentpipeline:delete"

	// ObservabilityAlertsNotificationChannel actions
	ActionCreateObservabilityAlertsNotificationChannel = "observabilityalertsnotificationchannel:create"
	ActionViewObservabilityAlertsNotificationChannel   = "observabilityalertsnotificationchannel:view"
	ActionUpdateObservabilityAlertsNotificationChannel = "observabilityalertsnotificationchannel:update"
	ActionDeleteObservabilityAlertsNotificationChannel = "observabilityalertsnotificationchannel:delete"

	// SecretReference actions
	ActionCreateSecretReference = "secretreference:create"
	ActionViewSecretReference   = "secretreference:view"
	ActionUpdateSecretReference = "secretreference:update"
	ActionDeleteSecretReference = "secretreference:delete"

	// Workload actions
	ActionCreateWorkload = "workload:create"
	ActionViewWorkload   = "workload:view"
	ActionUpdateWorkload = "workload:update"
	ActionDeleteWorkload = "workload:delete"

	// ClusterAuthzRole actions
	ActionCreateClusterAuthzRole = "clusterauthzrole:create"
	ActionViewClusterAuthzRole   = "clusterauthzrole:view"
	ActionUpdateClusterAuthzRole = "clusterauthzrole:update"
	ActionDeleteClusterAuthzRole = "clusterauthzrole:delete"

	// AuthzRole actions
	ActionCreateAuthzRole = "authzrole:create"
	ActionViewAuthzRole   = "authzrole:view"
	ActionUpdateAuthzRole = "authzrole:update"
	ActionDeleteAuthzRole = "authzrole:delete"

	// ClusterAuthzRoleBinding actions
	ActionCreateClusterAuthzRoleBinding = "clusterauthzrolebinding:create"
	ActionViewClusterAuthzRoleBinding   = "clusterauthzrolebinding:view"
	ActionUpdateClusterAuthzRoleBinding = "clusterauthzrolebinding:update"
	ActionDeleteClusterAuthzRoleBinding = "clusterauthzrolebinding:delete"

	// AuthzRoleBinding actions
	ActionCreateAuthzRoleBinding = "authzrolebinding:create"
	ActionViewAuthzRoleBinding   = "authzrolebinding:view"
	ActionUpdateAuthzRoleBinding = "authzrolebinding:update"
	ActionDeleteAuthzRoleBinding = "authzrolebinding:delete"

	// Logs actions
	ActionViewLogs = "logs:view"

	// Metrics actions
	ActionViewMetrics = "metrics:view"

	// Traces actions
	ActionViewTraces = "traces:view"

	// Alerts actions
	ActionViewAlerts = "alerts:view"

	// Incidents actions
	ActionViewIncidents   = "incidents:view"
	ActionUpdateIncidents = "incidents:update"

	// RCA Report actions
	ActionViewRCAReport   = "rcareport:view"
	ActionUpdateRCAReport = "rcareport:update"
)

// Action represents a system action with metadata
type Action struct {
	Name string
	// LowestScope indicates the lowest resource hierarchy level at which this action is evaluated
	LowestScope ActionScope
	// IsInternal indicates if the action is internal (not publicly visible)
	IsInternal bool
}

// systemActions defines all available actions in the system
var systemActions = []Action{
	// Namespace
	{Name: ActionCreateNamespace, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionViewNamespace, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateNamespace, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteNamespace, LowestScope: ScopeNamespace, IsInternal: false},

	// Project
	{Name: ActionCreateProject, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionViewProject, LowestScope: ScopeProject, IsInternal: false},
	{Name: ActionUpdateProject, LowestScope: ScopeProject, IsInternal: false},
	{Name: ActionDeleteProject, LowestScope: ScopeProject, IsInternal: false},

	// Component
	{Name: ActionCreateComponent, LowestScope: ScopeProject, IsInternal: false},
	{Name: ActionViewComponent, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionUpdateComponent, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionDeleteComponent, LowestScope: ScopeComponent, IsInternal: false},

	// ComponentRelease
	{Name: ActionViewComponentRelease, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionCreateComponentRelease, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionDeleteComponentRelease, LowestScope: ScopeComponent, IsInternal: false},

	// ReleaseBinding
	{Name: ActionViewReleaseBinding, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionCreateReleaseBinding, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionUpdateReleaseBinding, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionDeleteReleaseBinding, LowestScope: ScopeComponent, IsInternal: false},

	// ComponentType
	{Name: ActionViewComponentType, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateComponentType, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateComponentType, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteComponentType, LowestScope: ScopeNamespace, IsInternal: false},

	// Workflow
	{Name: ActionViewWorkflow, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateWorkflow, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateWorkflow, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteWorkflow, LowestScope: ScopeNamespace, IsInternal: false},

	// WorkflowRun (dynamic scope: namespace,or component depending on query context)
	{Name: ActionCreateWorkflowRun, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionViewWorkflowRun, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionUpdateWorkflowRun, LowestScope: ScopeComponent, IsInternal: false},

	// Trait
	{Name: ActionViewTrait, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateTrait, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateTrait, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteTrait, LowestScope: ScopeNamespace, IsInternal: false},

	// Environment
	{Name: ActionViewEnvironment, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateEnvironment, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateEnvironment, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteEnvironment, LowestScope: ScopeNamespace, IsInternal: false},

	// DataPlane
	{Name: ActionViewDataPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateDataPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateDataPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteDataPlane, LowestScope: ScopeNamespace, IsInternal: false},

	// WorkflowPlane
	{Name: ActionViewWorkflowPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateWorkflowPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateWorkflowPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteWorkflowPlane, LowestScope: ScopeNamespace, IsInternal: false},

	// ObservabilityPlane
	{Name: ActionViewObservabilityPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateObservabilityPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateObservabilityPlane, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteObservabilityPlane, LowestScope: ScopeNamespace, IsInternal: false},

	// ClusterComponentType
	{Name: ActionViewClusterComponentType, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionCreateClusterComponentType, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionUpdateClusterComponentType, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionDeleteClusterComponentType, LowestScope: ScopeCluster, IsInternal: false},

	// ClusterTrait
	{Name: ActionViewClusterTrait, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionCreateClusterTrait, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionUpdateClusterTrait, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionDeleteClusterTrait, LowestScope: ScopeCluster, IsInternal: false},

	// ClusterWorkflow
	{Name: ActionViewClusterWorkflow, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionCreateClusterWorkflow, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionUpdateClusterWorkflow, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionDeleteClusterWorkflow, LowestScope: ScopeCluster, IsInternal: false},

	// ClusterDataPlane
	{Name: ActionViewClusterDataPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionCreateClusterDataPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionUpdateClusterDataPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionDeleteClusterDataPlane, LowestScope: ScopeCluster, IsInternal: false},

	// ClusterWorkflowPlane
	{Name: ActionViewClusterWorkflowPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionCreateClusterWorkflowPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionUpdateClusterWorkflowPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionDeleteClusterWorkflowPlane, LowestScope: ScopeCluster, IsInternal: false},

	// ClusterObservabilityPlane
	{Name: ActionViewClusterObservabilityPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionCreateClusterObservabilityPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionUpdateClusterObservabilityPlane, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionDeleteClusterObservabilityPlane, LowestScope: ScopeCluster, IsInternal: false},

	// DeploymentPipeline
	{Name: ActionViewDeploymentPipeline, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateDeploymentPipeline, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateDeploymentPipeline, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteDeploymentPipeline, LowestScope: ScopeNamespace, IsInternal: false},

	// ObservabilityAlertsNotificationChannel
	{Name: ActionViewObservabilityAlertsNotificationChannel, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateObservabilityAlertsNotificationChannel, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateObservabilityAlertsNotificationChannel, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteObservabilityAlertsNotificationChannel, LowestScope: ScopeNamespace, IsInternal: false},

	// SecretReference
	{Name: ActionViewSecretReference, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateSecretReference, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateSecretReference, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteSecretReference, LowestScope: ScopeNamespace, IsInternal: false},

	// Workload
	{Name: ActionViewWorkload, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionCreateWorkload, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionUpdateWorkload, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionDeleteWorkload, LowestScope: ScopeComponent, IsInternal: false},

	// ClusterAuthzRole
	{Name: ActionViewClusterAuthzRole, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionCreateClusterAuthzRole, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionUpdateClusterAuthzRole, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionDeleteClusterAuthzRole, LowestScope: ScopeCluster, IsInternal: false},

	// AuthzRole
	{Name: ActionViewAuthzRole, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateAuthzRole, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateAuthzRole, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteAuthzRole, LowestScope: ScopeNamespace, IsInternal: false},

	// ClusterAuthzRoleBinding
	{Name: ActionViewClusterAuthzRoleBinding, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionCreateClusterAuthzRoleBinding, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionUpdateClusterAuthzRoleBinding, LowestScope: ScopeCluster, IsInternal: false},
	{Name: ActionDeleteClusterAuthzRoleBinding, LowestScope: ScopeCluster, IsInternal: false},

	// AuthzRoleBinding
	{Name: ActionViewAuthzRoleBinding, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionCreateAuthzRoleBinding, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionUpdateAuthzRoleBinding, LowestScope: ScopeNamespace, IsInternal: false},
	{Name: ActionDeleteAuthzRoleBinding, LowestScope: ScopeNamespace, IsInternal: false},

	// logs (dynamic scope: namespace or component depending on query)
	{Name: ActionViewLogs, LowestScope: ScopeComponent, IsInternal: false},

	// metrics (dynamic scope: namespace or component depending on query)
	{Name: ActionViewMetrics, LowestScope: ScopeComponent, IsInternal: false},

	// traces (dynamic scope: namespace or project depending on query)
	{Name: ActionViewTraces, LowestScope: ScopeProject, IsInternal: false},

	// alerts (dynamic scope: namespace, project, or component depending on query)
	{Name: ActionViewAlerts, LowestScope: ScopeComponent, IsInternal: false},

	// incidents (dynamic scope: namespace, project, or component depending on query)
	{Name: ActionViewIncidents, LowestScope: ScopeComponent, IsInternal: false},
	{Name: ActionUpdateIncidents, LowestScope: ScopeComponent, IsInternal: false},

	// RCA Report
	{Name: ActionViewRCAReport, LowestScope: ScopeProject, IsInternal: false},
	{Name: ActionUpdateRCAReport, LowestScope: ScopeProject, IsInternal: false},
}

// AllActions returns all system-defined actions
func AllActions() []Action {
	return systemActions
}

// PublicActions returns all public (non-internal) actions, sorted by name
func PublicActions() []Action {
	actions := make([]Action, 0)
	for _, action := range systemActions {
		if !action.IsInternal {
			actions = append(actions, action)
		}
	}

	// Sort by action name
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Name < actions[j].Name
	})

	return actions
}

// ConcretePublicActions returns only concrete (non-wildcarded) public actions, sorted by name
func ConcretePublicActions() []Action {
	actions := make([]Action, 0)
	for _, action := range systemActions {
		// exclude wildcarded actions (containing *) and internal actions
		if !action.IsInternal && !strings.Contains(action.Name, "*") {
			actions = append(actions, action)
		}
	}

	// Sort by action name
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Name < actions[j].Name
	})

	return actions
}
