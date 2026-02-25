// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewDeploymentPipelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeploymentPipelineRoot.Use,
		Aliases: constants.DeploymentPipelineRoot.Aliases,
		Short:   constants.DeploymentPipelineRoot.Short,
		Long:    constants.DeploymentPipelineRoot.Long,
	}

	cmd.AddCommand(
		newListDeploymentPipelineCmd(),
		newGetDeploymentPipelineCmd(),
		newDeleteDeploymentPipelineCmd(),
	)

	return cmd
}

func newListDeploymentPipelineCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListDeploymentPipelineDirect,
		Flags:   []flags.Flag{flags.Namespace},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(fg *builder.FlagGetter) error {
			return deploymentpipeline.New().List(deploymentpipeline.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}

func newGetDeploymentPipelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetDeploymentPipeline.Use,
		Short:   constants.GetDeploymentPipeline.Short,
		Long:    constants.GetDeploymentPipeline.Long,
		Example: constants.GetDeploymentPipeline.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return deploymentpipeline.New().Get(deploymentpipeline.GetParams{
				Namespace:              namespace,
				DeploymentPipelineName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newDeleteDeploymentPipelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteDeploymentPipeline.Use,
		Short:   constants.DeleteDeploymentPipeline.Short,
		Long:    constants.DeleteDeploymentPipeline.Long,
		Example: constants.DeleteDeploymentPipeline.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return deploymentpipeline.New().Delete(deploymentpipeline.DeleteParams{
				Namespace:              namespace,
				DeploymentPipelineName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
