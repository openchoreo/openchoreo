// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"sort"
	"strings"
)

// Action represents a system action with metadata
type Action struct {
	Name       string
	IsInternal bool
}

// systemActions defines all available actions in the system
// IsInternal indicates if the action is internal (not publicly visible)
var systemActions = []Action{
	// Namespace
	{Name: "namespace:view", IsInternal: false},
	{Name: "namespace:create", IsInternal: false},

	// Project
	{Name: "project:view", IsInternal: false},
	{Name: "project:create", IsInternal: false},
	{Name: "project:delete", IsInternal: false},

	// Component
	{Name: "component:view", IsInternal: false},
	{Name: "component:create", IsInternal: false},
	{Name: "component:update", IsInternal: false},
	{Name: "component:deploy", IsInternal: false},
	{Name: "component:delete", IsInternal: false},

	// ComponentRelease
	{Name: "componentrelease:view", IsInternal: false},
	{Name: "componentrelease:create", IsInternal: false},

	// ReleaseBinding
	{Name: "releasebinding:view", IsInternal: false},
	{Name: "releasebinding:update", IsInternal: false},

	// ComponentType
	{Name: "componenttype:view", IsInternal: false},
	{Name: "componenttype:create", IsInternal: false},

	// ComponentWorkflow
	{Name: "componentworkflow:view", IsInternal: false},
	{Name: "componentworkflow:create", IsInternal: false},

	// ComponentWorkflowRun
	{Name: "componentworkflowrun:view", IsInternal: false},

	// Workflow
	{Name: "workflow:view", IsInternal: false},

	// Trait
	{Name: "trait:view", IsInternal: false},
	{Name: "trait:create", IsInternal: false},

	// Environment
	{Name: "environment:view", IsInternal: false},
	{Name: "environment:create", IsInternal: false},

	// DataPlane
	{Name: "dataplane:view", IsInternal: false},
	{Name: "dataplane:create", IsInternal: false},

	// BuildPlane
	{Name: "buildplane:view", IsInternal: false},

	// ObservabilityPlane
	{Name: "observabilityplane:view", IsInternal: false},

	// Cluster-scoped planes
	{Name: "clusterdataplane:view", IsInternal: false},
	{Name: "clusterdataplane:create", IsInternal: false},
	{Name: "clusterbuildplane:view", IsInternal: false},
	{Name: "clusterobservabilityplane:view", IsInternal: false},

	// DeploymentPipeline
	{Name: "deploymentpipeline:view", IsInternal: false},

	// SecretReference
	{Name: "secretreference:create", IsInternal: false},
	{Name: "secretreference:view", IsInternal: false},
	{Name: "secretreference:delete", IsInternal: false},

	// Workload
	{Name: "workload:view", IsInternal: false},
	{Name: "workload:create", IsInternal: false},

	// roles
	{Name: "role:view", IsInternal: false},
	{Name: "role:create", IsInternal: false},
	{Name: "role:delete", IsInternal: false},
	{Name: "role:update", IsInternal: false},
	{Name: "action:view", IsInternal: false},

	// role mapping
	{Name: "rolemapping:view", IsInternal: false},
	{Name: "rolemapping:create", IsInternal: false},
	{Name: "rolemapping:delete", IsInternal: false},
	{Name: "rolemapping:update", IsInternal: false},

	// logs
	{Name: "logs:view", IsInternal: false},

	// metrics
	{Name: "metrics:view", IsInternal: false},

	// traces
	{Name: "traces:view", IsInternal: false},

	// alerts
	{Name: "alerts:view", IsInternal: false},

	// RCA Report
	{Name: "rcareport:view", IsInternal: false},
	{Name: "rcareport:update", IsInternal: false},
	{Name: "rcareport:delete", IsInternal: false},
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
