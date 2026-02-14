// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// NewComponentReleaseCmd creates the component-release command group
func NewComponentReleaseCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ComponentReleaseRoot.Use,
		Short: constants.ComponentReleaseRoot.Short,
		Long:  constants.ComponentReleaseRoot.Long,
	}

	cmd.AddCommand(
		newGenerateCmd(impl),
		newListCmd(impl),
	)
	return cmd
}

// newGenerateCmd creates the component-release generate command
func newGenerateCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ComponentReleaseGenerate,
		Flags: []flags.Flag{
			flags.All,
			flags.Project,
			flags.Component,
			flags.Name,
			flags.OutputPath,
			flags.DryRun,
			flags.Mode,
			flags.RootDir,
		},
		RunE: func(fg *builder.FlagGetter) error {
			// Check which flags were explicitly provided by the user on the command line
			// We check os.Args because context defaults are applied via PersistentPreRunE
			// which marks flags as "changed" even though the user didn't provide them
			allSet := isFlagInArgs("--all")
			projectSet := isFlagInArgs("--project")
			componentSet := isFlagInArgs("--component")
			nameSet := isFlagInArgs("--name")
			outputPathSet := isFlagInArgs("--output-path")

			// Validation logic:
			// 1. If --all is set, reject --project, --component, or --name
			if allSet {
				if projectSet || componentSet {
					return fmt.Errorf("--all cannot be combined with --project or --component")
				}
				if nameSet {
					return fmt.Errorf("--all cannot be combined with --name")
				}
				// --all is valid on its own
			} else if componentSet {
				// 2. If --component is set, --project MUST also be set
				if !projectSet {
					return fmt.Errorf("--component requires --project to be specified")
				}
				// 3. If --component is set, --output-path MUST also be set
				if !outputPathSet {
					return fmt.Errorf("--output-path is required when specifying --component")
				}
				// --component with --project and --output-path is valid
				// --name is optional when --component is specified
			} else if projectSet {
				// 4. --project alone is valid (processes all components in that project)
				// But cannot use --name with --project alone
				if nameSet {
					return fmt.Errorf("--name can only be used with --component (requires both --project and --component)")
				}
				// Nothing else to validate
			} else {
				// 5. None of the required flags were explicitly set
				return fmt.Errorf("one of --all, --project, or --component must be specified")
			}

			// Validate --name is only used with --component
			if nameSet && !componentSet {
				return fmt.Errorf("--name requires --component to be specified")
			}

			// Build params with only explicitly set values (not context defaults)
			params := api.GenerateComponentReleaseParams{
				OutputPath: fg.GetString(flags.OutputPath),
				DryRun:     fg.GetBool(flags.DryRun),
				Mode:       fg.GetString(flags.Mode),
				RootDir:    fg.GetString(flags.RootDir),
			}

			// Only set the values that were explicitly provided
			if allSet {
				params.All = true
			}
			if projectSet {
				params.ProjectName = fg.GetString(flags.Project)
			}
			if componentSet {
				params.ComponentName = fg.GetString(flags.Component)
			}
			if nameSet {
				params.ReleaseName = fg.GetString(flags.Name)
			}

			return impl.GenerateComponentRelease(params)
		},
	}).Build()

	return cmd
}

// isFlagInArgs checks if a flag was explicitly provided in os.Args
func isFlagInArgs(flagName string) bool {
	for _, arg := range os.Args {
		if arg == flagName || strings.HasPrefix(arg, flagName+"=") {
			return true
		}
	}
	return false
}

// newListCmd creates the component-release list command
func newListCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponentRelease,
		Flags: []flags.Flag{
			flags.Namespace,
			flags.Project,
			flags.Component,
		},
		RunE: func(fg *builder.FlagGetter) error {
			params := api.ListComponentReleasesParams{
				Namespace: fg.GetString(flags.Namespace),
				Project:   fg.GetString(flags.Project),
				Component: fg.GetString(flags.Component),
			}
			return impl.ListComponentReleases(params)
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
