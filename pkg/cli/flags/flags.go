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

// Mode constants for --mode flag
const (
	ModeAPIServer  = "api-server"
	ModeFileSystem = "file-system"
)

var (
	Namespace = Flag{
		Name:      "namespace",
		Shorthand: "n",
		Usage:     messages.FlagNamespaceDesc,
	}

	Project = Flag{
		Name:      "project",
		Shorthand: "p",
		Usage:     messages.FlagProjDesc,
	}

	Component = Flag{
		Name:      "component",
		Shorthand: "c",
		Usage:     messages.FlagCompDesc,
	}

	Environment = Flag{
		Name:  "env",
		Usage: messages.FlagEnvironmentDesc,
	}
	Deployment = Flag{
		Name:  "deployment",
		Usage: messages.FlagDeploymentDesc,
	}

	Image = Flag{
		Name:  "image",
		Usage: messages.FlagDockerImageDesc,
	}
	Name = Flag{
		Name:  "name",
		Usage: messages.FlagNameDesc,
	}

	Output = Flag{
		Name:      "output",
		Shorthand: "o", // Keep shorthand for output as it's a common convention
		Usage:     messages.FlagOutputDesc,
	}

	ApplyFileFlag = Flag{
		Name:      "file",
		Shorthand: "f",
		Usage:     messages.ApplyFileFlag,
	}

	Tail = Flag{
		Name:  "tail",
		Usage: messages.FlagTailDesc,
		Type:  "int",
	}
	Follow = Flag{
		Name:      "follow",
		Shorthand: "f",
		Usage:     messages.FlagFollowDesc,
		Type:      "bool",
	}
	Since = Flag{
		Name:  "since",
		Usage: "Only return logs newer than a relative duration like 5m, 1h, or 24h",
	}

	WorkloadDescriptor = Flag{
		Name:  "descriptor",
		Usage: messages.WorkloadDescriptorFlag,
	}

	// Control plane configuration flags

	ControlPlane = Flag{
		Name:  "controlplane",
		Usage: "Control plane name for this context",
	}

	Credentials = Flag{
		Name:  "credentials",
		Usage: "Credentials name for this context",
	}

	// Scaffold-specific flags

	ScaffoldType = Flag{
		Name:  "componenttype",
		Usage: "Namespace-scoped component type in format workloadType/componentTypeName (e.g., deployment/web-app)",
	}

	Traits = Flag{
		Name:  "traits",
		Usage: "Comma-separated list of namespace-scoped Trait names to include",
	}

	ClusterTraits = Flag{
		Name:  "clustertraits",
		Usage: "Comma-separated list of cluster-scoped ClusterTrait names to include",
	}

	Workflow = Flag{
		Name:  "workflow",
		Usage: "Namespace-scoped Workflow name",
	}

	ClusterWorkflow = Flag{
		Name:  "clusterworkflow",
		Usage: "Cluster-scoped ClusterWorkflow name",
	}

	ClusterComponentType = Flag{
		Name:  "clustercomponenttype",
		Usage: "Cluster-scoped component type in format workloadType/componentTypeName (e.g., deployment/web-app)",
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

	// Mode flags

	Mode = Flag{
		Name:  "mode",
		Usage: "Operational mode: 'api-server' (default) or 'file-system'",
	}

	RootDir = Flag{
		Name:  "root-dir",
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

	URL = Flag{
		Name:  "url",
		Usage: "Control plane URL to update",
	}

	// Component deploy flags

	Release = Flag{
		Name:  "release",
		Usage: "Component release name to deploy to lowest environment",
	}

	To = Flag{
		Name:  "to",
		Usage: "Target environment to promote to",
	}

	Set = Flag{
		Name:  "set",
		Usage: "Set override values (format: type.path=value)",
		Type:  "stringArray",
	}

	WorkflowRun = Flag{
		Name:  "workflowrun",
		Usage: "Workflow run name (defaults to latest run)",
	}
)

// AddFlags adds the specified flags to the given command.
func AddFlags(cmd *cobra.Command, flags ...Flag) {
	for _, flag := range flags {
		switch flag.Type {
		case "bool":
			cmd.Flags().BoolP(flag.Name, flag.Shorthand, false, flag.Usage)
		case "int":
			cmd.Flags().IntP(flag.Name, flag.Shorthand, 0, flag.Usage)
		case "stringArray":
			cmd.Flags().StringArrayP(flag.Name, flag.Shorthand, nil, flag.Usage)
		default:
			// Default to string type
			cmd.Flags().StringP(flag.Name, flag.Shorthand, "", flag.Usage)
		}
	}
}
