/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
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
