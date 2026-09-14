// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewWorkloadCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workload",
		Aliases: []string{"wl", "workloads"},
		Short:   "Manage workloads",
		Long:    `Manage workloads for OpenChoreo.`,
	}
	cmd.AddCommand(
		newCreateCmd(),
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
	)
	return cmd
}

func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workload from a descriptor file",
		Long: `Create a workload from a workload descriptor file.

The workload descriptor (workload.yaml) should be located alongside your source code
and describes the endpoints and configuration for your workload.

Examples:
  # Create workload from descriptor
  occ workload create --descriptor workload.yaml --namespace acme-corp --project online-store \
    --component product-catalog --image myimage:latest

  # Create workload and save to file
  occ workload create --descriptor workload.yaml --namespace acme-corp --project online-store \
    --component product-catalog --image myimage:latest --output-path workload-cr.yaml

  # Create workload with commit provenance, for Delivery Insights' Lead Time for Changes
  occ workload create --descriptor workload.yaml --namespace acme-corp --project online-store \
    --component product-catalog --image myimage:latest \
    --source-commit $(git rev-parse HEAD) --source-branch main \
    --source-repository https://github.com/my-org/my-repo \
    --source-authored-at $(git show -s --format=%aI HEAD)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			descriptor, _ := cmd.Flags().GetString("descriptor")
			image, _ := cmd.Flags().GetString("image")
			sourceCommit, _ := cmd.Flags().GetString("source-commit")
			sourceBranch, _ := cmd.Flags().GetString("source-branch")
			sourceRepository, _ := cmd.Flags().GetString("source-repository")
			sourceAuthoredAt, _ := cmd.Flags().GetString("source-authored-at")
			return New(nil).Create(CreateParams{
				FilePath:         descriptor,
				NamespaceName:    flags.GetNamespace(cmd),
				ProjectName:      flags.GetProject(cmd),
				ComponentName:    flags.GetComponent(cmd),
				ImageURL:         image,
				OutputPath:       flags.GetOutputPath(cmd),
				DryRun:           flags.GetDryRun(cmd),
				Mode:             flags.GetMode(cmd),
				RootDir:          flags.GetRootDir(cmd),
				SourceCommit:     sourceCommit,
				SourceBranch:     sourceBranch,
				SourceRepository: sourceRepository,
				SourceAuthoredAt: sourceAuthoredAt,
			})
		},
	}
	cmd.Flags().String("image", "", "Name of the Docker image (e.g., product-catalog:latest)")
	cmd.Flags().String("descriptor", "", "Path to the workload descriptor file (e.g., workload.yaml)")
	cmd.Flags().String("source-commit", "", "VCS commit SHA the image was built from (optional)")
	cmd.Flags().String("source-branch", "", "VCS branch the commit was built from (optional)")
	cmd.Flags().String("source-repository", "", "VCS repository URL the commit belongs to (optional)")
	cmd.Flags().String("source-authored-at", "",
		"RFC3339 commit author timestamp, e.g. from 'git show -s --format=%aI <sha>' (optional)")
	flags.AddNamespace(cmd)
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
		Short: "List workloads",
		Long:  `List all workloads in a namespace.`,
		Example: `  # List all workloads in a namespace
  occ workload list --namespace acme-corp`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [WORKLOAD_NAME]",
		Short: "Get a workload",
		Long:  `Get a workload and display its details in YAML format.`,
		Example: `  # Get a workload
  occ workload get my-workload --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:    flags.GetNamespace(cmd),
				WorkloadName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [WORKLOAD_NAME]",
		Short: "Delete a workload",
		Long:  `Delete a workload by name.`,
		Example: `  # Delete a workload
  occ workload delete my-workload --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:    flags.GetNamespace(cmd),
				WorkloadName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
