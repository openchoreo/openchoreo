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
	// Scope indicates the lowest resource hierarchy level at which this action is evaluated
	Scope ActionScope
	// IsInternal indicates if the action is internal (not publicly visible)
	IsInternal bool
}

// systemActions defines all available actions in the system
var systemActions = []Action{
	// Namespace
	{Name: "namespace:create", Scope: ScopeCluster, IsInternal: false},
	{Name: "namespace:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "namespace:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "namespace:delete", Scope: ScopeNamespace, IsInternal: false},

	// Project
	{Name: "project:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "project:view", Scope: ScopeProject, IsInternal: false},
	{Name: "project:update", Scope: ScopeProject, IsInternal: false},
	{Name: "project:delete", Scope: ScopeProject, IsInternal: false},

	// Component
	{Name: "component:create", Scope: ScopeProject, IsInternal: false},
	{Name: "component:view", Scope: ScopeComponent, IsInternal: false},
	{Name: "component:update", Scope: ScopeComponent, IsInternal: false},
	{Name: "component:delete", Scope: ScopeComponent, IsInternal: false},

	// ComponentRelease
	{Name: "componentrelease:view", Scope: ScopeComponent, IsInternal: false},
	{Name: "componentrelease:create", Scope: ScopeComponent, IsInternal: false},

	// ReleaseBinding
	{Name: "releasebinding:view", Scope: ScopeComponent, IsInternal: false},
	{Name: "releasebinding:create", Scope: ScopeComponent, IsInternal: false},
	{Name: "releasebinding:update", Scope: ScopeComponent, IsInternal: false},
	{Name: "releasebinding:delete", Scope: ScopeComponent, IsInternal: false},

	// ComponentType
	{Name: "componenttype:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "componenttype:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "componenttype:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "componenttype:delete", Scope: ScopeNamespace, IsInternal: false},

	// Workflow
	{Name: "workflow:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "workflow:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "workflow:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "workflow:delete", Scope: ScopeNamespace, IsInternal: false},

	// WorkflowRun (dynamic scope: namespace,or component depending on query context)
	{Name: "workflowrun:create", Scope: ScopeComponent, IsInternal: false},
	{Name: "workflowrun:view", Scope: ScopeComponent, IsInternal: false},
	{Name: "workflowrun:update", Scope: ScopeComponent, IsInternal: false},

	// Trait
	{Name: "trait:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "trait:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "trait:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "trait:delete", Scope: ScopeNamespace, IsInternal: false},

	// Environment
	{Name: "environment:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "environment:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "environment:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "environment:delete", Scope: ScopeNamespace, IsInternal: false},

	// DataPlane
	{Name: "dataplane:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "dataplane:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "dataplane:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "dataplane:delete", Scope: ScopeNamespace, IsInternal: false},

	// WorkflowPlane
	{Name: "workflowplane:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "workflowplane:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "workflowplane:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "workflowplane:delete", Scope: ScopeNamespace, IsInternal: false},

	// ObservabilityPlane
	{Name: "observabilityplane:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityplane:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityplane:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityplane:delete", Scope: ScopeNamespace, IsInternal: false},

	// ClusterComponentType
	{Name: "clustercomponenttype:view", Scope: ScopeCluster, IsInternal: false},
	{Name: "clustercomponenttype:create", Scope: ScopeCluster, IsInternal: false},
	{Name: "clustercomponenttype:update", Scope: ScopeCluster, IsInternal: false},
	{Name: "clustercomponenttype:delete", Scope: ScopeCluster, IsInternal: false},

	// ClusterTrait
	{Name: "clustertrait:view", Scope: ScopeCluster, IsInternal: false},
	{Name: "clustertrait:create", Scope: ScopeCluster, IsInternal: false},
	{Name: "clustertrait:update", Scope: ScopeCluster, IsInternal: false},
	{Name: "clustertrait:delete", Scope: ScopeCluster, IsInternal: false},

	// ClusterWorkflow
	{Name: "clusterworkflow:view", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflow:create", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflow:update", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflow:delete", Scope: ScopeCluster, IsInternal: false},

	// ClusterDataPlane
	{Name: "clusterdataplane:view", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterdataplane:create", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterdataplane:update", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterdataplane:delete", Scope: ScopeCluster, IsInternal: false},

	// ClusterWorkflowPlane
	{Name: "clusterworkflowplane:view", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflowplane:create", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflowplane:update", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterworkflowplane:delete", Scope: ScopeCluster, IsInternal: false},

	// ClusterObservabilityPlane
	{Name: "clusterobservabilityplane:view", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterobservabilityplane:create", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterobservabilityplane:update", Scope: ScopeCluster, IsInternal: false},
	{Name: "clusterobservabilityplane:delete", Scope: ScopeCluster, IsInternal: false},

	// DeploymentPipeline
	{Name: "deploymentpipeline:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "deploymentpipeline:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "deploymentpipeline:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "deploymentpipeline:delete", Scope: ScopeNamespace, IsInternal: false},

	// ObservabilityAlertsNotificationChannel
	{Name: "observabilityalertsnotificationchannel:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityalertsnotificationchannel:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityalertsnotificationchannel:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "observabilityalertsnotificationchannel:delete", Scope: ScopeNamespace, IsInternal: false},

	// SecretReference
	{Name: "secretreference:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "secretreference:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "secretreference:update", Scope: ScopeNamespace, IsInternal: false},
	{Name: "secretreference:delete", Scope: ScopeNamespace, IsInternal: false},

	// Workload
	{Name: "workload:view", Scope: ScopeComponent, IsInternal: false},
	{Name: "workload:create", Scope: ScopeComponent, IsInternal: false},
	{Name: "workload:update", Scope: ScopeComponent, IsInternal: false},
	{Name: "workload:delete", Scope: ScopeComponent, IsInternal: false},

	// roles
	{Name: "role:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "role:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "role:delete", Scope: ScopeNamespace, IsInternal: false},
	{Name: "role:update", Scope: ScopeNamespace, IsInternal: false},

	// role mapping
	{Name: "rolemapping:view", Scope: ScopeNamespace, IsInternal: false},
	{Name: "rolemapping:create", Scope: ScopeNamespace, IsInternal: false},
	{Name: "rolemapping:delete", Scope: ScopeNamespace, IsInternal: false},
	{Name: "rolemapping:update", Scope: ScopeNamespace, IsInternal: false},

	// logs (dynamic scope: namespace or component depending on query)
	{Name: "logs:view", Scope: ScopeComponent, IsInternal: false},

	// metrics (dynamic scope: namespace or component depending on query)
	{Name: "metrics:view", Scope: ScopeComponent, IsInternal: false},

	// traces (dynamic scope: namespace or project depending on query)
	{Name: "traces:view", Scope: ScopeProject, IsInternal: false},

	// alerts (dynamic scope: namespace, project, or component depending on query)
	{Name: "alerts:view", Scope: ScopeComponent, IsInternal: false},

	// incidents (dynamic scope: namespace, project, or component depending on query)
	{Name: "incidents:view", Scope: ScopeComponent, IsInternal: false},
	{Name: "incidents:update", Scope: ScopeComponent, IsInternal: false},

	// RCA Report
	{Name: "rcareport:view", Scope: ScopeProject, IsInternal: false},
	{Name: "rcareport:update", Scope: ScopeProject, IsInternal: false},
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
