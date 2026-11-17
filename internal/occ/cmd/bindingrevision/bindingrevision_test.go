// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package bindingrevision

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func ptr[T any](v T) *T { return &v }

// --- table ---

func TestPrintTable(t *testing.T) {
	now := time.Now().Add(-2 * time.Hour)
	var out bytes.Buffer
	require.NoError(t, PrintTable(&out, []Row{
		{Name: "api-ccc-r3", Cause: "Override", Health: "Progressing", Active: true, CreatedAt: &now},
		{Name: "api-bbb-r2", Cause: "Deploy", Health: "Healthy", KnownGood: true, CreatedAt: &now},
	}))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, []string{"NAME", "CAUSE", "HEALTH", "ACTIVE", "KNOWN-GOOD", "AGE", "MESSAGE"}, strings.Fields(lines[0]))
	assert.Equal(t, []string{"api-ccc-r3", "Override", "Progressing", "true", "false", "2h"}, strings.Fields(lines[1]))
	assert.Equal(t, []string{"api-bbb-r2", "Deploy", "Healthy", "false", "true", "2h"}, strings.Fields(lines[2]))
}

func TestPrintTable_Empty(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, PrintTable(&out, nil))
	assert.Equal(t, "No revisions found\n", out.String())
}

func TestStatusRow(t *testing.T) {
	health := gen.BindingRevisionStatusHealth("Degraded")
	row := StatusRow(Row{Name: "api-r1"}, &gen.BindingRevisionStatus{Health: &health, Active: ptr(true), KnownGood: ptr(true)})
	assert.Equal(t, Row{Name: "api-r1", Health: "Degraded", Active: true, KnownGood: true}, row)
	assert.Equal(t, Row{Name: "api-r1"}, StatusRow(Row{Name: "api-r1"}, nil), "a revision with no status yet")
}

func TestValidateOutput(t *testing.T) {
	assert.NoError(t, ValidateOutput("table"))
	assert.NoError(t, ValidateOutput("yaml"))
	assert.ErrorContains(t, ValidateOutput("json"), `unsupported output format "json"`)
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{in: nil, want: "(unset)"},
		{in: "api-aaa", want: `"api-aaa"`},
		{in: map[string]any{"replicas": float64(3)}, want: `{"replicas":3}`},
		{in: true, want: "true"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatValue(tt.in))
	}
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true}, {"YES\n", true}, {"n\n", false}, {"\n", false}, {"", false}, {"maybe\n", false},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		got, err := confirm(strings.NewReader(tt.input), &out, "Proceed?")
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, "input %q", tt.input)
		assert.Equal(t, "Proceed? [y/N]: ", out.String())
	}
}

// --- rollback flow ---

// fakeAPI records the rollback calls it receives.
type fakeAPI struct {
	calls   []bool   // the dryRun flag of each call
	targets []*int64 // the revision each call named; nil asks for the previous one
	preview *gen.RollbackResponse
	result  *gen.RollbackResponse
	err     error
}

func (f *fakeAPI) rollback(_ context.Context, revision *int64, dryRun bool) (*gen.RollbackResponse, error) {
	f.calls = append(f.calls, dryRun)
	f.targets = append(f.targets, revision)
	if f.err != nil {
		return nil, f.err
	}
	if dryRun {
		return f.preview, nil
	}
	return f.result, nil
}

// testKind and testBindingName name the binding the rollback and delete flows
// run against.
const (
	testKind        = "release binding"
	testBindingName = "api-dev"
)

func newFakeAPI() *fakeAPI {
	warnings := []gen.RollbackWarning{{Code: gen.TargetNotKnownGood, Message: "revision 1 never stayed Healthy for the known-good window"}}
	changes := []gen.RollbackSpecChange{{Field: "releaseName", Current: "api-ccc", Target: "api-aaa"}}
	resp := func(dryRun bool) *gen.RollbackResponse {
		return &gen.RollbackResponse{RevisionName: "api-dev-r1", Revision: 1, TargetRevision: 1, RevertedFrom: ptr(int64(3)),
			DryRun: dryRun, Changes: changes, Warnings: warnings}
	}
	return &fakeAPI{preview: resp(true), result: resp(false)}
}

func answer(yes bool, err error) (Confirmer, *int) {
	asked := 0
	return func(io.Writer, string) (bool, error) {
		asked++
		return yes, err
	}, &asked
}

func run(t *testing.T, api *fakeAPI, opts RollbackOptions) (string, error) {
	t.Helper()
	var out bytes.Buffer
	opts.Kind, opts.Binding, opts.Out = testKind, testBindingName, &out
	if opts.Revision == 0 {
		opts.Revision = 1
	}
	err := RunRollback(context.Background(), opts, api.rollback)
	return out.String(), err
}

func TestRunRollback_DryRunPreviewsOnly(t *testing.T) {
	api := newFakeAPI()
	confirmer, asked := answer(true, nil)

	out, err := run(t, api, RollbackOptions{DryRun: true, Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, api.calls, "only the dry-run call")
	assert.Zero(t, *asked, "no prompt for a dry run")
	assert.Contains(t, out, `Rolling back release binding "api-dev" from revision 3 to revision 1.`)
	assert.Contains(t, out, "No revision is created: api-dev-r1 becomes active again.")
	assert.Contains(t, out, `releaseName: "api-ccc" -> "api-aaa"`)
	assert.Contains(t, out, "! TargetNotKnownGood: revision 1 never stayed Healthy for the known-good window")
	assert.Contains(t, out, "Dry run: nothing was changed.")
}

func TestRunRollback_ConfirmedApplies(t *testing.T) {
	api := newFakeAPI()
	confirmer, asked := answer(true, nil)

	out, err := run(t, api, RollbackOptions{Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false}, api.calls, "preview, then apply")
	assert.Equal(t, 1, *asked)
	assert.Equal(t, 1, strings.Count(out, "! TargetNotKnownGood"), "warnings are shown once, before the prompt")
	assert.Contains(t, out, `Rolled back release binding "api-dev" to revision 1: api-dev-r1 is active again.`)
}

func TestRunRollback_DeclinedChangesNothing(t *testing.T) {
	api := newFakeAPI()
	confirmer, _ := answer(false, nil)

	out, err := run(t, api, RollbackOptions{Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, api.calls, "no apply call after a no")
	assert.Contains(t, out, "Rollback cancelled.")
}

func TestRunRollback_YesSkipsPromptAndPrintsWarnings(t *testing.T) {
	api := newFakeAPI()
	confirmer, asked := answer(false, nil)

	out, err := run(t, api, RollbackOptions{Yes: true, Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{false}, api.calls, "applied directly")
	assert.Zero(t, *asked)
	assert.Contains(t, out, "! TargetNotKnownGood")
	assert.Contains(t, out, "api-dev-r1 is active again")
}

func TestRunRollback_NoTerminalWithoutYesIsAnError(t *testing.T) {
	api := newFakeAPI()
	confirmer, _ := answer(false, errors.New("stdin is not a terminal; pass --yes to proceed without confirmation"))

	_, err := run(t, api, RollbackOptions{Confirm: confirmer})
	require.ErrorContains(t, err, "pass --yes")
	assert.Equal(t, []bool{true}, api.calls, "nothing applied")
}

func TestRunRollback_PreviewErrorStops(t *testing.T) {
	api := newFakeAPI()
	api.err = errors.New(`ComponentRelease "api-aaa" bound by revision 1 has been deleted`)
	confirmer, asked := answer(true, nil)

	_, err := run(t, api, RollbackOptions{Confirm: confirmer})
	require.ErrorContains(t, err, "has been deleted")
	assert.Zero(t, *asked)
}

func TestRunRollback_NoSpecChange(t *testing.T) {
	api := newFakeAPI()
	noChange := []gen.RollbackWarning{{Code: gen.NoSpecChange, Message: "revision 3 records the current spec"}}
	api.preview.Warnings, api.result.Warnings = noChange, noChange
	api.preview.Changes = nil
	for _, r := range []*gen.RollbackResponse{api.preview, api.result} {
		r.Revision, r.TargetRevision, r.RevisionName = 3, 3, "api-dev-r3"
	}
	confirmer, _ := answer(true, nil)

	out, err := run(t, api, RollbackOptions{Revision: 3, Confirm: confirmer})
	require.NoError(t, err)
	assert.Contains(t, out, "That revision is already active, so nothing will change.")
	assert.Contains(t, out, `Revision 3 (api-dev-r3) of release binding "api-dev" is already active; nothing changed.`)
	assert.NotContains(t, out, "Changes:")
}

func TestRunRollback_ShowsDisplayName(t *testing.T) {
	api := newFakeAPI()
	display := "api-aaa-r1"
	api.preview.RevisionDisplayName, api.result.RevisionDisplayName = &display, &display
	confirmer, _ := answer(true, nil)

	out, err := run(t, api, RollbackOptions{Confirm: confirmer})
	require.NoError(t, err)
	assert.Contains(t, out, "No revision is created: api-aaa-r1 becomes active again.")
	assert.Contains(t, out, `Rolled back release binding "api-dev" to revision 1: api-aaa-r1 is active again.`)
}

func TestDisplayName(t *testing.T) {
	assert.Equal(t, "frontend-r1", DisplayName("frontend", 1))
}

func TestRunRollback_NegativeRevisionRejected(t *testing.T) {
	api := newFakeAPI()
	err := RunRollback(context.Background(), RollbackOptions{Revision: -1, Out: io.Discard}, api.rollback)
	require.ErrorContains(t, err, "--revision must be at least 1")
	assert.Empty(t, api.calls)
}

// runImplicit runs RunRollback without a revision.
func runImplicit(t *testing.T, api *fakeAPI, opts RollbackOptions) (string, error) {
	t.Helper()
	var out bytes.Buffer
	opts.Kind, opts.Binding, opts.Out = testKind, testBindingName, &out
	err := RunRollback(context.Background(), opts, api.rollback)
	return out.String(), err
}

func TestRunRollback_ImplicitPreviewsThenAppliesTheConfirmedTarget(t *testing.T) {
	api := newFakeAPI()
	confirmer, asked := answer(true, nil)
	out, err := runImplicit(t, api, RollbackOptions{Confirm: confirmer})
	require.NoError(t, err)

	assert.Equal(t, []bool{true, false}, api.calls)
	require.Len(t, api.targets, 2)
	assert.Nil(t, api.targets[0], "the preview asks the server for the previous revision")
	require.NotNil(t, api.targets[1])
	assert.Equal(t, int64(1), *api.targets[1], "the rollback is applied to the previewed target by number")
	assert.Equal(t, 1, *asked)
	assert.Contains(t, out, `Rolling back release binding "api-dev" from revision 3 to revision 1.`)
	assert.Contains(t, out, "Target: api-dev-r1, the revision active before the current one (no --revision given).")
	assert.Contains(t, out, `Rolled back release binding "api-dev" to revision 1`)
}

func TestRunRollback_ImplicitDeclinedChangesNothing(t *testing.T) {
	api := newFakeAPI()
	confirmer, _ := answer(false, nil)
	out, err := runImplicit(t, api, RollbackOptions{Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, api.calls)
	assert.Contains(t, out, "Rollback cancelled.")
}

func TestRunRollback_ImplicitDryRun(t *testing.T) {
	api := newFakeAPI()
	out, err := runImplicit(t, api, RollbackOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, api.calls)
	assert.Nil(t, api.targets[0])
	assert.Contains(t, out, "the revision active before the current one")
	assert.Contains(t, out, "Dry run: nothing was changed.")
}

func TestRunRollback_ImplicitWithYesIsOneServerDecision(t *testing.T) {
	api := newFakeAPI()
	confirmer, asked := answer(false, nil)
	out, err := runImplicit(t, api, RollbackOptions{Yes: true, Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{false}, api.calls)
	assert.Nil(t, api.targets[0], "the server resolves and applies the previous revision in one request")
	assert.Equal(t, 0, *asked)
	assert.Contains(t, out, "Rolling back to revision 1, the previously active revision.")
}

func TestRunRollback_ImplicitWithoutPreviousRevisionFails(t *testing.T) {
	api := newFakeAPI()
	api.err = errors.New("No previous revision to roll back to: the previously active revision has been deleted")
	confirmer, asked := answer(true, nil)
	out, err := runImplicit(t, api, RollbackOptions{Confirm: confirmer})
	require.ErrorContains(t, err, "No previous revision to roll back to")
	assert.Equal(t, 0, *asked, "nothing is offered")
	assert.Equal(t, []bool{true}, api.calls)
	assert.NotContains(t, out, "Rolled back")
}

func TestRunRollback_ImplicitWithoutTerminalNeedsYes(t *testing.T) {
	api := newFakeAPI()
	confirmer, _ := answer(false, errors.New("stdin is not a terminal; pass --yes to proceed without confirmation"))
	_, err := runImplicit(t, api, RollbackOptions{Confirm: confirmer})
	require.ErrorContains(t, err, "pass --yes")
	assert.Equal(t, []bool{true}, api.calls, "nothing is applied")
}

func TestRunRollback_NotesAnActiveRevisionThatChangedAfterThePreview(t *testing.T) {
	api := newFakeAPI()
	api.result.RevertedFrom = ptr(int64(5))
	confirmer, _ := answer(true, nil)
	out, err := runImplicit(t, api, RollbackOptions{Confirm: confirmer})
	require.NoError(t, err)
	assert.Contains(t, out, "Note: the active revision changed from 3 to 5 after the preview; "+
		"the rollback went to revision 1 as confirmed.")
}

func TestRunRollback_ExplicitRevisionIsSentAsGiven(t *testing.T) {
	api := newFakeAPI()
	confirmer, _ := answer(true, nil)
	_, err := run(t, api, RollbackOptions{Revision: 1, Confirm: confirmer})
	require.NoError(t, err)
	for _, target := range api.targets {
		require.NotNil(t, target)
		assert.Equal(t, int64(1), *target)
	}
}

func TestGetRollbackRevision(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "rollback"}
		AddRollbackRevision(cmd)
		return cmd
	}

	cmd := newCmd()
	got, err := GetRollbackRevision(cmd)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got, "no --revision: the previously active revision")

	cmd = newCmd()
	require.NoError(t, cmd.Flags().Set("revision", "3"))
	got, err = GetRollbackRevision(cmd)
	require.NoError(t, err)
	assert.Equal(t, int64(3), got)

	cmd = newCmd()
	require.NoError(t, cmd.Flags().Set("revision", "0"))
	_, err = GetRollbackRevision(cmd)
	require.ErrorContains(t, err, "--revision must be at least 1", "--revision 0 is not a request for the previous revision")
}

func TestParseRevisionName(t *testing.T) {
	valid := map[string]int64{
		"frontend-development-r3":    3,  // object name
		"frontend-v7-r12":            12, // display name
		"frontend-20260803-1-r4":     4,  // release name with digits and dashes
		"my-r-service-production-r7": 7,  // "-r" inside the binding name
		"r3":                         3,
		"3":                          3,
	}
	for name, want := range valid {
		got, err := ParseRevisionName(name)
		require.NoError(t, err, name)
		assert.Equal(t, want, got, name)
	}
	n, err := ParseRevisionName(" r5 ")
	require.NoError(t, err, "surrounding spaces are ignored")
	assert.Equal(t, int64(5), n)

	for _, name := range []string{"", "r", "r0", "0", "frontend", "frontend-r", "frontend-r03", "frontend-rx", "-r3", "frontend-r3a"} {
		_, err := ParseRevisionName(name)
		assert.Error(t, err, name)
	}
}

func TestRevisionFromArgs(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "get"}
		AddRevision(cmd)
		return cmd
	}

	cmd := newCmd()
	n, err := RevisionFromArgs(cmd, []string{"api-dev-r4"})
	require.NoError(t, err)
	assert.Equal(t, int64(4), n, "the name alone names the revision")

	cmd = newCmd()
	require.NoError(t, cmd.Flags().Set("revision", "4"))
	n, err = RevisionFromArgs(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(4), n, "--revision alone still works")

	n, err = RevisionFromArgs(cmd, []string{"api-dev-r4"})
	require.NoError(t, err)
	assert.Equal(t, int64(4), n, "name and --revision may both be given when they agree")

	_, err = RevisionFromArgs(cmd, []string{"api-dev-r5"})
	assert.ErrorContains(t, err, "is revision 5, but --revision is 4")

	_, err = RevisionFromArgs(newCmd(), []string{"api-dev"})
	assert.ErrorContains(t, err, "ending in -r<N>")
}

// deleteAPI records the delete calls RunDelete makes and answers each with the
// error configured for a dry run or a real delete.
type deleteAPI struct {
	calls              []bool // dryRun of each call
	dryRunErr, realErr error
}

func (d *deleteAPI) del(_ context.Context, dryRun bool) (*gen.DeleteBindingRevisionResponse, error) {
	d.calls = append(d.calls, dryRun)
	if dryRun && d.dryRunErr != nil {
		return nil, d.dryRunErr
	}
	if !dryRun && d.realErr != nil {
		return nil, d.realErr
	}
	name := "api-v2-r2"
	return &gen.DeleteBindingRevisionResponse{RevisionName: "api-dev-r2", RevisionDisplayName: &name, Revision: 2, DryRun: dryRun}, nil
}

func runDelete(t *testing.T, api *deleteAPI, opts DeleteOptions) (string, error) {
	t.Helper()
	var out bytes.Buffer
	opts.Kind, opts.Binding, opts.Revision, opts.Out = testKind, testBindingName, 2, &out
	opts.Target = Row{Name: "api-v2-r2", Cause: "Deploy", Health: "Failed"}
	err := RunDelete(context.Background(), opts, api.del)
	return out.String(), err
}

func TestRunDelete_ConfirmedDeletesAfterTheServerCheck(t *testing.T) {
	api := &deleteAPI{}
	confirmer, asked := answer(true, nil)
	out, err := runDelete(t, api, DeleteOptions{Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false}, api.calls, "checked with a dry run first, then deleted")
	assert.Equal(t, 1, *asked)
	assert.Contains(t, out, `Revision:    api-v2-r2 (revision 2 of release binding "api-dev")`)
	assert.Contains(t, out, "Status:      Inactive")
	assert.Contains(t, out, "Cause:       Deploy")
	assert.Contains(t, out, "Successfully deleted revision 2: api-v2-r2")
}

func TestRunDelete_DeclinedDeletesNothing(t *testing.T) {
	api := &deleteAPI{}
	confirmer, _ := answer(false, nil)
	out, err := runDelete(t, api, DeleteOptions{Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, api.calls)
	assert.Contains(t, out, "Deletion cancelled.")
	assert.NotContains(t, out, "Successfully")
}

func TestRunDelete_YesSkipsTheQuestion(t *testing.T) {
	api := &deleteAPI{}
	confirmer, asked := answer(false, nil)
	out, err := runDelete(t, api, DeleteOptions{Yes: true, Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, 0, *asked)
	assert.Equal(t, []bool{true, false}, api.calls)
	assert.Contains(t, out, "Successfully deleted revision 2")
}

func TestRunDelete_DryRunOnlyChecks(t *testing.T) {
	api := &deleteAPI{}
	confirmer, asked := answer(true, nil)
	out, err := runDelete(t, api, DeleteOptions{DryRun: true, Confirm: confirmer})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, api.calls)
	assert.Equal(t, 0, *asked)
	assert.Contains(t, out, "Dry run: the revision can be deleted; nothing was changed.")
}

func TestRunDelete_RefusedBeforeAsking(t *testing.T) {
	api := &deleteAPI{dryRunErr: errors.New("Cannot delete revision 2 because it is currently active")}
	confirmer, asked := answer(true, nil)
	out, err := runDelete(t, api, DeleteOptions{Confirm: confirmer})
	require.EqualError(t, err, "Cannot delete revision 2 because it is currently active")
	assert.Equal(t, 0, *asked, "a refused deletion is not offered")
	assert.Equal(t, []bool{true}, api.calls)
	assert.NotContains(t, out, "Successfully")
}

func TestRunDelete_FailureIsNotReportedAsSuccess(t *testing.T) {
	api := &deleteAPI{realErr: errors.New("Revision \"api-dev-r2\" changed while it was being deleted; retry")}
	_, asked := answer(true, nil)
	out, err := runDelete(t, api, DeleteOptions{Yes: true})
	require.ErrorContains(t, err, "changed while it was being deleted")
	assert.Equal(t, 0, *asked)
	assert.NotContains(t, out, "Successfully")
}

func TestRunDelete_NoTerminalNeedsYes(t *testing.T) {
	api := &deleteAPI{}
	confirmer, _ := answer(false, errors.New("stdin is not a terminal; pass --yes to proceed without confirmation"))
	_, err := runDelete(t, api, DeleteOptions{Confirm: confirmer})
	require.ErrorContains(t, err, "pass --yes")
	assert.Equal(t, []bool{true}, api.calls, "nothing is deleted")
}
