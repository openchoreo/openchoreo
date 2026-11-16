// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// NewExportCmd builds the fetch-only export command.
func NewExportCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Fetch OpenChoreo configuration for GitOps adoption",
		Long: `Fetch supported cluster-scoped configuration and resources from every
accessible OpenChoreo control-plane namespace.

Prints the number of fetched objects. Manifest output is not yet available.`,
		Args:    cobra.NoArgs,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := f()
			if err != nil {
				return err
			}
			resources, err := Run(cmd.Context(), c)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Fetched %d resources.\n", len(resources))
			return err
		},
	}
}
