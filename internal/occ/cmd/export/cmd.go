// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// NewExportCmd builds the command for exporting configuration as YAML files.
func NewExportCmd(f client.NewClientFunc) *cobra.Command {
	var params Params
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export OpenChoreo configuration as a GitOps directory tree",
		Long: `Fetch supported cluster-scoped configuration and resources from every
accessible OpenChoreo control-plane namespace and write one YAML manifest per
resource beneath the output directory. Missing directories are created.
Supported manifests whose identity matches the export layout are managed:
identical files are skipped, changed files are replaced, and stale files are
deleted. Other files are preserved. Local edits to managed files are replaced.
File-operation failures trigger rollback to the previous contents.

Objects marked for deletion are skipped. Manifests omit top-level status,
server metadata, the last-applied-configuration annotation, and the OpenChoreo
restart annotation. Specs and other labels and annotations are preserved.`,
		Example: `  occ export -o ./gitops-export
  occ export --output /path/to/gitops-repository
  occ export -o .`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			return params.validate()
		},
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := f()
			if err != nil {
				return err
			}
			files, err := Run(cmd.Context(), c, params)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Exported %d resources to %s.\n", len(files), params.OutputDir)
			return err
		},
	}
	cmd.Flags().StringVarP(&params.OutputDir, "output", "o", "", "Directory for exported YAML manifests (required)")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}
