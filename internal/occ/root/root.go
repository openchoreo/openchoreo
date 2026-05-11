// Copyright 2026 The OpenChoreo Authors
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
	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterresourcetype"
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
	"github.com/openchoreo/openchoreo/internal/occ/cmd/resource"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/resourcerelease"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/resourcereleasebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/resourcetype"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/secretreference"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/trait"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/version"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workload"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// BuildRootCmd assembles the root command with all subcommands.
func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "occ",
		Short: "OpenChoreo CLI",
		Long:  "occ is the command-line interface for OpenChoreo.",
	}

	f := client.NewClientFunc(func() (client.Interface, error) {
		return client.NewClient()
	})

	rootCmd.AddCommand(
		apply.NewApplyCmd(f),
		login.NewLoginCmd(),
		logout.NewLogoutCmd(),
		config.NewConfigCmd(),
		version.NewVersionCmd(),
		componentrelease.NewComponentReleaseCmd(f),
		resourcerelease.NewResourceReleaseCmd(f),
		resourcereleasebinding.NewResourceReleaseBindingCmd(f),
		releasebinding.NewReleaseBindingCmd(f),
		namespace.NewNamespaceCmd(f),
		project.NewProjectCmd(f),
		component.NewComponentCmd(f),
		resource.NewResourceCmd(f),
		environment.NewEnvironmentCmd(f),
		dataplane.NewDataPlaneCmd(f),
		workflowplane.NewWorkflowPlaneCmd(f),
		observabilityplane.NewObservabilityPlaneCmd(f),
		componenttype.NewComponentTypeCmd(f),
		resourcetype.NewResourceTypeCmd(f),
		clustercomponenttype.NewClusterComponentTypeCmd(f),
		clusterresourcetype.NewClusterResourceTypeCmd(f),
		clusterdataplane.NewClusterDataPlaneCmd(f),
		clusterobservabilityplane.NewClusterObservabilityPlaneCmd(f),
		clusterworkflowplane.NewClusterWorkflowPlaneCmd(f),
		trait.NewTraitCmd(f),
		clustertrait.NewClusterTraitCmd(f),
		clusterworkflow.NewClusterWorkflowCmd(f),
		clusterauthzrole.NewClusterAuthzRoleCmd(f),
		clusterauthzrolebinding.NewClusterAuthzRoleBindingCmd(f),
		authzrole.NewAuthzRoleCmd(f),
		authzrolebinding.NewAuthzRoleBindingCmd(f),
		workflow.NewWorkflowCmd(f),
		workflowrun.NewWorkflowRunCmd(f),
		secretreference.NewSecretReferenceCmd(f),
		workload.NewWorkloadCmd(f),
		deploymentpipeline.NewDeploymentPipelineCmd(f),
		observabilityalertsnotificationchannel.NewObservabilityAlertsNotificationChannelCmd(f),
	)

	return rootCmd
}
