// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowplane"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewWorkflowPlaneCmd() *cobra.Command {
	workflowPlaneCmd := &cobra.Command{
		Use:     constants.WorkflowPlane.Use,
		Aliases: constants.WorkflowPlane.Aliases,
		Short:   constants.WorkflowPlane.Short,
		Long:    constants.WorkflowPlane.Long,
	}

	workflowPlaneCmd.AddCommand(
		newListWorkflowPlaneCmd(),
		newGetWorkflowPlaneCmd(),
		newDeleteWorkflowPlaneCmd(),
	)

	return workflowPlaneCmd
}

func newListWorkflowPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListWorkflowPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return workflowplane.New(cl).List(workflowplane.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetWorkflowPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetWorkflowPlane.Use,
		Short:   constants.GetWorkflowPlane.Short,
		Long:    constants.GetWorkflowPlane.Long,
		Example: constants.GetWorkflowPlane.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return workflowplane.New(cl).Get(workflowplane.GetParams{
				Namespace:         namespace,
				WorkflowPlaneName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newDeleteWorkflowPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteWorkflowPlane.Use,
		Short:   constants.DeleteWorkflowPlane.Short,
		Long:    constants.DeleteWorkflowPlane.Long,
		Example: constants.DeleteWorkflowPlane.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return workflowplane.New(cl).Delete(workflowplane.DeleteParams{
				Namespace:         namespace,
				WorkflowPlaneName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
