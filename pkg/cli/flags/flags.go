// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/messages"
)

type Flag struct {
	Name      string
	Shorthand string
	Usage     string
	Alias     string
	Type      string
}

var (
	Kubeconfig = Flag{
		Name:  "kubeconfig",
		Usage: messages.KubeconfigFlagDesc,
	}

	Kubecontext = Flag{
		Name:  "kubecontext",
		Usage: messages.KubecontextFlagDesc,
	}

	Organization = Flag{
		Name:  "organization",
		Usage: messages.FlagOrgDesc,
		Alias: "org",
	}

	Project = Flag{
		Name:  "project",
		Usage: messages.FlagProjDesc,
	}

	Component = Flag{
		Name:  "component",
		Usage: messages.FlagCompDesc,
	}
	Build = Flag{
		Name:  "build",
		Usage: messages.FlagBuildDesc,
	}
	Environment = Flag{
		Name:  "environment",
		Usage: messages.FlagEnvironmentDesc,
	}
	Deployment = Flag{
		Name:  "deployment",
		Usage: messages.FlagDeploymentDesc,
	}

	DeploymentTrack = Flag{
		Name:  "deployment-track",
		Usage: messages.FlagDeploymentTrackrDesc,
	}
	Image = Flag{
		Name:  "image",
		Usage: messages.FlagDockerImageDesc,
	}
	Name = Flag{
		Name:  "name",
		Usage: messages.FlagNameDesc,
	}

	GitRepositoryURL = Flag{
		Name:  "git-repository-url",
		Usage: messages.FlagURLDesc,
	}

	SecretRef = Flag{
		Name:  "secretRef",
		Usage: messages.FlagSecretRefDesc,
	}

	ComponentType = Flag{
		Name:  "type",
		Usage: messages.FlagTypeDesc,
	}

	Output = Flag{
		Name:      "output",
		Shorthand: "o", // Keep shorthand for output as it's a common convention
		Usage:     messages.FlagOutputDesc,
	}

	DisplayName = Flag{
		Name:  "display-name",
		Usage: messages.FlagDisplayDesc,
	}

	Description = Flag{
		Name:  "description",
		Usage: messages.FlagDescriptionDesc,
	}

	ApplyFileFlag = Flag{
		Name:      "file",
		Shorthand: "f",
		Usage:     messages.ApplyFileFlag,
	}

	LogType = Flag{
		Name:  "type",
		Usage: messages.FlagLogTypeDesc,
	}

	Tail = Flag{
		Name:  "tail",
		Usage: messages.FlagTailDesc,
	}
	Follow = Flag{
		Name:  "follow",
		Usage: messages.FlagFollowDesc,
		Type:  "bool",
	}
	BuildTypeName = Flag{
		Name:  "type",
		Usage: messages.FlagBuildTypeDesc,
	}

	DockerContext = Flag{
		Name:  "docker-context",
		Usage: messages.FlagDockerContext,
	}
	DockerfilePath = Flag{
		Name:  "dockerfile-path",
		Usage: messages.FlagDockerfilePath,
	}
	BuildpackName = Flag{
		Name:  "buildpack-name",
		Usage: messages.FlagBuildpackName,
	}
	BuildpackVersion = Flag{
		Name:  "buildpack-version",
		Usage: messages.FlagBuildpackVersion,
	}

	Revision = Flag{
		Name:  "revision",
		Usage: messages.FlagRevisionDesc,
	}
	Branch = Flag{
		Name:  "branch",
		Usage: messages.FlagBranchDesc,
	}

	Path = Flag{
		Name:  "path",
		Usage: messages.FlagPathDesc,
	}

	AutoBuild = Flag{
		Name:  "auto-build",
		Usage: messages.FlagAutoBuildDesc,
	}

	DeployableArtifact = Flag{
		Name:  "deployableartifact",
		Usage: messages.FlagDeployableArtifactDesc,
	}

	ConnectionConfigRef = Flag{
		Name:  "connection-config",
		Usage: "Reference to the connection configuration",
	}

	PublicVirtualHost = Flag{
		Name:  "public-virtual-host",
		Usage: "Public virtual host for the gateway",
	}

	OrgVirtualHost = Flag{
		Name:  "org-virtual-host",
		Usage: "Organization virtual host for the gateway",
	}

	ClusterAgentClientCA = Flag{
		Name:  "cluster-agent-client-ca",
		Usage: "The CA certificate used to verify the cluster agent's client certificate",
	}

	EndpointType = Flag{
		Name:  "type",
		Usage: "Type of the endpoint (HTTP, REST, gRPC, GraphQL, Websocket, TCP, UDP)",
	}

	Port = Flag{
		Name:  "port",
		Usage: "Port number for the service",
	}

	DataPlaneRef = Flag{
		Name:  "dataplane-ref",
		Usage: "Reference to the data plane",
	}

	IsProduction = Flag{
		Name:  "production",
		Usage: "Whether this is a production environment",
		Type:  "bool",
	}

	DNSPrefix = Flag{
		Name:  "dns-prefix",
		Usage: "DNS prefix for the environment",
	}

	APIVersion = Flag{
		Name:  "api-version",
		Usage: "API version for the deployment track",
	}

	AutoDeploy = Flag{
		Name:  "auto-deploy",
		Usage: "Enable automatic deployments",
	}

	DataPlane = Flag{
		Name:  "dataplane",
		Usage: "Name of the Data plane",
	}
	KubeconfigPath = Flag{
		Name:  "kubeconfig",
		Usage: "Path to the kubeconfig file",
	}
	KubeContext = Flag{
		Name:  "kube-context",
		Usage: "Name of the kubeconfig context to use",
	}

	Wait = Flag{
		Name:      "wait",
		Shorthand: "w",
		Usage:     messages.FlagWaitDesc,
		Type:      "bool",
	}

	DeleteFileFlag = Flag{
		Name:      "file",
		Shorthand: "f",
		Usage:     messages.DeleteFileFlag,
	}

	WorkloadDescriptor = Flag{
		Name:  "descriptor",
		Usage: messages.WorkloadDescriptorFlag,
	}

	EnvironmentOrder = Flag{
		Name:  "environment-order",
		Usage: messages.FlagEnvironmentOrderDesc,
	}

	DeploymentPipeline = Flag{
		Name:  "deployment-pipeline",
		Usage: messages.FlagDeploymentPipelineDesc,
	}

	// Control plane configuration flags

	Endpoint = Flag{
		Name:  "endpoint",
		Usage: "OpenChoreo API server endpoint URL",
	}

	Token = Flag{
		Name:  "token",
		Usage: "Authentication token for remote OpenChoreo API server",
	}

	// Scaffold-specific flags

	ScaffoldType = Flag{
		Name:  "type",
		Usage: "Component type in format workloadType/componentTypeName (e.g., deployment/web-app)",
	}

	Traits = Flag{
		Name:  "traits",
		Usage: "Comma-separated list of trait names to include",
	}

	Workflow = Flag{
		Name:  "workflow",
		Usage: "ComponentWorkflow name to include in the scaffold",
	}

	SkipComments = Flag{
		Name:  "skip-comments",
		Usage: "Skip section headers and field description comments for minimal output",
		Type:  "bool",
	}

	SkipOptional = Flag{
		Name:  "skip-optional",
		Usage: "Skip optional fields without defaults (show only required fields)",
		Type:  "bool",
	}

	OutputFile = Flag{
		Name:      "output-file",
		Shorthand: "o",
		Usage:     "Write output to specified file instead of stdout",
	}

	// Mode (context) flags

	Mode = Flag{
		Name:  "mode",
		Usage: "Context mode: 'api-server' (default) or 'file-system'",
	}

	RootDirectoryPath = Flag{
		Name:  "root-directory-path",
		Usage: "Root directory path for file-system mode (defaults to current directory)",
	}

	All = Flag{
		Name:  "all",
		Usage: "Process all resources",
		Type:  "bool",
	}

	OutputPath = Flag{
		Name:  "output-path",
		Usage: "Custom output directory path",
	}

	DryRun = Flag{
		Name:  "dry-run",
		Usage: "Preview changes without writing files",
		Type:  "bool",
	}

	TargetEnv = Flag{
		Name:      "target-env",
		Shorthand: "e",
		Usage:     "Target environment for the release binding",
	}

	UsePipeline = Flag{
		Name:  "use-pipeline",
		Usage: "Deployment pipeline name for environment validation",
	}

	ComponentRelease = Flag{
		Name:  "component-release",
		Usage: "Explicit component release name (only valid with --project and --component)",
	}

	// Authentication flags

	ClientCredentials = Flag{
		Name:  "client-credentials",
		Usage: "Use OAuth2 client credentials flow for authentication",
		Type:  "bool",
	}

	ClientID = Flag{
		Name:  "client-id",
		Usage: "OAuth2 client ID for service account authentication",
	}

	ClientSecret = Flag{
		Name:  "client-secret",
		Usage: "OAuth2 client secret for service account authentication",
	}

	CredentialName = Flag{
		Name:  "credential",
		Usage: "Name to save the credential as in config",
	}
)

// AddFlags adds the specified flags to the given command.
func AddFlags(cmd *cobra.Command, flags ...Flag) {
	for _, flag := range flags {
		if flag.Type == "bool" {
			cmd.Flags().BoolP(flag.Name, flag.Shorthand, false, flag.Usage)
		} else {
			// Default to string type
			cmd.Flags().StringP(flag.Name, flag.Shorthand, "", flag.Usage)
		}
	}
}
