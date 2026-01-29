// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewWorkloadCmd(impl api.CommandImplementationInterface) *cobra.Command {
	workloadCmd := &cobra.Command{
		Use:     constants.Workload.Use,
		Aliases: constants.Workload.Aliases,
		Short:   constants.Workload.Short,
		Long:    constants.Workload.Long,
	}

	workloadCmd.AddCommand(
		newCreateWorkloadCmd(impl),
	)

	return workloadCmd
}

func newCreateWorkloadCmd(impl api.CommandImplementationInterface) *cobra.Command {
	workloadFlags := []flags.Flag{
		flags.Name,
		flags.Namespace,
		flags.Project,
		flags.Component,
		flags.Image,
		flags.Output,
		flags.WorkloadDescriptor,
		flags.DryRun,
	}

	return (&builder.CommandBuilder{
		Command: constants.CreateWorkload,
		Flags:   workloadFlags,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.CreateWorkload(api.CreateWorkloadParams{
				FilePath:      fg.GetString(flags.WorkloadDescriptor),
				NamespaceName: fg.GetString(flags.Namespace),
				ProjectName:   fg.GetString(flags.Project),
				ComponentName: fg.GetString(flags.Component),
				ImageURL:      fg.GetString(flags.Image),
				OutputPath:    fg.GetString(flags.Output),
				DryRun:        fg.GetBool(flags.DryRun),
			})
		},
	}).Build()
}
