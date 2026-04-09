// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterWorkflowCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterworkflow",
		Aliases: []string{"clusterworkflows"},
		Short:   "Manage cluster workflows",
		Long:    `Manage cluster-scoped workflows for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newStartCmd(f),
		newLogsCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List cluster workflows",
		Long:  `List all cluster-scoped workflows available across the cluster.`,
		Example: `  # List all cluster workflows
  occ clusterworkflow list`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List()
		},
	}
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "get [CLUSTER_WORKFLOW_NAME]",
		Short: "Get a cluster workflow",
		Long:  `Get a cluster workflow and display its details in YAML format.`,
		Example: `  # Get a cluster workflow
  occ clusterworkflow get build-go`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterWorkflowName: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_WORKFLOW_NAME]",
		Short: "Delete a cluster workflow",
		Long:  `Delete a cluster workflow by name.`,
		Example: `  # Delete a cluster workflow
  occ clusterworkflow delete build-go`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterWorkflowName: args[0]})
		},
	}
}

func newStartCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run CLUSTER_WORKFLOW_NAME",
		Short: "Run a cluster workflow",
		Long: `Run a new cluster workflow with optional parameters.
Requires --namespace to specify where the workflow run will be created.`,
		Example: `  # Run a cluster workflow
  occ clusterworkflow run dockerfile-builder --namespace acme-corp

  # Run with parameters
  occ clusterworkflow run dockerfile-builder --namespace acme-corp \
    --set spec.workflow.parameters.repository.url=https://github.com/example/repo`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).StartRun(StartRunParams{
				Namespace:    flags.GetNamespace(cmd),
				WorkflowName: args[0],
				Set:          flags.GetSet(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddSet(cmd)
	return cmd
}

func newLogsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs CLUSTER_WORKFLOW_NAME",
		Short: "Get logs for a cluster workflow run",
		Long: `Get logs for a cluster workflow by finding the latest workflow run.
Use --workflowrun to specify a particular workflow run instead of the latest.`,
		Example: `  # Get logs for the latest run of a cluster workflow
  occ clusterworkflow logs dockerfile-builder --namespace acme-corp

  # Follow logs
  occ clusterworkflow logs dockerfile-builder --namespace acme-corp -f`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Logs(LogsParams{
				Namespace:    flags.GetNamespace(cmd),
				WorkflowName: args[0],
				RunName:      flags.GetWorkflowRun(cmd),
				Follow:       flags.GetFollow(cmd),
				Since:        flags.GetSince(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddFollow(cmd)
	flags.AddSince(cmd)
	flags.AddWorkflowRun(cmd)
	return cmd
}
