/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package logs

import (
	"github.com/spf13/cobra"

	"github.com/choreo-idp/choreo/pkg/cli/common/builder"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/flags"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
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
