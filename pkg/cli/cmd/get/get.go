// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package get

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
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

	// Deployment Pipeline command
	deploymentPipelineCmd := (&builder.CommandBuilder{
		Command: constants.ListDeploymentPipeline,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetDeploymentPipeline(api.GetDeploymentPipelineParams{
				Name:         name,
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
			})
		},
	}).Build()
	deploymentPipelineCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(deploymentPipelineCmd)

	// Configuration groups command
	configurationGroupsCmd := (&builder.CommandBuilder{
		Command: constants.ListConfigurationGroup,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Interactive},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return impl.GetConfigurationGroup(api.GetConfigurationGroupParams{
				Name:         name,
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
			})
		},
	}).Build()
	configurationGroupsCmd.Args = cobra.MaximumNArgs(1)
	listCmd.AddCommand(configurationGroupsCmd)

	return listCmd
}
