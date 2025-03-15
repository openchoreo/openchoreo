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

package messages

const (
	// CLI configuration

	DefaultCLIName             = "choreoctl"
	DefaultCLIShortDescription = "Welcome to Choreo CLI, " +
		"the command-line interface for Open Source Choreo - Internal Developer Platform"

	// Common prefix for errors

	ErrorPrefix = "Error: "

	// Apply command error messages with hints

	ErrFileRequired = "no file specified\n" +
		"hint: use -f or --file flag to specify the configuration file\n" +
		"See 'choreoctl apply --help' for usage"
	ErrFileNotFound   = "file not found at '%s'\nhint: verify the file path exists"
	ErrFilePermission = "permission denied accessing '%s'\nhint: check file permissions with 'ls -l'"
	ErrApplyFailed    = "failed to apply configuration from '%s': %v\n" +
		"hint: validate YAML syntax and resource specifications"

	// Apply command success messages

	SuccessApplyMsg = "✓ Successfully applied configuration from '%s'\nUse 'choreoctl get' commands to view resources"

	// Flag descriptions with examples

	KubeconfigFlagDesc         = "Path to the kubeconfig file (e.g., ~/.kube/config)"
	KubecontextFlagDesc        = "Name of the kubeconfig context (e.g., minikube)"
	ApplyFileFlag              = "Path to the configuration file to apply (e.g., manifests/deployment.yaml)"
	FlagOrgDesc                = "Name of the organization (e.g., acme-corp)"
	FlagProjDesc               = "Name of the project (e.g., online-store)"
	FlagNameDesc               = "Name of the resource (must be lowercase letters, numbers, or hyphens)"
	FlagURLDesc                = "URL of the git repository (e.g., https://github.com/acme-corp/product-catalog)"
	FlagSecretRefDesc          = "Secret reference for git authentication (e.g., github-token)"
	FlagOutputDesc             = "Output format [yaml]"
	FlagDisplayDesc            = "Display name for the component (e.g., \"Product Catalog\")"
	FlagDescriptionDesc        = "Brief description of the organization's purpose"
	FlagTypeDesc               = "Type of the component [WebApplication|ScheduledTask|Service]"
	FlagLogTypeDesc            = "Type of the log [deployment, build]"
	FlagBuildDesc              = "Name of the build (e.g., product-catalog-build-01)"
	FlagCompDesc               = "Name of the component (e.g., product-catalog)"
	FlagTailDesc               = "Number of lines to show from the end of logs"
	FlagFollowDesc             = "Follow the logs of the specified resource"
	FlagBuildTypeDesc          = "Type of the build [docker|buildpack]"
	FlagDockerContext          = "Path to the Docker build context directory"
	FlagDockerfilePath         = "Path to the Dockerfile"
	FlagBuildpackName          = "Name of the buildpack"
	FlagBuildpackVersion       = "Version of the buildpack"
	FlagBranchDesc             = "Name of the Git branch"
	FlagPathDesc               = "Path to the source code directory"
	FlagAutoBuildDesc          = "Enable automatic builds"
	FlagRevisionDesc           = "Git commit hash"
	FlagDeploymentTrackrDesc   = "Deployment track for the component [main|feature|bugfix]"
	FlagDockerImageDesc        = "Name of the Docker image (e.g., product-catalog:latest)"
	FlagEnvironmentDesc        = "Environment where the component will be deployed (e.g., dev, staging, production)"
	FlagDeployableArtifactDesc = "Deployable artifact name (e.g., product-catalog-artifact)"
	FlagDeploymentDesc         = "Name of the deployment (e.g., product-catalog-dev-01)"
	DeleteFileFlag             = "Path to the configuration file to delete (e.g., manifests/deployment.yaml)"
	FlagWaitDesc               = "Wait for resources to be deleted before returning"
	FlagEnvironmentOrderDesc   = "Comma-separated list of environment names in promotion order (e.g., dev,staging,prod)"
)
