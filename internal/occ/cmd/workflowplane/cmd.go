// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewWorkflowPlaneCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workflowplane",
		Aliases: []string{"wp", "workflowplanes"},
		Short:   "Manage workflow planes",
		Long:    `Manage workflow planes for OpenChoreo.`,
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
		Short: "List workflow planes",
		Long:  `List all workflow planes in a namespace.`,
		Example: `  # List all workflow planes in a namespace
  occ workflowplane list --namespace acme-corp`,
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
		Use:   "get [WORKFLOWPLANE_NAME]",
		Short: "Get a workflow plane",
		Long:  `Get a workflow plane and display its details in YAML format.`,
		Example: `  # Get a workflow plane
  occ workflowplane get primary-workflowplane --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:         flags.GetNamespace(cmd),
				WorkflowPlaneName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [WORKFLOWPLANE_NAME]",
		Short: "Delete a workflow plane",
		Long:  `Delete a workflow plane by name.`,
		Example: `  # Delete a workflow plane
  occ workflowplane delete primary-workflowplane --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:         flags.GetNamespace(cmd),
				WorkflowPlaneName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
