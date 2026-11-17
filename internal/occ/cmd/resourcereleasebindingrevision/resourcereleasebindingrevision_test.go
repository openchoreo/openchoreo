// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebindingrevision

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	testNS      = "ns"
	testBinding = "db-dev"
)

func mockFactory(mc *mocks.MockInterface) client.NewClientFunc {
	return func() (client.Interface, error) { return mc, nil }
}

func errFactory(msg string) client.NewClientFunc {
	return func() (client.Interface, error) { return nil, fmt.Errorf("%s", msg) }
}

func expectHistory(mc *mocks.MockInterface) {
	spec := &gen.ResourceReleaseBindingSpec{Environment: "dev"}
	spec.Owner.ResourceName = "db"
	spec.Owner.ProjectName = "shop"
	mc.EXPECT().GetResourceReleaseBinding(mock.Anything, testNS, testBinding).
		Return(&gen.ResourceReleaseBinding{Metadata: gen.ObjectMeta{Name: testBinding}, Spec: spec}, nil)

	rev := func(n int64, release, health string, active, knownGood bool) gen.ResourceReleaseBindingRevision {
		created := time.Now().Add(-time.Duration(n) * time.Hour)
		h := gen.BindingRevisionStatusHealth(health)
		r := gen.ResourceReleaseBindingRevision{
			Metadata: gen.ObjectMeta{Name: fmt.Sprintf("%s-r%d", testBinding, n), CreationTimestamp: &created},
			Spec:     gen.ResourceReleaseBindingRevisionSpec{Revision: n, ResourceReleaseName: release, Cause: "Deploy", SnapshotHash: "h"},
			Status:   &gen.BindingRevisionStatus{Health: &h, Active: &active, KnownGood: &knownGood},
		}
		r.Spec.BindingSpec.Owner.ResourceName = "db"
		return r
	}
	mc.EXPECT().ListResourceReleaseBindingRevisions(mock.Anything, testNS, mock.MatchedBy(func(p *gen.ListResourceReleaseBindingRevisionsParams) bool {
		return p.Resource == "db" && p.Environment != nil && *p.Environment == "dev"
	})).Return(&gen.ResourceReleaseBindingRevisionList{Items: []gen.ResourceReleaseBindingRevision{
		rev(2, "db-bbb", "Healthy", true, false),
		rev(1, "db-aaa", "Healthy", false, true),
	}}, nil)
}

func TestNewResourceReleaseBindingRevisionCmd(t *testing.T) {
	cmd := NewResourceReleaseBindingRevisionCmd(errFactory("unused"))
	assert.Equal(t, "resourcereleasebindingrevision", cmd.Use)
	assert.ElementsMatch(t, []string{"resourcereleasebindingrevisions", "rrbr"}, cmd.Aliases)

	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"list", "get", "delete"}, names)
}

func TestCmds_FactoryError(t *testing.T) {
	for name, run := range map[string]func() error{
		"list": func() error { c := newListCmd(errFactory("factory failed")); return c.RunE(c, nil) },
		"get":  func() error { c := newGetCmd(errFactory("factory failed")); return c.RunE(c, nil) },
	} {
		assert.EqualError(t, run(), "factory failed", name)
	}
}

func TestListCmd(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)

	cmd := newListCmd(mockFactory(mc))
	require.NoError(t, cmd.ParseFlags([]string{"-n", testNS, "--binding", testBinding}))
	out := testutil.CaptureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, nil)) })

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, []string{"db-r2", "Deploy", "Healthy", "true", "false", "2h"}, strings.Fields(lines[1]))
}

func TestList_BindingNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResourceReleaseBinding(mock.Anything, testNS, "missing").Return(nil, nil)

	err := New(mc).List(ListParams{Namespace: testNS, Binding: "missing"})
	assert.EqualError(t, err, `resource release binding "missing" not found in namespace "ns"`)
}

func TestGet(t *testing.T) {
	t.Run("yaml", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		expectHistory(mc)
		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, New(mc).Get(GetParams{Namespace: testNS, Binding: testBinding, Revision: 1, Output: "yaml"}))
		})
		assert.Contains(t, out, "resourceReleaseName: db-aaa")
	})

	t.Run("not found", func(t *testing.T) {
		mc := mocks.NewMockInterface(t)
		expectHistory(mc)
		err := New(mc).Get(GetParams{Namespace: testNS, Binding: testBinding, Revision: 7, Output: "table"})
		assert.EqualError(t, err, `revision 7 of resource release binding "db-dev" not found`)
	})
}

// Without --binding, list shows every resource binding's revisions, with each
// revision's binding and environment; --resource and --environment narrow it.
func TestList_AllBindings(t *testing.T) {
	binding := func(name, resource, environment string) gen.ResourceReleaseBinding {
		spec := &gen.ResourceReleaseBindingSpec{Environment: environment}
		spec.Owner.ResourceName = resource
		spec.Owner.ProjectName = "shop"
		return gen.ResourceReleaseBinding{Metadata: gen.ObjectMeta{Name: name}, Spec: spec}
	}
	rev := func(n int64, release string) gen.ResourceReleaseBindingRevision {
		created := time.Now().Add(-time.Hour)
		h := gen.BindingRevisionStatusHealth("Healthy")
		active, knownGood := true, true
		r := gen.ResourceReleaseBindingRevision{
			Metadata: gen.ObjectMeta{Name: fmt.Sprintf("x-r%d", n), CreationTimestamp: &created},
			Spec:     gen.ResourceReleaseBindingRevisionSpec{Revision: n, ResourceReleaseName: release, Cause: "Deploy", SnapshotHash: "h"},
			Status:   &gen.BindingRevisionStatus{Health: &h, Active: &active, KnownGood: &knownGood},
		}
		r.Spec.BindingSpec.Owner.ResourceName = "db"
		return r
	}

	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, testNS, mock.MatchedBy(func(p *gen.ListResourceReleaseBindingsParams) bool {
		return p.Resource != nil && *p.Resource == "db"
	})).Return(&gen.ResourceReleaseBindingList{Items: []gen.ResourceReleaseBinding{
		binding("db-prod", "db", "production"),
		binding("db-dev", "db", "dev"),
	}}, nil)
	for _, env := range []string{"dev", "production"} {
		mc.EXPECT().ListResourceReleaseBindingRevisions(mock.Anything, testNS, mock.MatchedBy(func(p *gen.ListResourceReleaseBindingRevisionsParams) bool {
			return p.Resource == "db" && p.Environment != nil && *p.Environment == env
		})).Return(&gen.ResourceReleaseBindingRevisionList{Items: []gen.ResourceReleaseBindingRevision{rev(1, "db-"+env)}}, nil)
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: testNS, Resource: "db"}))
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, []string{"PROJECT", "BINDING", "ENVIRONMENT", "NAME"}, strings.Fields(lines[0])[:4])
	assert.Equal(t, []string{"shop", "db-dev", "dev", "db-r1"}, strings.Fields(lines[1])[:4])
	assert.Equal(t, []string{"shop", "db-prod", "production", "db-r1"}, strings.Fields(lines[2])[:4])
}

func TestList_AllBindingsByProject(t *testing.T) {
	binding := func(name, project, resource string) gen.ResourceReleaseBinding {
		spec := &gen.ResourceReleaseBindingSpec{Environment: "dev"}
		spec.Owner.ResourceName = resource
		spec.Owner.ProjectName = project
		return gen.ResourceReleaseBinding{Metadata: gen.ObjectMeta{Name: name}, Spec: spec}
	}
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, testNS, mock.Anything).Return(&gen.ResourceReleaseBindingList{
		Items: []gen.ResourceReleaseBinding{binding("db-dev", "shop", "db"), binding("cache-dev", "finance", "cache")},
	}, nil)
	h := gen.BindingRevisionStatusHealth("Healthy")
	active := true
	dbRev := gen.ResourceReleaseBindingRevision{
		Metadata: gen.ObjectMeta{Name: "db-dev-r1"},
		Spec:     gen.ResourceReleaseBindingRevisionSpec{Revision: 1, ResourceReleaseName: "db-aaa", Cause: "Deploy", SnapshotHash: "h"},
		Status:   &gen.BindingRevisionStatus{Health: &h, Active: &active},
	}
	dbRev.Spec.BindingSpec.Owner.ResourceName = "db"
	mc.EXPECT().ListResourceReleaseBindingRevisions(mock.Anything, testNS, mock.MatchedBy(func(p *gen.ListResourceReleaseBindingRevisionsParams) bool {
		return p.Resource == "db"
	})).Return(&gen.ResourceReleaseBindingRevisionList{Items: []gen.ResourceReleaseBindingRevision{dbRev}}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: testNS, Project: "shop"}))
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, []string{"BINDING", "ENVIRONMENT", "NAME"}, strings.Fields(lines[0])[:3])
	assert.Equal(t, []string{"db-dev", "dev", "db-r1"}, strings.Fields(lines[1])[:3])
}

func TestListCmd_SkipsContextDefaults(t *testing.T) {
	cmd := newListCmd(errFactory("unused"))
	_, skip := cmd.Annotations[config.SkipContextDefaultsAnnotation]
	assert.True(t, skip)
}

func TestDelete_WithYes(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)
	params := func(dryRun bool) any {
		return mock.MatchedBy(func(p *gen.DeleteResourceReleaseBindingRevisionParams) bool {
			return p.Binding != nil && *p.Binding == testBinding && p.DryRun != nil && *p.DryRun == dryRun
		})
	}
	mc.EXPECT().DeleteResourceReleaseBindingRevision(mock.Anything, testNS, "db-dev-r1", params(true)).
		Return(&gen.DeleteBindingRevisionResponse{RevisionName: "db-dev-r1", Revision: 1, DryRun: true}, nil).Once()
	mc.EXPECT().DeleteResourceReleaseBindingRevision(mock.Anything, testNS, "db-dev-r1", params(false)).
		Return(&gen.DeleteBindingRevisionResponse{RevisionName: "db-dev-r1", Revision: 1}, nil).Once()

	r := New(mc)
	r.confirm = func(io.Writer, string) (bool, error) { t.Fatal("--yes must not ask"); return false, nil }
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Delete(DeleteParams{Namespace: testNS, Binding: testBinding, Revision: 1, Yes: true}))
	})
	assert.Contains(t, out, "Successfully deleted revision 1: db-r1")
}

func TestDelete_NotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)
	err := New(mc).Delete(DeleteParams{Namespace: testNS, Binding: testBinding, Revision: 5})
	require.ErrorContains(t, err, "not found; it may have been deleted")
}
