// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logs

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// NewLogsCmd creates the logs command
func NewLogsCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.Logs,
		Flags: []flags.Flag{
			flags.Organization,
			flags.Project,
			flags.Component,
			flags.Build,
			flags.LogType,
			flags.Follow,
			flags.Tail,
			flags.Interactive,
			flags.Environment,
			flags.Deployment,
			flags.DeploymentTrack,
		},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.GetLogs(api.LogParams{
				Type:            fg.GetString(flags.LogType),
				Organization:    fg.GetString(flags.Organization),
				Project:         fg.GetString(flags.Project),
				Component:       fg.GetString(flags.Component),
				Build:           fg.GetString(flags.Build),
				Follow:          fg.GetBool(flags.Follow),
				TailLines:       int64(fg.GetInt(flags.Tail)),
				Interactive:     fg.GetBool(flags.Interactive),
				Environment:     fg.GetString(flags.Environment),
				Deployment:      fg.GetString(flags.Deployment),
				DeploymentTrack: fg.GetString(flags.DeploymentTrack),
			})
		},
	}).Build()
}
