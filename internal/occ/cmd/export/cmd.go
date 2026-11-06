// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

type plaintextChoice string

const (
	plaintextKeep      plaintextChoice = "y"
	plaintextRemove    plaintextChoice = "n"
	plaintextKeepAll   plaintextChoice = "a"
	plaintextRemoveAll plaintextChoice = "r"
)

var errExportCanceled = errors.New("export canceled; no files were written")

// NewExportCmd creates the export command.
func NewExportCmd(newClient client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export declarative configuration as YAML",
		Long: `Export supported resources from every namespace into a GitOps-oriented directory tree.

Exports cluster-scoped and namespace platform configuration, Namespaces,
Projects, Components, Workloads, Resources, and their releases and release
bindings. By default, releases from the last three calendar months and any older
releases referenced by exported bindings are included. This applies to
ComponentRelease, ProjectRelease, and ResourceRelease. Use --all-releases to
include the entire release history. Releases without creation timestamps are
retained.

It writes resource manifests and does not create or modify Kustomize or Flux configuration.
Existing files outside the selected export are not removed.`,
		Example: `  # Export configuration, releases from the last three months, and older referenced releases
  occ export -o ./my-gitops-repo

  # Export configuration and the entire release history
  occ export -o ./my-gitops-repo --all-releases`,
		Args:    cobra.NoArgs,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputDir, _ := cmd.Flags().GetString("output")
			all, _ := cmd.Flags().GetBool("all-releases")
			cl, err := newClient()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Exporting resources...")
			reader := bufio.NewReader(cmd.InOrStdin())
			return runWithSecretDecision(cmd.Context(), cl, cmd.OutOrStdout(), Params{
				OutputDir: outputDir, AllReleases: all,
			},
				newPlaintextDecision(reader, cmd.OutOrStdout()))
		},
	}
	cmd.Flags().StringP("output", "o", "", "Root directory of the output repository (required; created if missing)")
	cmd.Flags().Bool("all-releases", false, "Export the entire release history instead of the default three-month window plus referenced releases")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

// newPlaintextDecision prompts for credentials and remembers choices for remaining findings.
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

// promptKeepPlaintext asks how to handle a credential without displaying its value.
func promptKeepPlaintext(reader *bufio.Reader, out io.Writer, finding Withheld) (plaintextChoice, error) {
	fmt.Fprintf(out, "\nPossible plaintext credential\n  Resource: %s\n  File: %s\n  Field: %s\n  Reason: Literal value in a sensitive field\n\n",
		resourceDisplayName(finding), finding.File, strings.Join(finding.Reasons, ", "))
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

// printReport writes export counts, release selection, and credential decisions.
func printReport(out io.Writer, report *Report, outputDir string, complete bool) error {
	var output strings.Builder
	w := tabwriter.NewWriter(&output, 0, 0, 3, ' ', 0)
	if complete {
		fmt.Fprintln(w, "\nExport completed")
	} else {
		fmt.Fprintln(w, "\nExport incomplete")
	}
	fmt.Fprintf(w, "  Output:\t%s\n  Resources:\t%d resource(s)\n", outputDir, report.Exported)
	fmt.Fprintln(w, "\nFiles:")
	fmt.Fprintln(w, "  CREATED\tUPDATED\tUNCHANGED")
	fmt.Fprintf(w, "  %d\t%d\t%d\n", report.Created, report.Updated, report.Unchanged)

	fmt.Fprintln(w, "\nResources by kind:")
	if len(report.ExportedByKind) == 0 {
		fmt.Fprintln(w, "  No resources exported.")
	} else {
		kinds := make([]string, 0, len(report.ExportedByKind))
		for kind := range report.ExportedByKind {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		fmt.Fprintln(w, "  KIND\tCOUNT")
		for _, kind := range kinds {
			fmt.Fprintf(w, "  %s\t%d\n", kind, report.ExportedByKind[kind])
		}
	}

	fmt.Fprintln(w, "\nRelease history:")
	if report.ReleaseCutoff == nil {
		fmt.Fprintln(w, "  Policy:\tAll releases (--all-releases)")
	} else {
		fmt.Fprintln(w, "  Policy:\tLast 3 calendar months, plus binding-referenced releases")
		fmt.Fprintf(w, "  Cutoff (UTC):\t%s (inclusive)\n", report.ReleaseCutoff.UTC().Format(time.RFC3339Nano))
		fmt.Fprintf(w, "  Older referenced:\t%d retained\n", report.OlderReferencedReleases)
		fmt.Fprintf(w, "  Older unreferenced:\t%d skipped\n", report.ReleasesSkipped)
		if report.UndatedReleases > 0 {
			fmt.Fprintf(w, "  Without creation timestamps:\t%d retained\n", report.UndatedReleases)
		}
	}

	fmt.Fprintln(w, "\nPlaintext credentials:")
	if len(report.SecretDecisions) == 0 {
		fmt.Fprintln(w, "  None detected.")
	} else {
		kept, removed := 0, 0
		for _, decision := range report.SecretDecisions {
			if decision.Action == "kept" {
				kept++
			} else {
				removed++
			}
		}
		fmt.Fprintf(w, "  %d kept, %d removed\n", kept, removed)
		for _, decision := range report.SecretDecisions {
			action := "Removed"
			if decision.Action == "kept" {
				action = "Kept"
			}
			fmt.Fprintf(w, "\n  %s\n", resourceDisplayName(decision.Withheld))
			fmt.Fprintf(w, "    Action:\t%s\n    File:\t%s\n    Field:\t%s\n", action, decision.File, strings.Join(decision.Reasons, ", "))
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := io.WriteString(out, output.String())
	return err
}

func resourceDisplayName(finding Withheld) string {
	var parts []string
	for _, part := range []string{finding.Namespace, finding.Project, finding.Name} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return finding.Kind + " " + strings.Join(parts, "/")
}
