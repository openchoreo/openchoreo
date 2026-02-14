// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

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

// NewReleaseBindingCmd creates the release-binding command group
func NewReleaseBindingCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ReleaseBindingRoot.Use,
		Short: constants.ReleaseBindingRoot.Short,
		Long:  constants.ReleaseBindingRoot.Long,
	}

	cmd.AddCommand(
		newGenerateCmd(impl),
		newListCmd(impl),
	)
	return cmd
}

// newGenerateCmd creates the release-binding generate command
func newGenerateCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ReleaseBindingGenerate,
		Flags: []flags.Flag{
			flags.All,
			flags.Project,
			flags.Component,
			flags.TargetEnv,
			flags.UsePipeline,
			flags.ComponentRelease,
			flags.OutputPath,
			flags.DryRun,
			flags.Mode,
			flags.RootDir,
		},
		RunE: func(fg *builder.FlagGetter) error {
			// Validate required flags
			targetEnv := fg.GetString(flags.TargetEnv)
			usePipeline := fg.GetString(flags.UsePipeline)

			if targetEnv == "" {
				return fmt.Errorf("--target-env is required")
			}
			if usePipeline == "" {
				return fmt.Errorf("--use-pipeline is required")
			}

			// Check flag combinations
			allSet := isFlagInArgs("--all")
			projectSet := isFlagInArgs("--project")
			componentSet := isFlagInArgs("--component")
			componentReleaseSet := isFlagInArgs("--component-release")

			// --component-release requires both --project and --component
			if componentReleaseSet && !(projectSet && componentSet) {
				return fmt.Errorf("--component-release requires both --project and --component to be specified")
			}

			// Standard scope validation
			if allSet {
				if projectSet || componentSet {
					return fmt.Errorf("--all cannot be combined with --project or --component")
				}
			} else if componentSet {
				if !projectSet {
					return fmt.Errorf("--component requires --project to be specified")
				}
			} else if !projectSet {
				return fmt.Errorf("one of --all, --project, or --component must be specified")
			}

			params := api.GenerateReleaseBindingParams{
				TargetEnv:   targetEnv,
				UsePipeline: usePipeline,
				OutputPath:  fg.GetString(flags.OutputPath),
				DryRun:      fg.GetBool(flags.DryRun),
				Mode:        fg.GetString(flags.Mode),
				RootDir:     fg.GetString(flags.RootDir),
			}

			if allSet {
				params.All = true
			}
			if projectSet {
				params.ProjectName = fg.GetString(flags.Project)
			}
			if componentSet {
				params.ComponentName = fg.GetString(flags.Component)
			}
			if componentReleaseSet {
				params.ComponentRelease = fg.GetString(flags.ComponentRelease)
			}

			return impl.GenerateReleaseBinding(params)
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

// newListCmd creates the release-binding list command
func newListCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListReleaseBinding,
		Flags: []flags.Flag{
			flags.Namespace,
			flags.Project,
			flags.Component,
		},
		RunE: func(fg *builder.FlagGetter) error {
			params := api.ListReleaseBindingsParams{
				Namespace: fg.GetString(flags.Namespace),
				Project:   fg.GetString(flags.Project),
				Component: fg.GetString(flags.Component),
			}
			return impl.ListReleaseBindings(params)
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
