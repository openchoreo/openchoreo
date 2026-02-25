// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/releasebinding"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

// NewReleaseBindingCmd creates the release-binding command group
func NewReleaseBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ReleaseBindingRoot.Use,
		Short: constants.ReleaseBindingRoot.Short,
		Long:  constants.ReleaseBindingRoot.Long,
	}

	cmd.AddCommand(
		newGenerateCmd(),
		newListCmd(),
		newGetCmd(),
		newDeleteCmd(),
	)
	return cmd
}

// newGenerateCmd creates the release-binding generate command
func newGenerateCmd() *cobra.Command {
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
			// Check flag combinations
			allSet := isFlagInArgs("--all")
			projectSet := isFlagInArgs("--project")
			componentSet := isFlagInArgs("--component")
			componentReleaseSet := isFlagInArgs("--component-release")

			// --use-pipeline is required for --all scope (no single project to derive from)
			usePipeline := fg.GetString(flags.UsePipeline)
			if allSet && usePipeline == "" {
				return fmt.Errorf("--use-pipeline is required when using --all scope")
			}

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

			params := releasebinding.GenerateParams{
				TargetEnv:   fg.GetString(flags.TargetEnv),
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

			return releasebinding.New().Generate(params)
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
func newListCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListReleaseBinding,
		Flags: []flags.Flag{
			flags.Namespace,
			flags.Project,
			flags.Component,
		},
		RunE: func(fg *builder.FlagGetter) error {
			params := releasebinding.ListParams{
				Namespace: fg.GetString(flags.Namespace),
				Project:   fg.GetString(flags.Project),
				Component: fg.GetString(flags.Component),
			}
			return releasebinding.New().List(params)
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetReleaseBinding.Use,
		Short:   constants.GetReleaseBinding.Short,
		Long:    constants.GetReleaseBinding.Long,
		Example: constants.GetReleaseBinding.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return releasebinding.New().Get(releasebinding.GetParams{
				Namespace:          namespace,
				ReleaseBindingName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteReleaseBinding.Use,
		Short:   constants.DeleteReleaseBinding.Short,
		Long:    constants.DeleteReleaseBinding.Long,
		Example: constants.DeleteReleaseBinding.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return releasebinding.New().Delete(releasebinding.DeleteParams{
				Namespace:          namespace,
				ReleaseBindingName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
