// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewReleaseBindingCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "releasebinding",
		Aliases: []string{"releasebindings", "rb"},
		Short:   "Manage release bindings",
		Long:    "Commands for managing release bindings.",
	}
	cmd.AddCommand(
		newGenerateCmd(),
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newTreeCmd(f),
	)
	return cmd
}

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate release binding manifests",
		Long:  "Generate ReleaseBinding manifests for one or more components.",
		RunE: func(cmd *cobra.Command, args []string) error {
			allSet := isFlagInArgs("--all")
			projectSet := isFlagInArgs("--project")
			componentSet := isFlagInArgs("--component")
			componentReleaseSet := isFlagInArgs("--component-release")

			usePipeline := flags.GetUsePipeline(cmd)
			if allSet && usePipeline == "" {
				return fmt.Errorf("--use-pipeline is required when using --all scope")
			}
			if componentReleaseSet && !(projectSet && componentSet) {
				return fmt.Errorf("--component-release requires both --project and --component to be specified")
			}
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

			params := GenerateParams{
				TargetEnv:   flags.GetTargetEnv(cmd),
				UsePipeline: usePipeline,
				OutputPath:  flags.GetOutputPath(cmd),
				DryRun:      flags.GetDryRun(cmd),
				Mode:        flags.GetMode(cmd),
				RootDir:     flags.GetRootDir(cmd),
			}
			if allSet {
				params.All = true
			}
			if projectSet {
				params.ProjectName = flags.GetProject(cmd)
			}
			if componentSet {
				params.ComponentName = flags.GetComponent(cmd)
			}
			if componentReleaseSet {
				params.ComponentRelease = flags.GetComponentRelease(cmd)
			}
			return New(nil).Generate(params)
		},
	}
	flags.AddAll(cmd)
	flags.AddProject(cmd)
	flags.AddComponent(cmd)
	flags.AddTargetEnv(cmd)
	flags.AddUsePipeline(cmd)
	flags.AddComponentRelease(cmd)
	flags.AddOutputPath(cmd)
	flags.AddDryRun(cmd)
	flags.AddMode(cmd)
	flags.AddRootDir(cmd)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List release bindings",
		Long:  `List all release bindings for a specific component.`,
		Example: `  # List all release bindings for a component
  occ releasebinding list --namespace acme-corp --project online-store --component product-catalog`,
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
		Use:   "get [RELEASE_BINDING_NAME]",
		Short: "Get a release binding",
		Long:  `Get a release binding and display its details in YAML format.`,
		Example: `  # Get a release binding
  occ releasebinding get my-binding --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:          flags.GetNamespace(cmd),
				ReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [RELEASE_BINDING_NAME]",
		Short: "Delete a release binding",
		Long:  `Delete a release binding by name.`,
		Example: `  # Delete a release binding
  occ releasebinding delete my-binding --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:          flags.GetNamespace(cmd),
				ReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newTreeCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tree [RELEASE_BINDING_NAME]",
		Short: "Show the resource tree for a release binding",
		Long: `Show the Kubernetes resources created by a release binding's releases,
including child resources discovered in the target planes (for example the
Pods running under a Deployment).

A line starting with ⚠ means children of that kind could not be discovered
under that node — the tree there is incomplete, not empty.`,
		Example: `  # Show the resource tree
  occ releasebinding tree checkout-dev --namespace acme-corp

  # Refresh continuously
  occ releasebinding tree checkout-dev --namespace acme-corp --watch

  # Watch for half an hour, then stop
  occ releasebinding tree checkout-dev --namespace acme-corp --watch --timeout 30m

  # Only branches containing Pods
  occ releasebinding tree checkout-dev --namespace acme-corp --kind Pod`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			watch, _ := cmd.Flags().GetBool("watch")
			interval, _ := cmd.Flags().GetDuration("interval")
			timeout, _ := cmd.Flags().GetDuration("timeout")
			kind, _ := cmd.Flags().GetString("kind")
			return New(cl).Tree(TreeParams{
				Namespace:          flags.GetNamespace(cmd),
				ReleaseBindingName: args[0],
				Watch:              watch,
				Interval:           interval,
				Timeout:            timeout,
				Kind:               kind,
			})
		},
	}
	flags.AddNamespace(cmd)
	cmd.Flags().Bool("watch", false, "Continuously refresh the tree; on a terminal it redraws in place and leaves the last tree behind on exit")
	cmd.Flags().Duration("interval", 10*time.Second, "Refresh interval used with --watch")
	cmd.Flags().Duration("timeout", 10*time.Minute, "Stop watching after this duration (0 = never); used with --watch")
	cmd.Flags().String("kind", "", "Only show branches containing this kind (optionally group/Kind)")
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
