// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package root

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/cmd/apply"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/buildplane"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/clustercomponenttype"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/clustertrait"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/component"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/componentrelease"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/componenttype"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/dataplane"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/deploymentpipeline"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/environment"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/logout"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/namespace"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/observabilityalertsnotificationchannel"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/observabilityplane"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/project"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/releasebinding"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/secretreference"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/trait"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/version"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/workflow"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/workload"
	"github.com/openchoreo/openchoreo/pkg/cli/common/config"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// BuildRootCmd assembles the root command with all subcommands
func BuildRootCmd(config *config.CLIConfig, impl api.CommandImplementationInterface) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   config.Name,
		Short: config.ShortDescription,
		Long:  config.LongDescription,
	}

	// Add all commands directly
	rootCmd.AddCommand(
		apply.NewApplyCmd(impl),
		login.NewLoginCmd(impl),
		logout.NewLogoutCmd(impl),
		configContext.NewConfigCmd(),
		version.NewVersionCmd(),
		componentrelease.NewComponentReleaseCmd(),
		releasebinding.NewReleaseBindingCmd(),
		// Resource commands
		namespace.NewNamespaceCmd(),
		project.NewProjectCmd(),
		component.NewComponentCmd(),
		environment.NewEnvironmentCmd(),
		dataplane.NewDataPlaneCmd(),
		buildplane.NewBuildPlaneCmd(),
		observabilityplane.NewObservabilityPlaneCmd(),
		componenttype.NewComponentTypeCmd(),
		clustercomponenttype.NewClusterComponentTypeCmd(),
		trait.NewTraitCmd(),
		clustertrait.NewClusterTraitCmd(),
		workflow.NewWorkflowCmd(),
		workflowrun.NewWorkflowRunCmd(),
		secretreference.NewSecretReferenceCmd(),
		workload.NewWorkloadCmd(),
		deploymentpipeline.NewDeploymentPipelineCmd(),
		observabilityalertsnotificationchannel.NewObservabilityAlertsNotificationChannelCmd(),
	)

	return rootCmd
}
