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

package flags

import (
	"github.com/spf13/cobra"

	"github.com/choreo-idp/choreo/pkg/cli/common/messages"
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
	DockerImage = Flag{
		Name:  "docker-image",
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

	KubernetesClusterName = Flag{
		Name:  "cluster-name",
		Usage: "Name of the Kubernetes cluster",
	}

	ConnectionConfigRef = Flag{
		Name:  "connection-config",
		Usage: "Reference to the connection configuration",
	}

	EnableCilium = Flag{
		Name:  "enable-cilium",
		Usage: "Enable Cilium networking",
		Type:  "bool",
	}

	EnableScaleToZero = Flag{
		Name:  "enable-scale-to-zero",
		Usage: "Enable scale to zero functionality",
	}

	GatewayType = Flag{
		Name:  "gateway-type",
		Usage: "Gateway type (e.g., envoy)",
	}

	PublicVirtualHost = Flag{
		Name:  "public-virtual-host",
		Usage: "Public virtual host for the gateway",
	}

	OrgVirtualHost = Flag{
		Name:  "org-virtual-host",
		Usage: "Organization virtual host for the gateway",
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
	Interactive = Flag{
		Name:      "interactive",
		Shorthand: "i",
		Usage:     "Enable interactive mode",
		Type:      "bool",
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

	EnvironmentOrder = Flag{
		Name:  "environment-order",
		Usage: messages.FlagEnvironmentOrderDesc,
	}

	DeploymentPipeline = Flag{
		Name:  "deployment-pipeline",
		Usage: messages.FlagDeploymentPipelineDesc,
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
