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
		[]flags.Flag{flags.Output, flags.Limit, flags.All},
		func(fg *builder.FlagGetter, name string) error {
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetOrganization(api.GetParams{
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	)
	listCmd.AddCommand(orgCmd)

	// Project command
	projectCmd := (&builder.CommandBuilder{
		Command: constants.ListProject,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetProject(api.GetProjectParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	}).Build()
	listCmd.AddCommand(projectCmd)

	// Component command
	componentCmd := (&builder.CommandBuilder{
		Command: constants.ListComponent,
		Flags:   []flags.Flag{flags.Organization, flags.Project, flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetComponent(api.GetComponentParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	}).Build()
	listCmd.AddCommand(componentCmd)

	// Build command
	buildCmd := (&builder.CommandBuilder{
		Command: constants.ListBuild,
		Flags:   []flags.Flag{flags.Organization, flags.Project, flags.Component, flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetBuild(api.GetBuildParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	}).Build()
	listCmd.AddCommand(buildCmd)

	// Deployable Artifact command
	deployableArtifactCmd := (&builder.CommandBuilder{
		Command: constants.ListDeployableArtifact,
		Flags: []flags.Flag{
			flags.Organization,
			flags.Project,
			flags.Component,
			flags.DeploymentTrack,
			flags.Build,
			flags.Image,
			flags.Output,
			flags.Limit,
			flags.All,
		},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetDeployableArtifact(api.GetDeployableArtifactParams{
				Organization:    fg.GetString(flags.Organization),
				Project:         fg.GetString(flags.Project),
				Component:       fg.GetString(flags.Component),
				DeploymentTrack: fg.GetString(flags.DeploymentTrack),
				Build:           fg.GetString(flags.Build),
				DockerImage:     fg.GetString(flags.Image),
				OutputFormat:    fg.GetString(flags.Output),
				Name:            name,
				Limit:           limit,
			})
		},
	}).Build()
	listCmd.AddCommand(deployableArtifactCmd)

	// Environment command
	envCmd := (&builder.CommandBuilder{
		Command: constants.ListEnvironment,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetEnvironment(api.GetEnvironmentParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	}).Build()
	listCmd.AddCommand(envCmd)

	// Deployment Track command
	deploymentTrackCmd := (&builder.CommandBuilder{
		Command: constants.ListDeploymentTrack,
		Flags:   []flags.Flag{flags.Organization, flags.Project, flags.Component, flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetDeploymentTrack(api.GetDeploymentTrackParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	}).Build()
	listCmd.AddCommand(deploymentTrackCmd)

	// Deployment command
	deploymentCmd := buildListCommand(
		constants.ListDeployment,
		[]flags.Flag{
			flags.Organization,
			flags.Project,
			flags.Component,
			flags.Environment,
			flags.Output,
			flags.Limit,
			flags.All,
		},
		func(fg *builder.FlagGetter, name string) error {
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetDeployment(api.GetDeploymentParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				Environment:  fg.GetString(flags.Environment),
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	)
	listCmd.AddCommand(deploymentCmd)

	// Endpoint command
	endpointCmd := buildListCommand(
		constants.ListEndpoint,
		[]flags.Flag{
			flags.Organization,
			flags.Project,
			flags.Component,
			flags.Environment,
			flags.Output,
			flags.Limit,
			flags.All,
		},
		func(fg *builder.FlagGetter, name string) error {
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetEndpoint(api.GetEndpointParams{
				Organization: fg.GetString(flags.Organization),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				Environment:  fg.GetString(flags.Environment),
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	)
	listCmd.AddCommand(endpointCmd)

	// DataPlane command
	dataPlaneCmd := (&builder.CommandBuilder{
		Command: constants.ListDataPlane,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetDataPlane(api.GetDataPlaneParams{
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Name:         name,
				Limit:        limit,
			})
		},
	}).Build()
	listCmd.AddCommand(dataPlaneCmd)

	// Deployment Pipeline command
	deploymentPipelineCmd := (&builder.CommandBuilder{
		Command: constants.ListDeploymentPipeline,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetDeploymentPipeline(api.GetDeploymentPipelineParams{
				Name:         name,
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Limit:        limit,
			})
		},
	}).Build()
	listCmd.AddCommand(deploymentPipelineCmd)

	// Configuration groups command
	configurationGroupsCmd := (&builder.CommandBuilder{
		Command: constants.ListConfigurationGroup,
		Flags:   []flags.Flag{flags.Organization, flags.Output, flags.Limit, flags.All},
		RunE: func(fg *builder.FlagGetter) error {
			name := ""
			if len(fg.GetArgs()) > 0 {
				name = fg.GetArgs()[0]
			}
			limit := fg.GetInt(flags.Limit)
			if fg.GetBool(flags.All) {
				limit = 0
			}
			return impl.GetConfigurationGroup(api.GetConfigurationGroupParams{
				Name:         name,
				Organization: fg.GetString(flags.Organization),
				OutputFormat: fg.GetString(flags.Output),
				Limit:        limit,
			})
		},
	}).Build()
	listCmd.AddCommand(configurationGroupsCmd)

	return listCmd
}
