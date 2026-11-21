// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewApplyCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply OpenChoreo resources by file name or stdin",
		Long: `Apply a configuration file, directory, or standard input to create or update OpenChoreo resources.

Examples:
  # Apply a namespace configuration
  occ apply -f namespace.yaml

  # Apply all YAML manifests in a directory
  occ apply -f manifests/

  # Apply resources from standard input
  cat namespace.yaml | occ apply -f -

  # Preview resources that would be created or updated without making changes
  occ apply -f namespace.yaml --dry-run`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			cl, err := f()
			if err != nil {
				return err
			}
			return Apply(cl.(*client.Client), Params{
				FilePath: filePath,
				DryRun:   flags.GetDryRun(cmd),
			})
		},
	}
	cmd.Flags().StringP("file", "f", "", "Path to the configuration file or directory to apply, or '-' to read from standard input")
	flags.AddDryRun(cmd)
	return cmd
}
