// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewComponentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	componentCmd := &cobra.Command{
		Use:     constants.Component.Use,
		Aliases: constants.Component.Aliases,
		Short:   constants.Component.Short,
		Long:    constants.Component.Long,
	}

	componentCmd.AddCommand(
		newListComponentCmd(impl),
		newScaffoldComponentCmd(impl),
		newDeployComponentCmd(impl),
		newLogsComponentCmd(impl),
	)

	return componentCmd
}

func newListComponentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponent,
		Flags:   []flags.Flag{flags.Namespace, flags.Project},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListComponents(api.ListComponentsParams{
				Namespace: fg.GetString(flags.Namespace),
				Project:   fg.GetString(flags.Project),
			})
		},
	}).Build()
}

func newScaffoldComponentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	componentFlags := []flags.Flag{
		flags.Name,
		flags.ScaffoldType,
		flags.Traits,
		flags.Workflow,
		flags.Project,
		flags.Namespace,
		flags.OutputFile,
		flags.SkipComments,
		flags.SkipOptional,
	}

	return (&builder.CommandBuilder{
		Command: constants.ScaffoldComponent,
		Flags:   componentFlags,
		PreRunE: auth.RequireLogin(impl),
		RunE: func(fg *builder.FlagGetter) error {
			// Parse traits from comma-separated string
			traitsStr := fg.GetString(flags.Traits)
			var traits []string
			if traitsStr != "" {
				parts := strings.Split(traitsStr, ",")
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						traits = append(traits, trimmed)
					}
				}
			}

			return impl.ScaffoldComponent(api.ScaffoldComponentParams{
				ComponentName: fg.GetString(flags.Name),
				ComponentType: fg.GetString(flags.ScaffoldType),
				Traits:        traits,
				WorkflowName:  fg.GetString(flags.Workflow),
				Namespace:     fg.GetString(flags.Namespace),
				ProjectName:   fg.GetString(flags.Project),
				OutputPath:    fg.GetString(flags.OutputFile),
				SkipComments:  fg.GetBool(flags.SkipComments),
				SkipOptional:  fg.GetBool(flags.SkipOptional),
			})
		},
	}).Build()
}

func newDeployComponentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeployComponent.Use,
		Short:   constants.DeployComponent.Short,
		Long:    constants.DeployComponent.Long,
		Example: constants.DeployComponent.Example,
		Args:    cobra.ExactArgs(1), // Requires COMPONENT_NAME
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get component name from positional arg
			componentName := args[0]

			// Get flag values
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			project, _ := cmd.Flags().GetString(flags.Project.Name)
			release, _ := cmd.Flags().GetString(flags.Release.Name)
			to, _ := cmd.Flags().GetString(flags.To.Name)
			set, _ := cmd.Flags().GetStringArray(flags.Set.Name)
			outputFormat, _ := cmd.Flags().GetString(flags.Output.Name)

			// Create params
			params := api.DeployComponentParams{
				ComponentName: componentName,
				Namespace:     namespace,
				Project:       project,
				Release:       release,
				To:            to,
				Set:           set,
				OutputFormat:  outputFormat,
			}

			// Execute deploy
			return impl.DeployComponent(params)
		},
	}

	// Add flags
	flags.AddFlags(cmd,
		flags.Namespace,
		flags.Project,
		flags.Release,
		flags.To,
		flags.Set,
		flags.Output,
	)

	return cmd
}

func newLogsComponentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs COMPONENT_NAME",
		Short: "Get logs for a component",
		Long: `Retrieve logs for a component from a specific environment.
If --env is not specified, uses the lowest environment from the deployment pipeline.`,
		Example: `  # Get logs for a component (uses lowest environment if --env not specified)
  occ component logs my-component

  # Get logs from a specific environment
  occ component logs my-component --env dev

  # Get logs with custom since duration
  occ component logs my-component --env dev --since 30m

  # Follow logs in real-time
  occ component logs my-component --env dev -f`,
		Args: cobra.ExactArgs(1), // Requires COMPONENT_NAME
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get component name from positional arg
			componentName := args[0]

			// Get flag values
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			project, _ := cmd.Flags().GetString(flags.Project.Name)
			environment, _ := cmd.Flags().GetString(flags.Environment.Name)
			follow, _ := cmd.Flags().GetBool(flags.Follow.Name)
			since, _ := cmd.Flags().GetString(flags.Since.Name)

			// Create params
			params := api.ComponentLogsParams{
				Namespace:   namespace,
				Project:     project,
				Component:   componentName,
				Environment: environment,
				Follow:      follow,
				Since:       since,
			}

			// Execute logs
			return impl.ComponentLogs(params)
		},
	}

	// Add flags
	flags.AddFlags(cmd,
		flags.Namespace,
		flags.Project,
		flags.Environment,
		flags.Follow,
		flags.Since,
	)

	return cmd
}
