// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package bindingrevision holds what the releasebindingrevision and
// resourcereleasebindingrevision commands share: their flags, the revision
// table, YAML output, and the rollback flow (preview, warnings, confirmation).
package bindingrevision

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Output formats for get.
const (
	OutputTable = "table"
	OutputYAML  = "yaml"
)

// ─────────────────────────────────────────────────────────────
// Flags
// ─────────────────────────────────────────────────────────────

func AddBinding(cmd *cobra.Command, kind string) {
	cmd.Flags().String("binding", "", fmt.Sprintf("Name of the %s", kind))
}

func GetBinding(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("binding")
	return v
}

// AddEnvironment adds --environment, which narrows a list to one environment.
func AddEnvironment(cmd *cobra.Command) {
	cmd.Flags().String("environment", "", "Only list revisions of bindings in this environment (e.g., production)")
}

func GetEnvironment(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("environment")
	return v
}

func AddRevision(cmd *cobra.Command) {
	cmd.Flags().Int64("revision", 0, "Revision number")
}

func GetRevision(cmd *cobra.Command) int64 {
	v, _ := cmd.Flags().GetInt64("revision")
	return v
}

// NamespaceOrContext returns --namespace, or the current context's namespace
// when the flag is not given. For commands that skip context defaults because
// their other flags are filters (config.SkipContextDefaultsAnnotation).
func NamespaceOrContext(cmd *cobra.Command) string {
	if ns, _ := cmd.Flags().GetString("namespace"); ns != "" {
		return ns
	}
	if ctx, err := config.GetCurrentContext(); err == nil && ctx != nil {
		return ctx.Namespace
	}
	return ""
}

// RevisionFromArgs returns the revision number a get command names: the
// optional REVISION_NAME argument, or else --revision. REVISION_NAME may be the
// revision's object name ({binding}-r{N}), its display name ({owner}-r{N}),
// "r{N}" or "{N}". Naming the revision both ways is allowed when they agree.
func RevisionFromArgs(cmd *cobra.Command, args []string) (int64, error) {
	flag := GetRevision(cmd)
	if len(args) == 0 {
		return flag, nil
	}
	n, err := ParseRevisionName(args[0])
	if err != nil {
		return 0, err
	}
	if flag != 0 && flag != n {
		return 0, fmt.Errorf("revision name %q is revision %d, but --revision is %d", args[0], n, flag)
	}
	return n, nil
}

// ParseRevisionName returns the revision number in name: "{N}", "r{N}", or a
// name ending in "-r{N}" such as an object name or display name.
func ParseRevisionName(name string) (int64, error) {
	s := strings.TrimSpace(name)
	digits := strings.TrimPrefix(s, "r")
	if !isRevisionNumber(digits) {
		idx := strings.LastIndex(s, "-r")
		if idx <= 0 {
			return 0, fmt.Errorf("revision name %q must be a revision name ending in -r<N>, r<N> or <N>", name)
		}
		digits = s[idx+len("-r"):]
	}
	if !isRevisionNumber(digits) {
		return 0, fmt.Errorf("revision name %q must be a revision name ending in -r<N>, r<N> or <N>", name)
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("revision name %q: %w", name, err)
	}
	return n, nil
}

// isRevisionNumber reports whether s is a revision number: digits, at least 1,
// without a leading zero.
func isRevisionNumber(s string) bool {
	if s == "" || s[0] == '0' {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func AddDryRun(cmd *cobra.Command, usage string) {
	cmd.Flags().Bool("dry-run", false, usage)
}

func GetDryRun(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("dry-run")
	return v
}

func AddYes(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolP("yes", "y", false, usage)
}

func GetYes(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("yes")
	return v
}

func AddOutput(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", OutputTable, "Output format: table or yaml")
}

func GetOutput(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("output")
	return v
}

// AddRollbackRevision adds --revision to a rollback command, where it is
// optional: without it the rollback returns to the previously active revision.
func AddRollbackRevision(cmd *cobra.Command) {
	cmd.Flags().Int64("revision", 0,
		"Revision to roll back to (default: the revision active before the current one)")
}

// GetRollbackRevision returns --revision for a rollback: 0 when the flag is
// not given. A given value must be at least 1, so --revision 0 is an error
// rather than a silent request for the previous revision.
func GetRollbackRevision(cmd *cobra.Command) (int64, error) {
	v, _ := cmd.Flags().GetInt64("revision")
	if cmd.Flags().Changed("revision") && v < 1 {
		return 0, fmt.Errorf("--revision must be at least 1; omit it to roll back to the previously active revision")
	}
	return v, nil
}

// ValidateOutput rejects an output format get does not support.
func ValidateOutput(format string) error {
	switch format {
	case OutputTable, OutputYAML:
		return nil
	}
	return fmt.Errorf("unsupported output format %q: use table or yaml", format)
}

// ─────────────────────────────────────────────────────────────
// Table and YAML
// ─────────────────────────────────────────────────────────────

// Row is one revision as the table shows it. Name is its display name,
// {owner}-r{N} (see DisplayName).
type Row struct {
	// Project, Binding and Environment are only shown by PrintBindingTable.
	Project     string
	Binding     string
	Environment string
	Name        string
	Cause       string
	Health      string
	Active      bool
	KnownGood   bool
	CreatedAt   *time.Time
	// Message is the revision's change message; tables show it shortened.
	Message string
}

// StatusRow fills a Row's status columns from a revision's status.
func StatusRow(row Row, status *gen.BindingRevisionStatus) Row {
	if status == nil {
		return row
	}
	if status.Health != nil {
		row.Health = string(*status.Health)
	}
	row.Active = status.Active != nil && *status.Active
	row.KnownGood = status.KnownGood != nil && *status.KnownGood
	return row
}

// revisionColumns are the columns every revision table ends with; revisionCells
// returns a row's values for them.
const revisionColumns = "NAME\tCAUSE\tHEALTH\tACTIVE\tKNOWN-GOOD\tAGE\tMESSAGE"

// tableMessageLength is how many characters of a change message a table shows;
// get -o yaml shows all of it.
const tableMessageLength = 50

// shortMessage returns msg on one line, cut to tableMessageLength characters.
func shortMessage(msg string) string {
	msg = strings.Join(strings.Fields(msg), " ")
	if runes := []rune(msg); len(runes) > tableMessageLength {
		return string(runes[:tableMessageLength-1]) + "…"
	}
	return msg
}

func revisionCells(r Row) string {
	age := ""
	if r.CreatedAt != nil {
		age = utils.FormatAge(*r.CreatedAt)
	}
	return fmt.Sprintf("%s\t%s\t%s\t%t\t%t\t%s\t%s", r.Name, r.Cause, r.Health, r.Active, r.KnownGood, age,
		shortMessage(r.Message))
}

// printNoRevisions writes the message an empty revision list shows.
func printNoRevisions(w io.Writer) error {
	_, err := fmt.Fprintln(w, "No revisions found")
	return err
}

// PrintTable writes revisions in the order given, under the columns every
// revision list uses.
func PrintTable(w io.Writer, rows []Row) error {
	if len(rows) == 0 {
		return printNoRevisions(w)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, revisionColumns)
	for _, r := range rows {
		fmt.Fprintln(tw, revisionCells(r))
	}
	return tw.Flush()
}

// PrintBindingTable writes the revisions of several bindings in the order
// given, with the binding and environment each revision belongs to, and its
// project when showProject is set (the list was not narrowed to one project).
func PrintBindingTable(w io.Writer, rows []Row, showProject bool) error {
	if len(rows) == 0 {
		return printNoRevisions(w)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	if showProject {
		fmt.Fprint(tw, "PROJECT\t")
	}
	fmt.Fprintln(tw, "BINDING\tENVIRONMENT\t"+revisionColumns)
	for _, r := range rows {
		if showProject {
			fmt.Fprintf(tw, "%s\t", r.Project)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Binding, r.Environment, revisionCells(r))
	}
	return tw.Flush()
}

// PrintYAML writes v as YAML.
func PrintYAML(w io.Writer, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal revision to YAML: %w", err)
	}
	_, err = w.Write(data)
	return err
}

// ─────────────────────────────────────────────────────────────
// Rollback
// ─────────────────────────────────────────────────────────────

// Confirmer asks the user a yes/no question.
type Confirmer func(out io.Writer, question string) (bool, error)

// ConfirmOnTerminal reads the answer from stdin. It refuses rather than blocks
// when stdin is not a terminal: a piped rollback or deletion must say --yes
// explicitly.
func ConfirmOnTerminal(out io.Writer, question string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("stdin is not a terminal; pass --yes to proceed without confirmation")
	}
	return confirm(os.Stdin, out, question)
}

func confirm(in io.Reader, out io.Writer, question string) (bool, error) {
	fmt.Fprintf(out, "%s [y/N]: ", question)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

// RollbackFunc calls the rollback API, as a dry run or for real. A nil
// revision asks the server for the revision that was active before the
// current one.
type RollbackFunc func(ctx context.Context, revision *int64, dryRun bool) (*gen.RollbackResponse, error)

// CheckMessage refuses a change message longer than a revision keeps, so it is
// never cut short without notice.
func CheckMessage(msg string) error {
	if n := utf8.RuneCountInString(msg); n > maxMessageLength {
		return fmt.Errorf("--message must be at most %d characters (got %d)", maxMessageLength, n)
	}
	return nil
}

// maxMessageLength is the longest change message a revision keeps: the CRD's
// maxLength for spec.message.
const maxMessageLength = 1024

// WithChangeMessage adds msg to a binding's metadata as its change message, so
// the revision recording this change carries it. An empty msg adds nothing.
func WithChangeMessage(meta *gen.ObjectMeta, msg string) {
	if msg == "" {
		return
	}
	if meta.Annotations == nil {
		meta.Annotations = &map[string]string{}
	}
	(*meta.Annotations)[openchoreov1alpha1.AnnotationKeyChangeMessage] = msg
}

// RollbackRequest is the request body of one rollback call. The message is only
// sent when the rollback is applied: a preview changes nothing to describe.
func RollbackRequest(revision *int64, dryRun bool, message string) gen.RollbackRequest {
	req := gen.RollbackRequest{Revision: revision, DryRun: &dryRun}
	if !dryRun && message != "" {
		req.Message = &message
	}
	return req
}

// RollbackOptions configures RunRollback.
type RollbackOptions struct {
	// Kind names the binding kind in messages, e.g. "release binding".
	Kind    string
	Binding string
	// Revision is the revision to roll back to; 0 rolls back to the revision
	// that was active before the current one.
	Revision int64
	DryRun   bool
	Yes      bool
	Out      io.Writer
	Confirm  Confirmer
}

// RunRollback runs the rollback flow:
//   - --dry-run: preview the rollback (changes and warnings) and write nothing.
//   - default: preview it, ask for confirmation, then apply it.
//   - --yes: apply it straight away and print the warnings with the result.
//
// Without --revision the server picks the target: the revision that was
// active immediately before the current one, by activation history. The
// preview shows which, and the confirmed rollback is applied to that revision
// by number, so it cannot move to another target if the history changes in
// between. With --yes the server picks and applies it in one request.
//
// The server repeats every check when the rollback is applied, so a binding
// that changes between the preview and the confirmation is refused, not
// overwritten.
func RunRollback(ctx context.Context, opts RollbackOptions, rollback RollbackFunc) error {
	if opts.Revision < 0 {
		return fmt.Errorf("--revision must be at least 1, or omitted for the previously active revision")
	}
	implicit := opts.Revision == 0
	var target *int64
	if !implicit {
		target = &opts.Revision
	}

	var preview *gen.RollbackResponse
	if opts.DryRun || !opts.Yes {
		var err error
		if preview, err = rollback(ctx, target, true); err != nil {
			return err
		}
		printPlan(opts.Out, opts.Kind, opts.Binding, preview, implicit)
		if opts.DryRun {
			fmt.Fprintln(opts.Out, "\nDry run: nothing was changed.")
			return nil
		}

		ok, err := opts.Confirm(opts.Out, fmt.Sprintf("\nRoll back %s %q to revision %d?", opts.Kind, opts.Binding,
			preview.TargetRevision))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(opts.Out, "Rollback cancelled.")
			return nil
		}
		// Apply exactly the target the user confirmed.
		target = &preview.TargetRevision
	}

	result, err := rollback(ctx, target, false)
	if err != nil {
		return err
	}
	if opts.Yes {
		if implicit {
			fmt.Fprintf(opts.Out, "Rolling back to revision %d, the previously active revision.\n", result.TargetRevision)
		}
		printWarnings(opts.Out, result.Warnings)
	}
	if preview != nil && preview.RevertedFrom != nil && result.RevertedFrom != nil &&
		*preview.RevertedFrom != *result.RevertedFrom {
		fmt.Fprintf(opts.Out, "Note: the active revision changed from %d to %d after the preview; "+
			"the rollback went to revision %d as confirmed.\n", *preview.RevertedFrom, *result.RevertedFrom,
			result.TargetRevision)
	}
	if hasWarning(result.Warnings, gen.NoSpecChange) {
		fmt.Fprintf(opts.Out, "Revision %d (%s) of %s %q is already active; nothing changed.\n",
			result.Revision, displayName(result), opts.Kind, opts.Binding)
		return nil
	}
	fmt.Fprintf(opts.Out, "Rolled back %s %q to revision %d: %s is active again.\n",
		opts.Kind, opts.Binding, result.Revision, displayName(result))
	return nil
}

// printPlan describes what a rollback will do.
// With implicit, the server chose the target and the plan says why.
func printPlan(out io.Writer, kind, binding string, plan *gen.RollbackResponse, implicit bool) {
	from := ""
	if plan.RevertedFrom != nil {
		from = fmt.Sprintf(" from revision %d", *plan.RevertedFrom)
	}
	fmt.Fprintf(out, "Rolling back %s %q%s to revision %d.\n", kind, binding, from, plan.TargetRevision)
	if implicit {
		fmt.Fprintf(out, "Target: %s, the revision active before the current one (no --revision given).\n",
			displayName(plan))
	}
	if hasWarning(plan.Warnings, gen.NoSpecChange) {
		fmt.Fprintln(out, "That revision is already active, so nothing will change.")
	} else {
		fmt.Fprintf(out, "No revision is created: %s becomes active again.\n", displayName(plan))
	}

	if len(plan.Changes) > 0 {
		fmt.Fprintln(out, "\nChanges:")
		for _, c := range plan.Changes {
			fmt.Fprintf(out, "  %s: %s -> %s\n", c.Field, formatValue(c.Current), formatValue(c.Target))
		}
	}
	printWarnings(out, plan.Warnings)
}

// DisplayName returns how a revision is shown: {owner}-r{N}, where owner is
// the component or resource name, e.g. frontend-r1. Object names are
// {binding}-r{N}.
func DisplayName(owner string, n int64) string {
	return fmt.Sprintf("%s-r%d", owner, n)
}

// displayName returns the target's display name, or its object name from a
// server that does not send one.
func displayName(r *gen.RollbackResponse) string {
	if r.RevisionDisplayName != nil && *r.RevisionDisplayName != "" {
		return *r.RevisionDisplayName
	}
	return r.RevisionName
}

func printWarnings(out io.Writer, warnings []gen.RollbackWarning) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(out, "\nWarnings:")
	for _, w := range warnings {
		fmt.Fprintf(out, "  ! %s: %s\n", w.Code, w.Message)
	}
}

func hasWarning(warnings []gen.RollbackWarning, code gen.RollbackWarningCode) bool {
	for _, w := range warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

// formatValue renders a spec value compactly: strings quoted, objects as JSON,
// unset as (unset).
func formatValue(v any) string {
	switch val := v.(type) {
	case nil:
		return "(unset)"
	case string:
		return strconv.Quote(val)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

// ─────────────────────────────────────────────────────────────
// Delete
// ─────────────────────────────────────────────────────────────

// DeleteFunc calls the revision delete API, as a dry run or for real.
type DeleteFunc func(ctx context.Context, dryRun bool) (*gen.DeleteBindingRevisionResponse, error)

// DeleteOptions configures RunDelete.
type DeleteOptions struct {
	// Kind names the binding kind in messages, e.g. "release binding".
	Kind    string
	Binding string
	// Target is the revision to delete, as list shows it.
	Target   Row
	Revision int64
	DryRun   bool
	Yes      bool
	Out      io.Writer
	Confirm  Confirmer
}

// RunDelete runs the revision deletion flow:
//   - show the revision to delete, then ask the server whether it may be
//     deleted (a dry run), so a protected revision is refused before any prompt;
//   - --dry-run: stop there;
//   - default: ask for confirmation, then delete it;
//   - --yes: delete it without asking.
//
// Success is only reported once the server has deleted the revision, and the
// server repeats every check then.
func RunDelete(ctx context.Context, opts DeleteOptions, del DeleteFunc) error {
	t := opts.Target
	status := "Inactive"
	if t.Active {
		status = "Active"
	}
	fmt.Fprintf(opts.Out, "Revision:    %s (revision %d of %s %q)\n", t.Name, opts.Revision, opts.Kind, opts.Binding)
	fmt.Fprintf(opts.Out, "Status:      %s\n", status)
	fmt.Fprintf(opts.Out, "Health:      %s\n", t.Health)
	fmt.Fprintf(opts.Out, "Known-good:  %t\n", t.KnownGood)
	fmt.Fprintf(opts.Out, "Cause:       %s\n", t.Cause)
	if t.Message != "" {
		fmt.Fprintf(opts.Out, "Message:     %s\n", t.Message)
	}

	if _, err := del(ctx, true); err != nil {
		return err
	}
	fmt.Fprintln(opts.Out, "\nDeleting it removes it from the rollback history. The other revisions keep their numbers.")
	if opts.DryRun {
		fmt.Fprintln(opts.Out, "Dry run: the revision can be deleted; nothing was changed.")
		return nil
	}
	if !opts.Yes {
		ok, err := opts.Confirm(opts.Out, "\nDelete this revision?")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(opts.Out, "Deletion cancelled.")
			return nil
		}
	}

	result, err := del(ctx, false)
	if err != nil {
		return err
	}
	name := t.Name
	if result.RevisionDisplayName != nil {
		name = *result.RevisionDisplayName
	}
	fmt.Fprintf(opts.Out, "Successfully deleted revision %d: %s\n", result.Revision, name)
	return nil
}
