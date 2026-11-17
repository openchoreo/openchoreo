// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	revNS      = "ns"
	revOwner   = "api"
	revEnv     = "dev"
	revBinding = revOwner + "-" + revEnv
)

// expectRevisionHistory sets up a binding with revisions 2 (active) and 1.
func expectRevisionHistory(mc *mocks.MockInterface) {
	spec := &gen.ReleaseBindingSpec{Environment: revEnv}
	spec.Owner.ComponentName = revOwner
	spec.Owner.ProjectName = "shop"
	mc.EXPECT().GetReleaseBinding(mock.Anything, revNS, revBinding).
		Return(&gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: revBinding}, Spec: spec}, nil)

	rev := func(n int64, release string, active bool) gen.ReleaseBindingRevision {
		created := time.Now().Add(-time.Duration(n) * time.Hour)
		h := gen.BindingRevisionStatusHealth("Healthy")
		knownGood := !active
		r := gen.ReleaseBindingRevision{
			Metadata: gen.ObjectMeta{Name: fmt.Sprintf("%s-r%d", revBinding, n), CreationTimestamp: &created},
			Spec:     gen.ReleaseBindingRevisionSpec{Revision: n, ComponentReleaseName: release, Cause: "Deploy", SnapshotHash: "h"},
			Status:   &gen.BindingRevisionStatus{Health: &h, Active: &active, KnownGood: &knownGood},
		}
		r.Spec.BindingSpec.Owner.ComponentName = revOwner
		return r
	}
	mc.EXPECT().ListReleaseBindingRevisions(mock.Anything, revNS, mock.MatchedBy(func(p *gen.ListReleaseBindingRevisionsParams) bool {
		return p.Component == revOwner && p.Environment != nil && *p.Environment == revEnv
	})).Return(&gen.ReleaseBindingRevisionList{Items: []gen.ReleaseBindingRevision{
		rev(2, "api-bbb", true),
		rev(1, "api-aaa", false),
	}}, nil)
}

func revDeleteParams(dryRun bool) any {
	return mock.MatchedBy(func(p *gen.DeleteReleaseBindingRevisionParams) bool {
		return p.Binding != nil && *p.Binding == revBinding && p.DryRun != nil && *p.DryRun == dryRun
	})
}

func revRollbackResp(dryRun bool) *gen.RollbackResponse {
	return &gen.RollbackResponse{
		RevisionName: revBinding + "-r1", Revision: 1, TargetRevision: 1, DryRun: dryRun,
		Changes: []gen.RollbackSpecChange{{Field: "releaseName", Current: "api-bbb", Target: "api-aaa"}},
	}
}

// revRollbackReq matches a rollback request: revision 0 means none was named.
func revRollbackReq(revision int64, dryRun bool) any {
	return mock.MatchedBy(func(r gen.RollbackRequest) bool {
		if r.DryRun == nil || *r.DryRun != dryRun {
			return false
		}
		if revision == 0 {
			return r.Revision == nil
		}
		return r.Revision != nil && *r.Revision == revision
	})
}

// The short commands are reachable under "occ component".
func TestRevisionCmds_Registered(t *testing.T) {
	root := NewComponentCmd(errFactory("unused"))
	for _, path := range [][]string{
		{"revisions"}, {"revision", "get"}, {"revision", "delete"}, {"rollback"},
	} {
		sub, rest, err := root.Find(path)
		require.NoError(t, err, path)
		assert.Empty(t, rest, path)
		assert.Equal(t, path[len(path)-1], sub.Name())
	}

	revision, _, _ := root.Find([]string{"revision"})
	names := make([]string, 0, len(revision.Commands()))
	for _, sub := range revision.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"get", "delete"}, names)
}

func TestRevisionCmds_Flags(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		not   []string
	}{
		{name: "revisions", flags: []string{"namespace"}, not: []string{"binding"}},
		{name: "get", flags: []string{"namespace", "output"}, not: []string{"binding", "revision"}},
		{name: "delete", flags: []string{"namespace", "dry-run", "yes"}, not: []string{"binding", "revision"}},
		{name: "rollback", flags: []string{"namespace", "revision", "dry-run", "yes", "message"}, not: []string{"to"}},
	}
	f := errFactory("unused")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var flags = map[string]func(string) bool{
				"revisions": func(n string) bool { return newRevisionsCmd(f).Flags().Lookup(n) != nil },
				"get":       func(n string) bool { return newRevisionGetCmd(f).Flags().Lookup(n) != nil },
				"delete":    func(n string) bool { return newRevisionDeleteCmd(f).Flags().Lookup(n) != nil },
				"rollback":  func(n string) bool { return newRollbackCmd(f).Flags().Lookup(n) != nil },
			}[tt.name]
			for _, n := range tt.flags {
				assert.True(t, flags(n), "flag --%s", n)
			}
			for _, n := range tt.not {
				assert.False(t, flags(n), "flag --%s must not exist", n)
			}
		})
	}
	assert.Equal(t, "o", newRevisionGetCmd(f).Flags().Lookup("output").Shorthand)
	assert.Equal(t, "m", newRollbackCmd(f).Flags().Lookup("message").Shorthand)
}

func TestRevisionCmds_Args(t *testing.T) {
	f := errFactory("unused")
	one := []string{revBinding}
	two := []string{revBinding, "2"}

	revisions := newRevisionsCmd(f)
	require.NoError(t, revisions.Args(revisions, one))
	require.ErrorContains(t, revisions.Args(revisions, nil), "BINDING_NAME")
	require.Error(t, revisions.Args(revisions, two))

	rollback := newRollbackCmd(f)
	require.NoError(t, rollback.Args(rollback, one))
	require.ErrorContains(t, rollback.Args(rollback, nil), "BINDING_NAME")
	require.Error(t, rollback.Args(rollback, two))

	for _, cmd := range []*cobra.Command{newRevisionGetCmd(f), newRevisionDeleteCmd(f)} {
		require.NoError(t, cmd.Args(cmd, two), cmd.Name())
		require.Error(t, cmd.Args(cmd, one), cmd.Name())
		require.Error(t, cmd.Args(cmd, nil), cmd.Name())
		require.Error(t, cmd.Args(cmd, []string{revBinding, "2", "3"}), cmd.Name())
	}
}

func TestRevisionCmds_FactoryError(t *testing.T) {
	f := errFactory("factory failed")
	for name, run := range map[string]func() error{
		"revisions": func() error { c := newRevisionsCmd(f); return c.RunE(c, []string{revBinding}) },
		"get":       func() error { c := newRevisionGetCmd(f); return c.RunE(c, []string{revBinding, "1"}) },
		"delete":    func() error { c := newRevisionDeleteCmd(f); return c.RunE(c, []string{revBinding, "1"}) },
		"rollback":  func() error { c := newRollbackCmd(f); return c.RunE(c, []string{revBinding}) },
	} {
		assert.EqualError(t, run(), "factory failed", name)
	}
}

func TestRevisionsCmd_ListsTheBindingsRevisions(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectRevisionHistory(mc)

	cmd := newRevisionsCmd(mockFactory(mc))
	require.NoError(t, cmd.ParseFlags([]string{"-n", revNS}))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{revBinding}))
	})
	assert.Contains(t, out, "KNOWN-GOOD")
	assert.Contains(t, out, "api-r2")
	assert.Contains(t, out, "api-r1")
}

func TestRevisionsCmd_BindingNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBinding(mock.Anything, revNS, "missing").Return(nil, nil)

	cmd := newRevisionsCmd(mockFactory(mc))
	require.NoError(t, cmd.ParseFlags([]string{"-n", revNS}))
	err := cmd.RunE(cmd, []string{"missing"})
	assert.EqualError(t, err, `release binding "missing" not found in namespace "ns"`)
}

func TestRevisionGetCmd(t *testing.T) {
	// The revision can be given as a number, r<N> or a revision name.
	for _, ref := range []string{"1", "r1", revBinding + "-r1", "api-r1"} {
		t.Run("table "+ref, func(t *testing.T) {
			mc := mocks.NewMockInterface(t)
			expectRevisionHistory(mc)

			cmd := newRevisionGetCmd(mockFactory(mc))
			require.NoError(t, cmd.ParseFlags([]string{"-n", revNS}))
			out := testutil.CaptureStdout(t, func() {
				require.NoError(t, cmd.RunE(cmd, []string{revBinding, ref}))
			})
			assert.Contains(t, out, "api-r1")
			assert.NotContains(t, out, "api-r2")
		})
	}

	t.Run("yaml", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		expectRevisionHistory(mc)

		cmd := newRevisionGetCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "-o", "yaml"}))
		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, cmd.RunE(cmd, []string{revBinding, "1"}))
		})
		assert.Contains(t, out, "componentReleaseName: api-aaa")
	})

	t.Run("revision not found", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		expectRevisionHistory(mc)

		cmd := newRevisionGetCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS}))
		err := cmd.RunE(cmd, []string{revBinding, "9"})
		assert.EqualError(t, err, `revision 9 of release binding "api-dev" not found`)
	})

	t.Run("bad revision", func(t *testing.T) {
		cmd := newRevisionGetCmd(mockFactory(mocks.NewMockInterface(t)))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS}))
		for _, ref := range []string{"abc", "0", "r0", "-1"} {
			require.ErrorContains(t, cmd.RunE(cmd, []string{revBinding, ref}), "revision name", ref)
		}
	})

	t.Run("bad output format", func(t *testing.T) {
		cmd := newRevisionGetCmd(mockFactory(mocks.NewMockInterface(t)))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "-o", "json"}))
		require.ErrorContains(t, cmd.RunE(cmd, []string{revBinding, "1"}), "unsupported output format")
	})
}

func TestRevisionDeleteCmd(t *testing.T) {
	name := revBinding + "-r1"

	t.Run("dry run does not delete", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		expectRevisionHistory(mc)
		mc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, revNS, name, revDeleteParams(true)).
			Return(&gen.DeleteBindingRevisionResponse{RevisionName: name, Revision: 1, DryRun: true}, nil).Once()

		cmd := newRevisionDeleteCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--dry-run"}))
		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, cmd.RunE(cmd, []string{revBinding, "1"}))
		})
		assert.Contains(t, out, "Dry run: the revision can be deleted; nothing was changed.")
	})

	t.Run("yes deletes without a prompt", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		expectRevisionHistory(mc)
		mc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, revNS, name, revDeleteParams(true)).
			Return(&gen.DeleteBindingRevisionResponse{RevisionName: name, Revision: 1, DryRun: true}, nil).Once()
		mc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, revNS, name, revDeleteParams(false)).
			Return(&gen.DeleteBindingRevisionResponse{RevisionName: name, Revision: 1}, nil).Once()

		cmd := newRevisionDeleteCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--yes"}))
		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, cmd.RunE(cmd, []string{revBinding, "r1"}))
		})
		assert.Contains(t, out, "Successfully deleted revision 1")
	})

	t.Run("the server's refusal is returned", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		expectRevisionHistory(mc)
		mc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, revNS, revBinding+"-r2", revDeleteParams(true)).
			Return(nil, fmt.Errorf("Cannot delete revision 2 because it is currently active")).Once()

		cmd := newRevisionDeleteCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--yes"}))
		var err error
		testutil.CaptureStdout(t, func() { err = cmd.RunE(cmd, []string{revBinding, "2"}) })
		assert.EqualError(t, err, "Cannot delete revision 2 because it is currently active")
	})

	t.Run("revision not found", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		expectRevisionHistory(mc)

		cmd := newRevisionDeleteCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--yes"}))
		require.ErrorContains(t, cmd.RunE(cmd, []string{revBinding, "9"}), "revision 9 of release binding")
	})

	t.Run("bad revision", func(t *testing.T) {
		cmd := newRevisionDeleteCmd(mockFactory(mocks.NewMockInterface(t)))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--yes"}))
		require.ErrorContains(t, cmd.RunE(cmd, []string{revBinding, "abc"}), "revision name")
	})
}

func TestRollbackCmd(t *testing.T) {
	t.Run("dry run to a named revision", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackReleaseBinding(mock.Anything, revNS, revBinding, revRollbackReq(1, true)).
			Return(revRollbackResp(true), nil).Once()

		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--revision", "1", "--dry-run"}))
		out := testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{revBinding})) })
		assert.Contains(t, out, `releaseName: "api-bbb" -> "api-aaa"`)
		assert.Contains(t, out, "Dry run: nothing was changed.")
	})

	t.Run("yes applies without a prompt", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackReleaseBinding(mock.Anything, revNS, revBinding, revRollbackReq(1, false)).
			Return(revRollbackResp(false), nil).Once()

		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--revision", "1", "--yes"}))
		out := testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{revBinding})) })
		assert.Contains(t, out, revBinding+"-r1 is active again")
	})

	t.Run("without --revision: the previously active revision", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackReleaseBinding(mock.Anything, revNS, revBinding, revRollbackReq(0, true)).
			Return(revRollbackResp(true), nil).Once()

		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--dry-run"}))
		out := testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{revBinding})) })
		assert.Contains(t, out, "Dry run: nothing was changed.")
	})

	t.Run("the message is sent with the rollback", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackReleaseBinding(mock.Anything, revNS, revBinding, mock.MatchedBy(func(r gen.RollbackRequest) bool {
			return r.Message != nil && *r.Message == "r2 broke login"
		})).Return(revRollbackResp(false), nil).Once()

		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--revision", "1", "--yes", "-m", "r2 broke login"}))
		testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{revBinding})) })
	})

	t.Run("--revision 0 is refused", func(t *testing.T) {
		cmd := newRollbackCmd(mockFactory(mocks.NewMockInterface(t)))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--revision", "0"}))
		require.ErrorContains(t, cmd.RunE(cmd, []string{revBinding}), "--revision must be at least 1")
	})

	t.Run("the server's refusal is returned", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		mc.EXPECT().RollbackReleaseBinding(mock.Anything, revNS, revBinding, mock.Anything).
			Return(nil, fmt.Errorf("binding is managed by GitOps")).Once()

		cmd := newRollbackCmd(mockFactory(mc))
		require.NoError(t, cmd.ParseFlags([]string{"-n", revNS, "--revision", "1", "--yes"}))
		var err error
		testutil.CaptureStdout(t, func() { err = cmd.RunE(cmd, []string{revBinding}) })
		require.ErrorContains(t, err, "binding is managed by GitOps")
	})
}
