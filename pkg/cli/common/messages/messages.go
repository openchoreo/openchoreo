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

import "fmt"

const (
	// CLI configuration

	DefaultCLIName             = "choreoctl"
	DefaultCLIShortDescription = "Welcome to Choreo CLI, " +
		"the command-line interface for Open Source Internal Developer Platform"

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

	SuccessApplyMsg = "✓ Successfully applied configuration from '%s'\nUse 'choreoctl list' commands to view resources"

	// Flag descriptions with examples

	KubeconfigFlagDesc         = "Path to the kubeconfig file (e.g., ~/.kube/config)"
	KubecontextFlagDesc        = "Name of the kubeconfig context (e.g., minikube)"
	ApplyFileFlag              = "Path to the configuration file to apply (e.g., deploy.yaml)"
	FlagOrgDesc                = "Name of the organization (e.g., my-org)"
	FlagProjDesc               = "Name of the project (e.g., my-project)"
	FlagNameDesc               = "Name of the resource (must be lowercase letters, numbers, or hyphens)"
	FlagURLDesc                = "URL of the git repository (e.g., https://github.com/org/repo)"
	FlagSecretRefDesc          = "Secret reference for git authentication (e.g., github-token)"
	FlagOutputDesc             = "Output format [table|yaml]"
	FlagDisplayDesc            = "Human-readable display name for the organization (e.g., \"My Organization\")"
	FlagDescriptionDesc        = "Brief description of the organization's purpose"
	FlagTypeDesc               = "Type of the component [WebApplication|ScheduledTask]"
	FlagLogTypeDesc            = "Type of the log [component-application, component-gateway, project, build]"
	FlagBuildDesc              = "Name of the build (e.g., my-build)"
	FlagCompDesc               = "Name of the component (e.g., my-component)"
	FlagTailDesc               = "Tail the logs of the specified resource"
	FlagFollowDesc             = "Follow the logs of the specified resource"
	FlagBuildTypeDesc          = "Type of the build [docker|buildpack]"
	FlagDockerContext          = "Path to the build context directory"
	FlagDockerfilePath         = "Path to the Dockerfile"
	FlagBuildpackName          = "Name of the buildpack"
	FlagBuildpackVersion       = "Version of the buildpack"
	FlagBranchDesc             = "Git branch name"
	FlagPathDesc               = "Path to the source code"
	FlagAutoBuildDesc          = "Enable auto build"
	FlagRevisionDesc           = "Git commit hash"
	FlagDeploymentTrackrDesc   = "Deployment track for the component [main|feature|bugfix]"
	FlagDockerImageDesc        = "Docker image name (e.g., my-image:latest)"
	FlagEnvironmentDesc        = "Environment to deploy the component (e.g., dev, staging, prod)"
	FlagDeployableArtifactDesc = "Deployable artifact name (e.g., my-artifact)"
	FlagDeploymentDesc         = "Name of the deployment (e.g., my-deployment)"
	DeleteFileFlag             = "Path to the configuration file to delete (e.g., deploy.yaml)"
	FlagWaitDesc               = "Wait for resources to be deleted before returning"
)

type ApplyError struct {
	msg  string
	path string
	err  error
}

func (e *ApplyError) Error() string {
	if e.err != nil {
		return fmt.Sprintf(ErrorPrefix+e.msg, e.path, e.err)
	}
	if e.path != "" {
		return fmt.Sprintf(ErrorPrefix+e.msg, e.path)
	}
	return ErrorPrefix + e.msg
}

func NewFileRequiredError() error {
	return &ApplyError{msg: ErrFileRequired}
}

func NewFileNotFoundError(path string) error {
	return &ApplyError{msg: ErrFileNotFound, path: path}
}

func NewFilePermissionError(path string) error {
	return &ApplyError{msg: ErrFilePermission, path: path}
}

func NewApplyFailedError(path string, err error) error {
	return &ApplyError{msg: ErrApplyFailed, path: path, err: err}
}
