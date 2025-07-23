// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package constants

const (
	ChoreoGroup = "openchoreo.dev"
)

const (
	Docker    = "docker"
	Buildpack = "buildpack"
)

const (
	LabelOrganization    = "openchoreo.dev/organization"
	LabelProject         = "openchoreo.dev/project"
	LabelComponent       = "openchoreo.dev/component"
	LabelBuild           = "openchoreo.dev/build"
	LabelName            = "openchoreo.dev/name"
	LabelType            = "openchoreo.dev/type"
	LabelVersion         = "openchoreo.dev/version"
	LabelArtifact        = "openchoreo.dev/deployment-artifact"
	LabelDeployment      = "openchoreo.dev/deployment"
	LabelEnvironment     = "openchoreo.dev/environment"
	LabelDeploymentTrack = "openchoreo.dev/deployment-track"
)
const (
	AnnotationDescription = "openchoreo.dev/description"
	AnnotationDisplayName = "openchoreo.dev/display-name"
)

type APIVersion string

const (
	V1 APIVersion = "v1"
	V1Alpha1 APIVersion = "v1alpha1"
)

const (
	OutputFormatYAML = "yaml"
	OrganizationKind = "Organization"
	ProjectKind      = "Project"
	ComponentKind    = "Component"
	WorkloadKind     = "Workload"
)

type CRDConfig struct {
	Group   string
	Version APIVersion
	Kind    string
}

var (
	OrganizationV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    OrganizationKind,
	}
	ProjectV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    ProjectKind,
	}
	ComponentV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    ComponentKind,
	}
	WorkloadV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    WorkloadKind,
	}
	BuildV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "Build",
	}
	DeployableArtifactV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "DeployableArtifact",
	}
	DeploymentV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "Deployment",
	}
	DataPlaneV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "DataPlane",
	}
	DeploymentTrackV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "DeploymentTrack",
	}
	EndpointV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "Endpoint",
	}
	EnvironmentV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "Environment",
	}
	DeploymentPipelineV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "DeploymentPipeline",
	}
	ConfigurationGroupV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "ConfigurationGroup",
	}
)
