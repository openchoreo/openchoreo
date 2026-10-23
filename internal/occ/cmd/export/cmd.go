// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// NewExportCmd returns the `occ export` command.
func NewExportCmd(newClient client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export declarative configuration as YAML",
		Long: `Export supported resources from every namespace into a GitOps-oriented directory tree.

This version exports supported cluster-scoped resources, namespace platform
configuration, Namespaces, Projects, Components, Workloads, ComponentReleases,
and ReleaseBindings. It writes only OpenChoreo resource manifests and does not
create or modify Kustomize or Flux configuration. Review the reported coverage
gaps before using the output with Flux.`,
		Example: `  occ export -o ./my-gitops-repo`,
		Args:    cobra.NoArgs,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputDir, _ := cmd.Flags().GetString("output")
			cl, err := newClient()
			if err != nil {
				return err
			}
			reader := bufio.NewReader(cmd.InOrStdin())
			return runWithSecretDecision(context.Background(), cl, cmd.OutOrStdout(), Params{OutputDir: outputDir},
				func(finding Withheld) bool {
					return promptKeepPlaintext(reader, cmd.OutOrStdout(), finding)
				})
		},
	}
	cmd.Flags().StringP("output", "o", "", "Root directory of the output repository (created if missing)")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

// Run executes the export and prints a summary to w.
func Run(ctx context.Context, c client.Interface, w io.Writer, params Params) error {
	return runWithSecretDecision(ctx, c, w, params, nil)
}

func runWithSecretDecision(ctx context.Context, c client.Interface, w io.Writer, params Params, keep func(Withheld) bool) error {
	report, err := runEngine(ctx, c, Options{OutputDir: params.OutputDir, KeepPlaintext: keep})
	if err != nil && isExportPermissionError(err) {
		return errors.New("your role does not have permission to perform this export operation!")
	}
	if report != nil {
		fmt.Fprintf(w, "Exported %d resource(s) to %s: %d created, %d updated, %d unchanged\n",
			report.Exported, params.OutputDir, report.Created, report.Updated, report.Unchanged)
		kinds := make([]string, 0, len(report.ExportedByKind))
		for kind := range report.ExportedByKind {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			fmt.Fprintf(w, "  %-20s %d\n", kind, report.ExportedByKind[kind])
		}
		for _, decision := range report.SecretDecisions {
			fmt.Fprintf(w, "PLAINTEXT %s %s/%s/%s: %s (%s)\n",
				decision.Kind, decision.Namespace, decision.Project, decision.Name,
				decision.Action, strings.Join(decision.Reasons, ", "))
		}
		fmt.Fprintln(w, "Coverage is incomplete; not exported by this version:")
		for _, category := range report.Unsupported {
			fmt.Fprintf(w, "  - %s\n", category)
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func isExportPermissionError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "do not have permission") ||
		strings.Contains(message, "permission denied") ||
		strings.Contains(message, "forbidden")
}

func promptKeepPlaintext(reader *bufio.Reader, out io.Writer, finding Withheld) bool {
	fmt.Fprintf(out, "\nPossible plaintext credential in %s %s/%s/%s at %s.\n",
		finding.Kind, finding.Namespace, finding.Project, finding.Name, strings.Join(finding.Reasons, ", "))
	fmt.Fprint(out, "Keep these value(s) in the exported YAML? [y/N]: ")
	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			fmt.Fprintln(out, "\nNo response; stripping detected value(s).")
			return false
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true
		case "n", "no", "":
			return false
		default:
			fmt.Fprint(out, "Please answer y or n: ")
		}
	}
}
