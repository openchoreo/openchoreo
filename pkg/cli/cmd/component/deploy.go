// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// newDeployComponentCmd creates the deploy subcommand for components
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

			// Validate required flags
			if namespace == "" {
				return fmt.Errorf("--namespace is required")
			}
			if project == "" {
				return fmt.Errorf("--project is required")
			}

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
