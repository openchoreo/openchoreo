// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewApplyCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply OpenChoreo resources by file name",
		Long: `Apply a configuration file to create or update OpenChoreo resources.

Examples:
  # Apply a namespace configuration
  occ apply -f namespace.yaml`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			cl, err := f()
			if err != nil {
				return err
			}
			return Apply(cl.(*client.Client), Params{FilePath: filePath})
		},
	}
	cmd.Flags().StringP("file", "f", "", "Path to the configuration file to apply (e.g., manifests/deployment.yaml)")
	return cmd
}
