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

package constants

const (
	ChoreoGroup = "core.choreo.dev"
)

const (
	Docker    = "docker"
	Buildpack = "buildpack"
)

const (
	LabelOrganization    = "core.choreo.dev/organization"
	LabelProject         = "core.choreo.dev/project"
	LabelComponent       = "core.choreo.dev/component"
	LabelBuild           = "core.choreo.dev/build"
	LabelName            = "core.choreo.dev/name"
	LabelType            = "core.choreo.dev/type"
	LabelVersion         = "core.choreo.dev/version"
	LabelArtifact        = "core.choreo.dev/deployment-artifact"
	LabelDeployment      = "core.choreo.dev/deployment"
	LabelEnvironment     = "core.choreo.dev/environment"
	LabelDeploymentTrack = "core.choreo.dev/deployment-track"
)
const (
	AnnotationDescription = "core.choreo.dev/description"
	AnnotationDisplayName = "core.choreo.dev/display-name"
)

type APIVersion string

const (
	V1 APIVersion = "v1"
)

const (
	OutputFormatYAML = "yaml"
	OrganizationKind = "Organization"
	ProjectKind      = "Project"
	ComponentKind    = "Component"
)

type CRDConfig struct {
	Group   string
	Version APIVersion
	Kind    string
}

var (
	OrganizationV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    OrganizationKind,
	}
	ProjectV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    ProjectKind,
	}
	ComponentV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    ComponentKind,
	}
	BuildV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    "Build",
	}
	DeployableArtifactV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    "DeployableArtifact",
	}
	DeploymentV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    "Deployment",
	}
	DataPlaneV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    "DataPlane",
	}
	DeploymentTrackV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    "DeploymentTrack",
	}
	EndpointV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    "Endpoint",
	}
	EnvironmentV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1,
		Kind:    "Environment",
	}
	DeploymentPipelineV1Config = CRDConfig{
		Group:   "core.choreo.dev",
		Version: "v1",
		Kind:    "DeploymentPipeline",
	}
)
