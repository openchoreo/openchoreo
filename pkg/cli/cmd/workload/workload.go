// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	workloadcmd "github.com/openchoreo/openchoreo/internal/occ/cmd/workload"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewWorkloadCmd() *cobra.Command {
	workloadCmd := &cobra.Command{
		Use:     constants.Workload.Use,
		Aliases: constants.Workload.Aliases,
		Short:   constants.Workload.Short,
		Long:    constants.Workload.Long,
	}

	workloadCmd.AddCommand(
		newCreateWorkloadCmd(),
		newListWorkloadCmd(),
		newGetWorkloadCmd(),
		newDeleteWorkloadCmd(),
	)

	return workloadCmd
}

func newCreateWorkloadCmd() *cobra.Command {
	workloadFlags := []flags.Flag{
		flags.Name,
		flags.Namespace,
		flags.Project,
		flags.Component,
		flags.Image,
		flags.Output,
		flags.WorkloadDescriptor,
		flags.DryRun,
		flags.Mode,
		flags.RootDir,
	}

	return (&builder.CommandBuilder{
		Command: constants.CreateWorkload,
		Flags:   workloadFlags,
		RunE: func(fg *builder.FlagGetter) error {
			return workloadcmd.New().Create(workloadcmd.CreateParams{
				FilePath:      fg.GetString(flags.WorkloadDescriptor),
				NamespaceName: fg.GetString(flags.Namespace),
				ProjectName:   fg.GetString(flags.Project),
				ComponentName: fg.GetString(flags.Component),
				ImageURL:      fg.GetString(flags.Image),
				OutputPath:    fg.GetString(flags.Output),
				DryRun:        fg.GetBool(flags.DryRun),
				Mode:          fg.GetString(flags.Mode),
				RootDir:       fg.GetString(flags.RootDir),
			})
		},
	}).Build()
}

func newListWorkloadCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListWorkload,
		Flags:   []flags.Flag{flags.Namespace},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(fg *builder.FlagGetter) error {
			return workloadcmd.New().List(workloadcmd.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}

func newGetWorkloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetWorkload.Use,
		Short:   constants.GetWorkload.Short,
		Long:    constants.GetWorkload.Long,
		Example: constants.GetWorkload.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return workloadcmd.New().Get(workloadcmd.GetParams{
				Namespace:    namespace,
				WorkloadName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newDeleteWorkloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteWorkload.Use,
		Short:   constants.DeleteWorkload.Short,
		Long:    constants.DeleteWorkload.Long,
		Example: constants.DeleteWorkload.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return workloadcmd.New().Delete(workloadcmd.DeleteParams{
				Namespace:    namespace,
				WorkloadName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
