// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewDeploymentPipelineCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploymentpipeline",
		Aliases: []string{"deppipe", "deploymentpipelines"},
		Short:   "Manage deployment pipelines",
		Long:    `Manage deployment pipelines for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployment pipelines",
		Long:  `List all deployment pipelines in a namespace.`,
		Example: `  # List all deployment pipelines in a namespace
  occ deploymentpipeline list --namespace acme-corp`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [DEPLOYMENT_PIPELINE_NAME]",
		Short: "Get a deployment pipeline",
		Long:  `Get a deployment pipeline and display its details in YAML format.`,
		Example: `  # Get a deployment pipeline
  occ deploymentpipeline get my-pipeline --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:              flags.GetNamespace(cmd),
				DeploymentPipelineName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [DEPLOYMENT_PIPELINE_NAME]",
		Short: "Delete a deployment pipeline",
		Long:  `Delete a deployment pipeline by name.`,
		Example: `  # Delete a deployment pipeline
  occ deploymentpipeline delete my-pipeline --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:              flags.GetNamespace(cmd),
				DeploymentPipelineName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
