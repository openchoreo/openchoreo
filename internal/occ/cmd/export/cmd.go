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

Exports cluster-scoped and namespace platform configuration, Namespaces,
Projects, Components, Workloads, Resources, and their releases and release
bindings. It writes resource manifests and does not create or modify Kustomize
or Flux configuration.`,
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
			return runWithSecretDecision(cmd.Context(), cl, cmd.OutOrStdout(), Params{OutputDir: outputDir},
				newPlaintextDecision(reader, cmd.OutOrStdout()))
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

func runWithSecretDecision(ctx context.Context, c client.Interface, w io.Writer, params Params, keep func(Withheld) (bool, error)) error {
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

type plaintextChoice string

const (
	plaintextKeep      plaintextChoice = "y"
	plaintextRemove    plaintextChoice = "n"
	plaintextKeepAll   plaintextChoice = "a"
	plaintextRemoveAll plaintextChoice = "r"
)

var errExportCanceled = errors.New("export canceled; no files were written")

func newPlaintextDecision(reader *bufio.Reader, out io.Writer) func(Withheld) (bool, error) {
	var remaining plaintextChoice
	return func(finding Withheld) (bool, error) {
		if remaining != "" {
			return remaining == plaintextKeepAll, nil
		}
		choice, err := promptKeepPlaintext(reader, out, finding)
		if err != nil {
			return false, err
		}
		if choice == plaintextKeepAll || choice == plaintextRemoveAll {
			remaining = choice
		}
		return choice == plaintextKeep || choice == plaintextKeepAll, nil
	}
}

func promptKeepPlaintext(reader *bufio.Reader, out io.Writer, finding Withheld) (plaintextChoice, error) {
	var parts []string
	for _, part := range []string{finding.Namespace, finding.Project, finding.Name} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	fmt.Fprintf(out, "\nPossible plaintext credential\n  Resource: %s %s\n  File: %s\n  Field: %s\n  Reason: Literal value in a sensitive field\n\n",
		finding.Kind, strings.Join(parts, "/"), finding.File, strings.Join(finding.Reasons, ", "))
	fmt.Fprint(out, "Keep this value in the exported YAML?\n[y] Keep this\n[n] Remove this\n[a] Keep this and all remaining findings\n[r] Remove this and all remaining findings\n[q] Cancel export\nChoice [y/n/a/r/q]: ")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return "", fmt.Errorf("read plaintext credential choice: %w", err)
			}
			if len(line) == 0 {
				fmt.Fprintln(out, "\nNo response; removing this value.")
				return plaintextRemove, nil
			}
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return plaintextKeep, nil
		case "n", "no", "":
			return plaintextRemove, nil
		case "a":
			return plaintextKeepAll, nil
		case "r":
			return plaintextRemoveAll, nil
		case "q":
			return "", errExportCanceled
		default:
			fmt.Fprint(out, "Invalid choice. Enter y, n, a, r, or q.\nChoice [y/n/a/r/q]: ")
		}
	}
}
