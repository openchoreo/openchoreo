// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewComponentReleaseCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "componentrelease",
		Aliases: []string{"cr", "componentreleases"},
		Short:   "Manage component releases",
		Long:    "Commands for managing component releases.",
	}
	cmd.AddCommand(
		newGenerateCmd(),
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
	)
	return cmd
}

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate component release manifests",
		Long:  "Generate ComponentRelease manifests for one or more components.",
		RunE: func(cmd *cobra.Command, args []string) error {
			allSet := isFlagInArgs("--all")
			projectSet := isFlagInArgs("--project")
			componentSet := isFlagInArgs("--component")
			nameSet := isFlagInArgs("--name")

			if allSet {
				if projectSet || componentSet {
					return fmt.Errorf("--all cannot be combined with --project or --component")
				}
				if nameSet {
					return fmt.Errorf("--all cannot be combined with --name")
				}
			} else if componentSet {
				if !projectSet {
					return fmt.Errorf("--component requires --project to be specified")
				}
			} else if projectSet {
				if nameSet {
					return fmt.Errorf("--name can only be used with --component (requires both --project and --component)")
				}
			} else {
				return fmt.Errorf("one of --all, --project, or --component must be specified")
			}
			if nameSet && !componentSet {
				return fmt.Errorf("--name requires --component to be specified")
			}

			params := GenerateParams{
				OutputPath: flags.GetOutputPath(cmd),
				DryRun:     flags.GetDryRun(cmd),
				Mode:       flags.GetMode(cmd),
				RootDir:    flags.GetRootDir(cmd),
			}
			name, _ := cmd.Flags().GetString("name")
			if allSet {
				params.All = true
			}
			if projectSet {
				params.ProjectName = flags.GetProject(cmd)
			}
			if componentSet {
				params.ComponentName = flags.GetComponent(cmd)
			}
			if nameSet {
				params.ReleaseName = name
			}
			return New(nil).Generate(params)
		},
	}
	cmd.Flags().String("name", "", "Name of the resource (must be lowercase letters, numbers, or hyphens)")
	flags.AddAll(cmd)
	flags.AddProject(cmd)
	flags.AddComponent(cmd)
	flags.AddOutputPath(cmd)
	flags.AddDryRun(cmd)
	flags.AddMode(cmd)
	flags.AddRootDir(cmd)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List component releases",
		Long:  `List all component releases for a specific component.`,
		Example: `  # List all component releases for a component
  occ componentrelease list --namespace acme-corp --project online-store --component product-catalog`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
				Project:   flags.GetProject(cmd),
				Component: flags.GetComponent(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	flags.AddComponent(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [COMPONENT_RELEASE_NAME]",
		Short: "Get a component release",
		Long:  `Get a component release and display its details in YAML format.`,
		Example: `  # Get a component release
  occ componentrelease get my-release --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:            flags.GetNamespace(cmd),
				ComponentReleaseName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [COMPONENT_RELEASE_NAME]",
		Short: "Delete a component release",
		Long:  `Delete a component release by name.`,
		Example: `  # Delete a component release
  occ componentrelease delete my-release --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:            flags.GetNamespace(cmd),
				ComponentReleaseName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

// isFlagInArgs checks if a flag was explicitly provided in os.Args.
func isFlagInArgs(flagName string) bool {
	for _, arg := range os.Args {
		if arg == flagName || strings.HasPrefix(arg, flagName+"=") {
			return true
		}
	}
	return false
}
