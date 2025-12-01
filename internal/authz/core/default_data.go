// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

// This file contains all default authorization data that is seeded at application startup.
// To add new actions or roles, simply update the arrays below and restart the application.

// ListDefaultActions returns a list of all defined actions in the system
func ListDefaultActions() []string {
	return []string{
		// Organization
		"organization:view",

		// Project
		"project:view",
		"project:create",

		// Component
		"component:view",
		"component:create",
		"component:update",
		"component:deploy",
		"component:promote",

		// ComponentRelease
		"componentrelease:view",
		"componentrelease:create",

		// ReleaseBinding
		"releasebinding:view",
		"releasebinding:update",

		// ComponentBinding
		"componentbinding:view",
		"componentbinding:update",

		// ComponentType
		"componenttype:view",

		// ComponentWorkflow
		"componentworkflow:view",
		"componentworkflow:trigger",

		// Workflow
		"workflow:view",

		// Trait
		"trait:view",

		// Environment
		"environment:view",
		"environment:create",

		// DataPlane
		"dataplane:view",
		"dataplane:create",

		// BuildPlane
		"buildplane:view",

		// DeploymentPipeline
		"deploymentpipeline:view",

		// Schema
		"schema:view",

		// SecretReference
		"secretreference:view",

		// Workload
		"workload:view",
		"workload:create",
	}
}

// ListDefaultRoles returns the default role definitions
func ListDefaultRoles() []Role {
	return []Role{
		{
			Name: "super-admin",
			Actions: []string{
				"*",
			},
		},
	}
}
