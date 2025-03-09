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

package get

import (
	"github.com/spf13/cobra"

	"github.com/choreo-idp/choreo/pkg/cli/common/builder"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/flags"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

func NewListCmd(impl api.CommandImplementationInterface) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   constants.List.Use,
		Short: constants.List.Short,
		Long:  constants.List.Long,
	}

	// Organization command
	orgCmd := (&builder.CommandBuilder{
		Command: constants.ListOrganization,
		Flags:   []flags.Flag{flags.Output},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetOrganization(api.GetParams{
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
			})
		},
	}).Build()
	orgCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(orgCmd)

	// Project command
	projectCmd := (&builder.CommandBuilder{
		Command: constants.ListProject,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetProject(api.GetProjectParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
			})
		},
	}).Build()
	projectCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(projectCmd)

	// Component command
	componentCmd := (&builder.CommandBuilder{
		Command: constants.ListComponent,
		Flags:   []flags.Flag{flags.Organization, flags.Project, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetComponent(api.GetComponentParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
			})
		},
	}).Build()
	componentCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(componentCmd)

	// Build command
	buildCmd := (&builder.CommandBuilder{
		Command: constants.ListBuild,
		Flags:   []flags.Flag{flags.Organization, flags.Project, flags.Component, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetBuild(api.GetBuildParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
			})
		},
	}).Build()
	buildCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(buildCmd)

	// Deployable Artifact command
	deployableArtifactCmd := (&builder.CommandBuilder{
		Command: constants.ListDeployableArtifact,
		Flags: []flags.Flag{flags.Organization, flags.Project, flags.Component, flags.DeploymentTrack,
			flags.Build, flags.DockerImage, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetDeployableArtifact(api.GetDeployableArtifactParams{
				Organization:    fg.GetString(flags.Organization),
				Project:         fg.GetString(flags.Project),
				Component:       fg.GetString(flags.Component),
				DeploymentTrack: fg.GetString(flags.DeploymentTrack),
				Build:           fg.GetString(flags.Build),
				DockerImage:     fg.GetString(flags.DockerImage),
				OutputFormat:    fg.GetString(flags.Output),
				Interactive:     fg.GetBool(flags.Interactive),
				Name:            name,
			})
		},
	}).Build()
	listCmd.AddCommand(deployableArtifactCmd)

	// Environment command
	envCmd := (&builder.CommandBuilder{
		Command: constants.ListEnvironment,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetEnvironment(api.GetEnvironmentParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
			})
		},
	}).Build()
	envCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(envCmd)

	// Deployment Track command
	deploymentTrackCmd := (&builder.CommandBuilder{
		Command: constants.ListDeploymentTrack,
		Flags:   []flags.Flag{flags.Organization, flags.Project, flags.Component, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetDeploymentTrack(api.GetDeploymentTrackParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
			})
		},
	}).Build()
	deploymentTrackCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(deploymentTrackCmd)

	// Deployment command
	deploymentCmd := (&builder.CommandBuilder{
		Command: constants.ListDeployment,
		Flags: []flags.Flag{flags.Organization, flags.Project, flags.Component, flags.Environment,
			flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetDeployment(api.GetDeploymentParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				Environment:  fg.GetString(flags.Environment),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
			})
		},
	}).Build()
	listCmd.AddCommand(deploymentCmd)

	// Endpoint command
	endpointCmd := (&builder.CommandBuilder{
		Command: constants.ListEndpoint,
		Flags: []flags.Flag{flags.Organization, flags.Project, flags.Component, flags.Environment,
			flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetEndpoint(api.GetEndpointParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				Environment:  fg.GetString(flags.Environment),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
			})
		},
	}).Build()
	endpointCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(endpointCmd)

	// DataPlane command
	dataPlaneCmd := (&builder.CommandBuilder{
		Command: constants.ListDataPlane,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetDataPlane(api.GetDataPlaneParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
			})
		},
	}).Build()
	dataPlaneCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(dataPlaneCmd)

	return listCmd
}
