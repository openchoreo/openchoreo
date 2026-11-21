// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestRollback(t *testing.T) {
	const ns, binding = "ns", "db-dev"
	resp := func(dryRun bool) *gen.RollbackResponse {
		return &gen.RollbackResponse{RevisionName: "db-dev-r1", Revision: 1, TargetRevision: 1, DryRun: dryRun,
			Changes:  []gen.RollbackSpecChange{{Field: "resourceRelease", Current: "db-bbb", Target: "db-aaa"}},
			Warnings: []gen.RollbackWarning{{Code: gen.TargetNotKnownGood, Message: "revision 1 never stayed Healthy for the known-good window"}}}
	}
	dry := func(want bool) any {
		return mock.MatchedBy(func(r gen.RollbackRequest) bool {
			return r.Revision != nil && *r.Revision == 1 && r.DryRun != nil && *r.DryRun == want
		})
	}

	t.Run("confirmed", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding, dry(true)).Return(resp(true), nil).Once()
		mc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding, dry(false)).Return(resp(false), nil).Once()
		r := New(mc)
		r.confirm = func(io.Writer, string) (bool, error) { return true, nil }

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, r.Rollback(RollbackParams{Namespace: ns, ResourceReleaseBindingName: binding, Revision: 1}))
		})
		assert.Contains(t, out, `resourceRelease: "db-bbb" -> "db-aaa"`)
		assert.Contains(t, out, `Rolled back resource release binding "db-dev" to revision 1`)
	})

	t.Run("declined", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding, dry(true)).Return(resp(true), nil).Once()
		r := New(mc)
		r.confirm = func(io.Writer, string) (bool, error) { return false, nil }

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, r.Rollback(RollbackParams{Namespace: ns, ResourceReleaseBindingName: binding, Revision: 1}))
		})
		assert.Contains(t, out, "Rollback cancelled.")
	})

	t.Run("dry run via the command", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding, dry(true)).Return(resp(true), nil).Once()
		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", ns, "--revision", "1", "--dry-run"}))

		out := testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{binding})) })
		assert.Contains(t, out, "! TargetNotKnownGood")
		assert.Contains(t, out, "Dry run: nothing was changed.")
	})

	t.Run("yes via the command", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding, dry(false)).Return(resp(false), nil).Once()
		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", ns, "--revision", "1", "--yes"}))

		out := testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{binding})) })
		assert.Contains(t, out, "db-dev-r1 is active again")
	})

	t.Run("without --revision via the command: the previously active revision", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding, mock.MatchedBy(func(r gen.RollbackRequest) bool {
			return r.Revision == nil && r.DryRun != nil && *r.DryRun
		})).Return(resp(true), nil).Once()
		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", ns, "--dry-run"}))
		out := testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{binding})) })
		assert.Contains(t, out, "the revision active before the current one (no --revision given)")
	})

	t.Run("without --revision and --yes: one request", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding, mock.MatchedBy(func(r gen.RollbackRequest) bool {
			return r.Revision == nil && r.DryRun != nil && !*r.DryRun
		})).Return(resp(false), nil).Once()
		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", ns, "--yes"}))
		out := testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{binding})) })
		assert.Contains(t, out, "Rolling back to revision 1, the previously active revision.")
	})
}
