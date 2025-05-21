/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
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
		Group:   ChoreoGroup,
		Version: "v1",
		Kind:    "DeploymentPipeline",
	}
	ConfigurationGroupV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: "v1",
		Kind:    "ConfigurationGroup",
	}
)
