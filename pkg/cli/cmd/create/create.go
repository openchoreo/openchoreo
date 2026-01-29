// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package create

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// Helper functions for common flag sets
func getBasicFlags() []flags.Flag {
	return []flags.Flag{
		flags.Name,
	}
}

func getOrgScopedFlags() []flags.Flag {
	return append(getBasicFlags(),
		flags.Namespace,
	)
}

func getProjectLevelFlags() []flags.Flag {
	return append(getOrgScopedFlags(),
		flags.Project,
	)
}

func getComponentLevelFlags() []flags.Flag {
	return append(getProjectLevelFlags(),
		flags.Component,
	)
}

func getMetadataFlags() []flags.Flag { //nolint:unused // Used by temporarily disabled create commands
	return append(getBasicFlags(),
		flags.DisplayName,
		flags.Description,
	)
}

//nolint:unused // Temporarily disabled
func newCreateNamespaceCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.CreateNamespace,
		Flags:   getMetadataFlags(),
		RunE: func(fg *builder.FlagGetter) error {
			return impl.CreateNamespace(api.CreateNamespaceParams{
				Name:        fg.GetString(flags.Name),
				DisplayName: fg.GetString(flags.DisplayName),
				Description: fg.GetString(flags.Description),
			})
		},
	}).Build()
}

//nolint:unused // Temporarily disabled
func newCreateProjectCmd(impl api.CommandImplementationInterface) *cobra.Command {
	projectFlags := append(getOrgScopedFlags(),
		flags.DisplayName,
		flags.Description,
	)
	return (&builder.CommandBuilder{
		Command: constants.CreateProject,
		Flags:   projectFlags,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.CreateProject(api.CreateProjectParams{
				Name:               fg.GetString(flags.Name),
				Namespace:          fg.GetString(flags.Namespace),
				DisplayName:        fg.GetString(flags.DisplayName),
				Description:        fg.GetString(flags.Description),
				DeploymentPipeline: fg.GetString(flags.DeploymentPipeline),
			})
		},
	}).Build()
}

func newCreateWorkloadCmd(impl api.CommandImplementationInterface) *cobra.Command {
	workloadFlags := append(getComponentLevelFlags(),
		flags.Image,
		flags.Output,
		flags.WorkloadDescriptor,
		flags.DryRun,
	)

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

//nolint:unused // Temporarily disabled
func newCreateDataPlaneCmd(impl api.CommandImplementationInterface) *cobra.Command {
	dpFlags := append(getMetadataFlags(),
		flags.PublicVirtualHost,
		flags.NamespaceVirtualHost,
		flags.Namespace,
		flags.ClusterAgentClientCA,
	)
	return (&builder.CommandBuilder{
		Command: constants.CreateDataPlane,
		Flags:   dpFlags,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.CreateDataPlane(api.CreateDataPlaneParams{
				Name:                 fg.GetString(flags.Name),
				Namespace:            fg.GetString(flags.Namespace),
				DisplayName:          fg.GetString(flags.DisplayName),
				Description:          fg.GetString(flags.Description),
				ClusterAgentClientCA: fg.GetString(flags.ClusterAgentClientCA),
				PublicVirtualHost:    fg.GetString(flags.PublicVirtualHost),
				NamespaceVirtualHost: fg.GetString(flags.NamespaceVirtualHost),
			})
		},
	}).Build()
}

//nolint:unused // Temporarily disabled
func newCreateEnvironmentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	envFlags := append(getOrgScopedFlags(),
		flags.DisplayName,
		flags.Description,
		flags.IsProduction,
		flags.DNSPrefix,
		flags.DataPlaneRef,
	)
	return (&builder.CommandBuilder{
		Command: constants.CreateEnvironment,
		Flags:   envFlags,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.CreateEnvironment(api.CreateEnvironmentParams{
				Name:         fg.GetString(flags.Name),
				Namespace:    fg.GetString(flags.Namespace),
				DisplayName:  fg.GetString(flags.DisplayName),
				Description:  fg.GetString(flags.Description),
				DataPlaneRef: fg.GetString(flags.DataPlaneRef),
				IsProduction: fg.GetBool(flags.IsProduction),
				DNSPrefix:    fg.GetString(flags.DNSPrefix),
			})
		},
	}).Build()
}

//nolint:unused // Temporarily disabled
func newCreateDeploymentPipelineCmd(impl api.CommandImplementationInterface) *cobra.Command {
	dpFlags := []flags.Flag{
		flags.Namespace,
		flags.Name,
		flags.EnvironmentOrder,
	}

	return (&builder.CommandBuilder{
		Command: constants.CreateDeploymentPipeline,
		Flags:   dpFlags,
		RunE: func(fg *builder.FlagGetter) error {
			// Get environment order from flag
			envOrderStr := fg.GetString(flags.EnvironmentOrder)
			var environmentOrder []string
			if envOrderStr != "" {
				environmentOrder = strings.Split(envOrderStr, ",")
				for i := range environmentOrder {
					environmentOrder[i] = strings.TrimSpace(environmentOrder[i])
				}
			}

			return impl.CreateDeploymentPipeline(api.CreateDeploymentPipelineParams{
				Name:             fg.GetString(flags.Name),
				Namespace:        fg.GetString(flags.Namespace),
				EnvironmentOrder: environmentOrder,
			})
		},
	}).Build()
}
