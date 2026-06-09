// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewWorkflowRunCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workflowrun",
		Aliases: []string{"wr", "workflowruns"},
		Short:   "Manage workflow runs",
		Long:    `Manage workflow runs for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newLogsCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow runs",
		Long:  `List all workflow runs in a namespace.`,
		Example: `  # List all workflow runs in a namespace
  occ workflowrun list --namespace acme-corp`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			ns := flags.GetNamespace(cmd)
			wf, _ := cmd.Flags().GetString("workflow")
			return New(cl).List(ListParams{
				Namespace: ns,
				Workflow:  wf,
			})
		},
	}
	flags.AddNamespace(cmd)
	cmd.Flags().String("workflow", "", "Namespace-scoped Workflow name")
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [WORKFLOW_RUN_NAME]",
		Short: "Get a workflow run",
		Long:  `Get a workflow run and display its details in YAML format.`,
		Example: `  # Get a workflow run
  occ workflowrun get my-run --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:       flags.GetNamespace(cmd),
				WorkflowRunName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newLogsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [WORKFLOW_RUN_NAME]",
		Short: "Get logs for a workflow run",
		Long: `Get logs for a workflow run.
Fetches live logs from the workflow plane for active runs,
or archived logs from the observer for completed runs.`,
		Example: `  # Get logs for a workflow run
  occ workflowrun logs my-run --namespace acme-corp

  # Follow logs
  occ workflowrun logs my-run --namespace acme-corp -f`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Logs(LogsParams{
				Namespace:       flags.GetNamespace(cmd),
				WorkflowRunName: args[0],
				Follow:          flags.GetFollow(cmd),
				Since:           flags.GetSince(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddFollow(cmd)
	flags.AddSince(cmd)
	return cmd
}
