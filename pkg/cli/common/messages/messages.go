// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package messages

const (
	// CLI configuration

	DefaultCLIName             = "occ"
	DefaultCLIShortDescription = "Welcome to Choreo CLI, " +
		"the command-line interface for OpenChoreo - Internal Developer Platform"

	// Flag descriptions with examples

	ApplyFileFlag          = "Path to the configuration file to apply (e.g., manifests/deployment.yaml)"
	FlagNamespaceDesc      = "Name of the namespace (e.g., acme-corp)"
	FlagProjDesc           = "Name of the project (e.g., online-store)"
	FlagNameDesc           = "Name of the resource (must be lowercase letters, numbers, or hyphens)"
	FlagOutputDesc         = "Output format [yaml]"
	FlagCompDesc           = "Name of the component (e.g., product-catalog)"
	FlagTailDesc           = "Number of lines to show from the end of logs"
	FlagFollowDesc         = "Follow the logs of the specified resource"
	FlagDockerImageDesc    = "Name of the Docker image (e.g., product-catalog:latest)"
	FlagEnvironmentDesc    = "Environment where the component will be deployed (e.g., dev, staging, production)"
	FlagDeploymentDesc     = "Name of the deployment (e.g., product-catalog-dev-01)"
	WorkloadDescriptorFlag = "Path to the workload descriptor file (e.g., workload.yaml)"
)
