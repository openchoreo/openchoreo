// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewWorkflowCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workflow",
		Aliases: []string{"wf", "workflows"},
		Short:   "Manage workflows",
		Long:    `Manage workflows for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newStartCmd(f),
		newLogsCmd(),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflows",
		Long:  `List all workflows available in a namespace.`,
		Example: `  # List all workflows in a namespace
  occ workflow list --namespace acme-corp`,
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
		Use:   "get [WORKFLOW_NAME]",
		Short: "Get a workflow",
		Long:  `Get a workflow and display its details in YAML format.`,
		Example: `  # Get a workflow
  occ workflow get docker --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:    flags.GetNamespace(cmd),
				WorkflowName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [WORKFLOW_NAME]",
		Short: "Delete a workflow",
		Long:  `Delete a workflow by name.`,
		Example: `  # Delete a workflow
  occ workflow delete my-workflow --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:    flags.GetNamespace(cmd),
				WorkflowName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newStartCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run WORKFLOW_NAME",
		Short: "Run a workflow",
		Long:  `Run a new workflow with optional parameters.`,
		Example: `  # Run a workflow
  occ workflow run database-migration --namespace acme-corp

  # Run with parameters
  occ workflow run database-migration --namespace acme-corp --set key=value`,
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

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs WORKFLOW_NAME",
		Short: "Get logs for a workflow",
		Long: `Get logs for a workflow by finding the latest workflow run.
Use --workflowrun to specify a particular workflow run instead of the latest.`,
		Example: `  # Get logs for the latest run of a workflow
  occ workflow logs my-workflow --namespace acme-corp

  # Follow logs
  occ workflow logs my-workflow --namespace acme-corp -f`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return New(nil).Logs(LogsParams{
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
