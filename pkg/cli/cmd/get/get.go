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

// buildListCommand creates a list command that accepts an optional name argument.
func buildListCommand(
	command constants.Command,
	flags []flags.Flag,
	executeFunc func(fg *builder.FlagGetter, name string) error,
) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: command,
		Flags:   flags,
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			return executeFunc(fg, name)
		},
	}).Build()
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}

func NewListCmd(impl api.CommandImplementationInterface) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   constants.List.Use,
		Short: constants.List.Short,
		Long:  constants.List.Long,
	}

	// Organization command
	orgCmd := buildListCommand(
		constants.ListOrganization,
		[]flags.Flag{flags.Output, flags.Limit},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetOrganization(api.GetParams{
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(orgCmd)

	// Project command
	projectCmd := buildListCommand(
		constants.ListProject,
		[]flags.Flag{
			flags.Organization, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetProject(api.GetProjectParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(projectCmd)

	// Component command
	componentCmd := buildListCommand(
		constants.ListComponent,
		[]flags.Flag{
			flags.Organization, flags.Project, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetComponent(api.GetComponentParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(componentCmd)

	// Build command
	buildCmd := buildListCommand(
		constants.ListBuild,
		[]flags.Flag{
			flags.Organization, flags.Project, flags.Component, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetBuild(api.GetBuildParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(buildCmd)

	// Deployable Artifact command
	deployableArtifactCmd := buildListCommand(
		constants.ListDeployableArtifact,
		[]flags.Flag{
			flags.Organization, flags.Project, flags.Component, flags.DeploymentTrack,
			flags.Build, flags.Image, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetDeployableArtifact(api.GetDeployableArtifactParams{
				Organization:    fg.GetString(flags.Organization),
				Project:         fg.GetString(flags.Project),
				Component:       fg.GetString(flags.Component),
				DeploymentTrack: fg.GetString(flags.DeploymentTrack),
				Build:           fg.GetString(flags.Build),
				DockerImage:     fg.GetString(flags.Image),
				OutputFormat:    fg.GetString(flags.Output),
				Interactive:     fg.GetBool(flags.Interactive),
				Name:            name,
				Limit:           fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(deployableArtifactCmd)

	// Environment command
	envCmd := buildListCommand(
		constants.ListEnvironment,
		[]flags.Flag{
			flags.Organization, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetEnvironment(api.GetEnvironmentParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(envCmd)

	// Deployment Track command
	deploymentTrackCmd := buildListCommand(
		constants.ListDeploymentTrack,
		[]flags.Flag{
			flags.Organization, flags.Project, flags.Component, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetDeploymentTrack(api.GetDeploymentTrackParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(deploymentTrackCmd)

	// Deployment command
	deploymentCmd := buildListCommand(
		constants.ListDeployment,
		[]flags.Flag{
			flags.Organization, flags.Project, flags.Component, flags.Environment,
			flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetDeployment(api.GetDeploymentParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				Environment:  fg.GetString(flags.Environment),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(deploymentCmd)

	// Endpoint command
	endpointCmd := buildListCommand(
		constants.ListEndpoint,
		[]flags.Flag{
			flags.Organization, flags.Project, flags.Component, flags.Environment,
			flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetEndpoint(api.GetEndpointParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				Environment:  fg.GetString(flags.Environment),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(endpointCmd)

	// DataPlane command
	dataPlaneCmd := buildListCommand(
		constants.ListDataPlane,
		[]flags.Flag{
			flags.Organization, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetDataPlane(api.GetDataPlaneParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Interactive:  fg.GetBool(flags.Interactive),
				Name:         name,
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(dataPlaneCmd)

	// Deployment Pipeline command
	deploymentPipelineCmd := buildListCommand(
		constants.ListDeploymentPipeline,
		[]flags.Flag{
			flags.Organization, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetDeploymentPipeline(api.GetDeploymentPipelineParams{
				Name:         name,
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(deploymentPipelineCmd)

	// Configuration groups command
	configurationGroupsCmd := buildListCommand(
		constants.ListConfigurationGroup,
		[]flags.Flag{
			flags.Organization, flags.Output, flags.Interactive, flags.Limit,
		},
		func(fg *builder.FlagGetter, name string) error {
			return impl.GetConfigurationGroup(api.GetConfigurationGroupParams{
				Name:         name,
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Limit:        fg.GetInt(flags.Limit),
			})
		},
	)
	listCmd.AddCommand(configurationGroupsCmd)

	return listCmd
}
