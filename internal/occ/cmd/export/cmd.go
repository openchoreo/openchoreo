// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// NewExportCmd returns the `occ export` command.
func NewExportCmd(newClient client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export component configuration as YAML",
		Long: `Export components from every namespace into a GitOps-oriented directory tree.

This initial version exports Component resources only. Namespace and Project
manifests, Workloads, platform resources, and Kustomize configuration are not
included, so review the reported coverage gaps before using the output with Flux.`,
		Example: `  occ export -o ./my-gitops-repo`,
		Args:    cobra.NoArgs,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputDir, _ := cmd.Flags().GetString("output")
			cl, err := newClient()
			if err != nil {
				return err
			}
			return Run(context.Background(), cl, cmd.OutOrStdout(), outputDir)
		},
	}
	cmd.Flags().StringP("output", "o", "", "Root directory of the output repository (created if missing)")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

// Run executes the export and prints a summary to w.
func Run(ctx context.Context, c client.Interface, w io.Writer, outputDir string) error {
	report, err := runEngine(ctx, c, Options{OutputDir: outputDir})
	if report != nil {
		fmt.Fprintf(w, "Exported %d component(s) to %s: %d created, %d updated, %d unchanged\n",
			report.Exported, outputDir, report.Created, report.Updated, report.Unchanged)
		for _, withheld := range report.Withheld {
			fmt.Fprintf(w, "WITHHELD %s/%s/%s: possible plaintext secret at %s\n",
				withheld.Namespace, withheld.Project, withheld.Name, strings.Join(withheld.Reasons, ", "))
		}
		fmt.Fprintln(w, "Coverage is incomplete; not exported by this version:")
		for _, category := range report.Unsupported {
			fmt.Fprintf(w, "  - %s\n", category)
		}
	}
	if err != nil {
		return err
	}
	if len(report.Withheld) > 0 {
		return fmt.Errorf("%d component(s) were withheld because they appear to contain plaintext secrets", len(report.Withheld))
	}
	return nil
}
