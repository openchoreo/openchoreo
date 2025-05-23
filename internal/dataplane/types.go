/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package dataplane

import (
	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// DeploymentContext is a struct that holds the all necessary data required for the resource handlers to
// perform their operations.
type DeploymentContext struct {
	Project            *choreov1.Project
	Component          *choreov1.Component
	DeploymentTrack    *choreov1.DeploymentTrack
	Build              *choreov1.Build
	DeployableArtifact *choreov1.DeployableArtifact
	Deployment         *choreov1.Deployment
	Environment        *choreov1.Environment

	ConfigurationGroups []*choreov1.ConfigurationGroup

	ContainerImage string
}

// EndpointContext is a struct that holds the all necessary data required for the resource handlers to perform their operations.
type EndpointContext struct {
	Project         *choreov1.Project
	DataPlane       *choreov1.DataPlane
	Component       *choreov1.Component
	DeploymentTrack *choreov1.DeploymentTrack
	Deployment      *choreov1.Deployment
	Environment     *choreov1.Environment
	Endpoint        *choreov1.Endpoint
}

// ProjectContext is a struct that holds the all necessary data required for the resource handlers to perform their operations.
type ProjectContext struct {
	DeploymentPipeline *choreov1.DeploymentPipeline
	Project            *choreov1.Project
	EnvironmentNames   []string
	NamespaceNames     []string
}

// EnvironmentContext is a struct that holds the all necessary data required for the resource handlers of the environment
// to perform its operations.
type EnvironmentContext struct {
	Environment *choreov1.Environment
	DataPlane   *choreov1.DataPlane
}
