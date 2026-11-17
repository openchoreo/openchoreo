// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	rollbackNS      = "ns"
	rollbackBinding = "api-dev"
)

func rollbackResp(dryRun bool) *gen.RollbackResponse {
	from := int64(3)
	return &gen.RollbackResponse{
		RevisionName: "api-dev-r1", Revision: 1, TargetRevision: 1, RevertedFrom: &from, DryRun: dryRun,
		Changes:  []gen.RollbackSpecChange{{Field: "releaseName", Current: "api-ccc", Target: "api-aaa"}},
		Warnings: []gen.RollbackWarning{{Code: gen.ProjectCellDrift, Message: "the project's cell moved"}},
	}
}

func isDryRun(want bool) any {
	return mock.MatchedBy(func(r gen.RollbackRequest) bool {
		return r.Revision != nil && *r.Revision == 1 && r.DryRun != nil && *r.DryRun == want
	})
}

func TestRollback_DryRun(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, isDryRun(true)).Return(rollbackResp(true), nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding, Revision: 1, DryRun: true}))
	})
	assert.Contains(t, out, `releaseName: "api-ccc" -> "api-aaa"`)
	assert.Contains(t, out, "! ProjectCellDrift: the project's cell moved")
	assert.Contains(t, out, "Dry run: nothing was changed.")
}

func TestRollback_ConfirmedAppliesAfterPreview(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, isDryRun(true)).Return(rollbackResp(true), nil).Once()
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, isDryRun(false)).Return(rollbackResp(false), nil).Once()

	r := New(mc)
	var question string
	r.confirm = func(_ io.Writer, q string) (bool, error) { question = q; return true, nil }

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding, Revision: 1}))
	})
	assert.Contains(t, question, `Roll back release binding "api-dev" to revision 1?`)
	assert.Contains(t, out, `to revision 1: api-dev-r1 is active again`)
}

func TestRollback_DeclinedDoesNotApply(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, isDryRun(true)).Return(rollbackResp(true), nil).Once()

	r := New(mc)
	r.confirm = func(io.Writer, string) (bool, error) { return false, nil }

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding, Revision: 1}))
	})
	assert.Contains(t, out, "Rollback cancelled.")
}

func TestRollback_YesAppliesWithoutPrompt(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, isDryRun(false)).Return(rollbackResp(false), nil).Once()

	r := New(mc)
	r.confirm = func(io.Writer, string) (bool, error) { t.Fatal("must not prompt with --yes"); return false, nil }

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding, Revision: 1, Yes: true}))
	})
	assert.Contains(t, out, "! ProjectCellDrift")
	assert.Contains(t, out, "api-dev-r1 is active again")
}

func TestRollback_ServerConflict(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, mock.Anything).
		Return(nil, fmt.Errorf(`release bound by the revision has been deleted: ComponentRelease "api-aaa" bound by revision 1`))

	err := New(mc).Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding, Revision: 1, Yes: true})
	assert.ErrorContains(t, err, `ComponentRelease "api-aaa"`)
}

func TestRollback_MissingNamespace(t *testing.T) {
	err := New(mocks.NewMockInterface(t)).Rollback(RollbackParams{ReleaseBindingName: rollbackBinding, Revision: 1})
	assert.ErrorContains(t, err, "namespace")
}

// The binding is the command's argument; --revision, --dry-run and --yes are flags.
func TestRollbackCmd(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, isDryRun(false)).Return(rollbackResp(false), nil).Once()

	cmd := newRollbackCmd(mockFactory(mc))
	require.NoError(t, cmd.ParseFlags([]string{"-n", rollbackNS, "--revision", "1", "--yes"}))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{rollbackBinding}))
	})
	assert.Contains(t, out, "Rolled back release binding")

	dry := newRollbackCmd(mockFactory(mc))
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, isDryRun(true)).Return(rollbackResp(true), nil).Once()
	require.NoError(t, dry.ParseFlags([]string{"-n", rollbackNS, "--revision", "1", "--dry-run"}))
	out = testutil.CaptureStdout(t, func() {
		require.NoError(t, dry.RunE(dry, []string{rollbackBinding}))
	})
	assert.Contains(t, out, "Dry run: nothing was changed.")

	noArg := newRollbackCmd(mockFactory(mc))
	assert.Error(t, noArg.Args(noArg, nil), "the binding name is required")
}

func TestRollback_WithoutRevisionGoesToThePreviouslyActiveRevision(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	implicitPreview := mock.MatchedBy(func(r gen.RollbackRequest) bool {
		return r.Revision == nil && r.DryRun != nil && *r.DryRun
	})
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, implicitPreview).Return(rollbackResp(true), nil).Once()
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, isDryRun(false)).Return(rollbackResp(false), nil).Once()

	r := New(mc)
	var question string
	r.confirm = func(_ io.Writer, q string) (bool, error) { question = q; return true, nil }

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding}))
	})
	assert.Contains(t, out, "the revision active before the current one (no --revision given)")
	assert.Contains(t, question, `Roll back release binding "api-dev" to revision 1?`)
	assert.Contains(t, out, "to revision 1: api-dev-r1 is active again")
}

func TestRollback_WithoutRevisionAndNoPreviousRevision(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().RollbackReleaseBinding(mock.Anything, rollbackNS, rollbackBinding, mock.Anything).
		Return(nil, fmt.Errorf("No previous revision to roll back to: no revision was active before the current one")).Once()

	r := New(mc)
	r.confirm = func(io.Writer, string) (bool, error) { t.Fatal("must not ask"); return false, nil }
	err := r.Rollback(RollbackParams{Namespace: rollbackNS, ReleaseBindingName: rollbackBinding})
	require.ErrorContains(t, err, "No previous revision to roll back to")
}

func TestRollbackCmd_RevisionZeroIsRejected(t *testing.T) {
	mc := mocks.NewMockInterface(t) // no API call is expected
	cmd := newRollbackCmd(func() (client.Interface, error) { return mc, nil })
	cmd.PreRunE = nil
	require.NoError(t, cmd.Flags().Set("revision", "0"))
	require.ErrorContains(t, cmd.RunE(cmd, []string{rollbackBinding}), "--revision must be at least 1")
}
