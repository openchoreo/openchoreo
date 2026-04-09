// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package root

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/apply"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/authzrole"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/authzrolebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterauthzrole"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterauthzrolebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clustercomponenttype"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterdataplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterobservabilityplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clustertrait"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflowplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/component"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/componentrelease"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/componenttype"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/dataplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/environment"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/logout"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/namespace"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/observabilityalertsnotificationchannel"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/observabilityplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/project"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/releasebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/secretreference"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/trait"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/version"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workload"
)

// BuildRootCmd assembles the root command with all subcommands.
func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "occ",
		Short: "OpenChoreo CLI",
		Long:  "occ is the command-line interface for OpenChoreo.",
	}

	rootCmd.AddCommand(
		apply.NewApplyCmd(),
		login.NewLoginCmd(),
		logout.NewLogoutCmd(),
		config.NewConfigCmd(),
		version.NewVersionCmd(),
		componentrelease.NewComponentReleaseCmd(),
		releasebinding.NewReleaseBindingCmd(),
		namespace.NewNamespaceCmd(),
		project.NewProjectCmd(),
		component.NewComponentCmd(),
		environment.NewEnvironmentCmd(),
		dataplane.NewDataPlaneCmd(),
		workflowplane.NewWorkflowPlaneCmd(),
		observabilityplane.NewObservabilityPlaneCmd(),
		componenttype.NewComponentTypeCmd(),
		clustercomponenttype.NewClusterComponentTypeCmd(),
		clusterdataplane.NewClusterDataPlaneCmd(),
		clusterobservabilityplane.NewClusterObservabilityPlaneCmd(),
		clusterworkflowplane.NewClusterWorkflowPlaneCmd(),
		trait.NewTraitCmd(),
		clustertrait.NewClusterTraitCmd(),
		clusterworkflow.NewClusterWorkflowCmd(),
		clusterauthzrole.NewClusterAuthzRoleCmd(),
		clusterauthzrolebinding.NewClusterAuthzRoleBindingCmd(),
		authzrole.NewAuthzRoleCmd(),
		authzrolebinding.NewAuthzRoleBindingCmd(),
		workflow.NewWorkflowCmd(),
		workflowrun.NewWorkflowRunCmd(),
		secretreference.NewSecretReferenceCmd(),
		workload.NewWorkloadCmd(),
		deploymentpipeline.NewDeploymentPipelineCmd(),
		observabilityalertsnotificationchannel.NewObservabilityAlertsNotificationChannelCmd(),
	)

	return rootCmd
}
