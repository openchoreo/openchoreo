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
	{Name: "namespace:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "namespace:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "namespace:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "namespace:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// Project
	{Name: "project:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "project:view", LowestScope: ScopeProject, IsInternal: false},
	{Name: "project:update", LowestScope: ScopeProject, IsInternal: false},
	{Name: "project:delete", LowestScope: ScopeProject, IsInternal: false},

	// Component
	{Name: "component:create", LowestScope: ScopeProject, IsInternal: false},
	{Name: "component:view", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "component:update", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "component:delete", LowestScope: ScopeComponent, IsInternal: false},

	// ComponentRelease
	{Name: "componentrelease:view", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "componentrelease:create", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "componentrelease:delete", LowestScope: ScopeComponent, IsInternal: false},

	// ReleaseBinding
	{Name: "releasebinding:view", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "releasebinding:create", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "releasebinding:update", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "releasebinding:delete", LowestScope: ScopeComponent, IsInternal: false},

	// ComponentType
	{Name: "componenttype:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "componenttype:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "componenttype:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "componenttype:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// Workflow
	{Name: "workflow:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "workflow:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "workflow:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "workflow:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// WorkflowRun (dynamic scope: namespace,or component depending on query context)
	{Name: "workflowrun:create", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "workflowrun:view", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "workflowrun:update", LowestScope: ScopeComponent, IsInternal: false},

	// Trait
	{Name: "trait:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "trait:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "trait:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "trait:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// Environment
	{Name: "environment:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "environment:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "environment:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "environment:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// DataPlane
	{Name: "dataplane:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "dataplane:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "dataplane:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "dataplane:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// WorkflowPlane
	{Name: "workflowplane:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "workflowplane:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "workflowplane:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "workflowplane:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// ObservabilityPlane
	{Name: "observabilityplane:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityplane:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityplane:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityplane:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// ClusterComponentType
	{Name: "clustercomponenttype:view", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clustercomponenttype:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clustercomponenttype:update", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clustercomponenttype:delete", LowestScope: ScopeCluster, IsInternal: false},

	// ClusterTrait
	{Name: "clustertrait:view", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clustertrait:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clustertrait:update", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clustertrait:delete", LowestScope: ScopeCluster, IsInternal: false},

	// ClusterWorkflow
	{Name: "clusterworkflow:view", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflow:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflow:update", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflow:delete", LowestScope: ScopeCluster, IsInternal: false},

	// ClusterDataPlane
	{Name: "clusterdataplane:view", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterdataplane:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterdataplane:update", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterdataplane:delete", LowestScope: ScopeCluster, IsInternal: false},

	// ClusterWorkflowPlane
	{Name: "clusterworkflowplane:view", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflowplane:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflowplane:update", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflowplane:delete", LowestScope: ScopeCluster, IsInternal: false},

	// ClusterObservabilityPlane
	{Name: "clusterobservabilityplane:view", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterobservabilityplane:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterobservabilityplane:update", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterobservabilityplane:delete", LowestScope: ScopeCluster, IsInternal: false},

	// DeploymentPipeline
	{Name: "deploymentpipeline:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "deploymentpipeline:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "deploymentpipeline:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "deploymentpipeline:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// ObservabilityAlertsNotificationChannel
	{Name: "observabilityalertsnotificationchannel:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityalertsnotificationchannel:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityalertsnotificationchannel:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityalertsnotificationchannel:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// SecretReference
	{Name: "secretreference:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "secretreference:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "secretreference:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "secretreference:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// Workload
	{Name: "workload:view", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "workload:create", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "workload:update", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "workload:delete", LowestScope: ScopeComponent, IsInternal: false},

	// ClusterAuthzRole
	{Name: "clusterauthzrole:view", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterauthzrole:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterauthzrole:update", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterauthzrole:delete", LowestScope: ScopeCluster, IsInternal: false},

	// AuthzRole
	{Name: "authzrole:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "authzrole:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "authzrole:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "authzrole:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// ClusterAuthzRoleBinding
	{Name: "clusterauthzrolebinding:view", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterauthzrolebinding:create", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterauthzrolebinding:update", LowestScope: ScopeCluster, IsInternal: false},
	{Name: "clusterauthzrolebinding:delete", LowestScope: ScopeCluster, IsInternal: false},

	// AuthzRoleBinding
	{Name: "authzrolebinding:view", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "authzrolebinding:create", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "authzrolebinding:update", LowestScope: ScopeNamespace, IsInternal: false},
	{Name: "authzrolebinding:delete", LowestScope: ScopeNamespace, IsInternal: false},

	// logs (dynamic scope: namespace or component depending on query)
	{Name: "logs:view", LowestScope: ScopeComponent, IsInternal: false},

	// metrics (dynamic scope: namespace or component depending on query)
	{Name: "metrics:view", LowestScope: ScopeComponent, IsInternal: false},

	// traces (dynamic scope: namespace or project depending on query)
	{Name: "traces:view", LowestScope: ScopeProject, IsInternal: false},

	// alerts (dynamic scope: namespace, project, or component depending on query)
	{Name: "alerts:view", LowestScope: ScopeComponent, IsInternal: false},

	// incidents (dynamic scope: namespace, project, or component depending on query)
	{Name: "incidents:view", LowestScope: ScopeComponent, IsInternal: false},
	{Name: "incidents:update", LowestScope: ScopeComponent, IsInternal: false},

	// RCA Report
	{Name: "rcareport:view", LowestScope: ScopeProject, IsInternal: false},
	{Name: "rcareport:update", LowestScope: ScopeProject, IsInternal: false},
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
