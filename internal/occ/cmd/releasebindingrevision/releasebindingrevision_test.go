// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebindingrevision

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	testNS        = "ns"
	testBinding   = "api-dev"
	testComponent = "api"
)

func testBindingObj() *gen.ReleaseBinding {
	spec := &gen.ReleaseBindingSpec{Environment: "dev"}
	spec.Owner.ComponentName = testComponent
	spec.Owner.ProjectName = "shop"
	return &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: testBinding}, Spec: spec}
}

func testRevision(n int64, release, cause, health string, active, knownGood bool) gen.ReleaseBindingRevision {
	created := time.Now().Add(-time.Duration(n) * time.Hour)
	h := gen.BindingRevisionStatusHealth(health)
	rev := gen.ReleaseBindingRevision{
		Metadata: gen.ObjectMeta{Name: fmt.Sprintf("%s-r%d", testBinding, n), CreationTimestamp: &created},
		Spec: gen.ReleaseBindingRevisionSpec{
			Revision: n, ComponentReleaseName: release, Cause: gen.RevisionCause(cause), SnapshotHash: "h",
		},
		Status: &gen.BindingRevisionStatus{Health: &h, Active: &active, KnownGood: &knownGood},
	}
	rev.Spec.BindingSpec.Owner.ComponentName = testComponent
	return rev
}

// expectHistory sets up a binding with three revisions, newest first as the API returns them.
func expectHistory(mc *mocks.MockInterface) {
	mc.EXPECT().GetReleaseBinding(mock.Anything, testNS, testBinding).Return(testBindingObj(), nil)
	mc.EXPECT().ListReleaseBindingRevisions(mock.Anything, testNS, mock.MatchedBy(func(p *gen.ListReleaseBindingRevisionsParams) bool {
		return p.Component == testComponent && p.Environment != nil && *p.Environment == "dev" &&
			p.Binding != nil && *p.Binding == testBinding
	})).Return(&gen.ReleaseBindingRevisionList{Items: []gen.ReleaseBindingRevision{
		testRevision(3, "api-ccc", "Override", "Progressing", true, false),
		testRevision(2, "api-bbb", "Deploy", "Degraded", false, false),
		testRevision(1, "api-aaa", "Deploy", "Healthy", false, true),
	}}, nil)
}

// --- List ---

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: testNS, Binding: testBinding}))
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, []string{"NAME", "CAUSE", "HEALTH", "ACTIVE", "KNOWN-GOOD", "AGE", "MESSAGE"}, strings.Fields(lines[0]))
	assert.Equal(t, []string{"api-r3", "Override", "Progressing", "true", "false", "3h"}, strings.Fields(lines[1]))
	assert.Equal(t, []string{"api-r1", "Deploy", "Healthy", "false", "true", "1h"}, strings.Fields(lines[3]))
}

func TestList_BindingNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBinding(mock.Anything, testNS, "missing").Return(nil, nil)

	err := New(mc).List(ListParams{Namespace: testNS, Binding: "missing"})
	assert.EqualError(t, err, `release binding "missing" not found in namespace "ns"`)
}

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBinding(mock.Anything, testNS, testBinding).Return(testBindingObj(), nil)
	mc.EXPECT().ListReleaseBindingRevisions(mock.Anything, testNS, mock.Anything).Return(nil, fmt.Errorf("server error"))

	assert.EqualError(t, New(mc).List(ListParams{Namespace: testNS, Binding: testBinding}), "server error")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetReleaseBinding(mock.Anything, testNS, testBinding).Return(testBindingObj(), nil)
	mc.EXPECT().ListReleaseBindingRevisions(mock.Anything, testNS, mock.Anything).
		Return(&gen.ReleaseBindingRevisionList{}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: testNS, Binding: testBinding}))
	})
	assert.Contains(t, out, "No revisions found")
}

func TestList_MissingNamespace(t *testing.T) {
	err := New(mocks.NewMockInterface(t)).List(ListParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace")
}

func bindingObj(name, component, environment string) gen.ReleaseBinding {
	return bindingInProject(name, "shop", component, environment)
}

func bindingInProject(name, project, component, environment string) gen.ReleaseBinding {
	spec := &gen.ReleaseBindingSpec{Environment: environment}
	spec.Owner.ComponentName = component
	spec.Owner.ProjectName = project
	return gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: name}, Spec: spec}
}

func expectRevisionsOf(mc *mocks.MockInterface, component, environment string, revs ...gen.ReleaseBindingRevision) {
	for i := range revs {
		revs[i].Spec.BindingSpec.Owner.ComponentName = component
	}
	mc.EXPECT().ListReleaseBindingRevisions(mock.Anything, testNS, mock.MatchedBy(func(p *gen.ListReleaseBindingRevisionsParams) bool {
		return p.Component == component && p.Environment != nil && *p.Environment == environment
	})).Return(&gen.ReleaseBindingRevisionList{Items: revs}, nil)
}

// Without --binding, list shows every binding's revisions in binding-name
// order, with each revision's binding and environment.
func TestList_AllBindings(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, testNS, mock.MatchedBy(func(p *gen.ListReleaseBindingsParams) bool {
		return p.Component == nil
	})).Return(&gen.ReleaseBindingList{Items: []gen.ReleaseBinding{
		bindingObj("web-prod", "web", "production"),
		bindingObj("api-dev", testComponent, "dev"),
	}}, nil)
	expectRevisionsOf(mc, testComponent, "dev",
		testRevision(2, "api-bbb", "Deploy", "Healthy", true, false),
		testRevision(1, "api-aaa", "Deploy", "Healthy", false, true))
	expectRevisionsOf(mc, "web", "production",
		testRevision(1, "web-aaa", "Deploy", "Healthy", true, true))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: testNS}))
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, []string{"PROJECT", "BINDING", "ENVIRONMENT", "NAME", "CAUSE", "HEALTH", "ACTIVE", "KNOWN-GOOD", "AGE", "MESSAGE"}, strings.Fields(lines[0]))
	assert.Equal(t, []string{"shop", "api-dev", "dev", "api-r2"}, strings.Fields(lines[1])[:4])
	assert.Equal(t, []string{"shop", "api-dev", "dev", "api-r1"}, strings.Fields(lines[2])[:4])
	assert.Equal(t, []string{"shop", "web-prod", "production", "web-r1"}, strings.Fields(lines[3])[:4])
}

func TestList_AllBindingsFiltered(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, testNS, mock.MatchedBy(func(p *gen.ListReleaseBindingsParams) bool {
		return p.Component != nil && *p.Component == testComponent
	})).Return(&gen.ReleaseBindingList{Items: []gen.ReleaseBinding{
		bindingObj("api-dev", testComponent, "dev"),
		bindingObj("api-prod", testComponent, "production"),
	}}, nil)
	// Only the production binding's revisions are fetched.
	expectRevisionsOf(mc, testComponent, "production",
		testRevision(1, "api-aaa", "Deploy", "Healthy", true, true))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: testNS, Component: testComponent, Environment: "production"}))
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, []string{"shop", "api-prod", "production", "api-r1"}, strings.Fields(lines[1])[:4])
}

// --project keeps only that project's bindings and drops the PROJECT column.
func TestList_AllBindingsByProject(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, testNS, mock.Anything).Return(&gen.ReleaseBindingList{Items: []gen.ReleaseBinding{
		bindingInProject("api-dev", "shop", testComponent, "dev"),
		bindingInProject("billing-dev", "finance", "billing", "dev"),
	}}, nil)
	expectRevisionsOf(mc, testComponent, "dev", testRevision(1, "api-aaa", "Deploy", "Healthy", true, true))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: testNS, Project: "shop"}))
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "BINDING", strings.Fields(lines[0])[0])
	assert.Equal(t, []string{"api-dev", "dev", "api-r1"}, strings.Fields(lines[1])[:3])
}

func TestList_AllBindingsNone(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, testNS, mock.Anything).Return(&gen.ReleaseBindingList{}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: testNS}))
	})
	assert.Contains(t, out, "No revisions found")
}

// --- Get ---

func TestGet_Table(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Get(GetParams{Namespace: testNS, Binding: testBinding, Revision: 2, Output: "table"}))
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, []string{"api-r2", "Deploy", "Degraded", "false", "false", "2h"}, strings.Fields(lines[1]))
}

func TestGet_YAML(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Get(GetParams{Namespace: testNS, Binding: testBinding, Revision: 1, Output: "yaml"}))
	})
	assert.Contains(t, out, "name: api-dev-r1")
	assert.Contains(t, out, "componentReleaseName: api-aaa")
	assert.Contains(t, out, "knownGood: true")
}

func TestGet_RevisionNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)

	err := New(mc).Get(GetParams{Namespace: testNS, Binding: testBinding, Revision: 9, Output: "table"})
	assert.EqualError(t, err, `revision 9 of release binding "api-dev" not found`)
}

func TestGet_InvalidArgs(t *testing.T) {
	tests := []struct {
		name   string
		params GetParams
		want   string
	}{
		{name: "no revision", params: GetParams{Namespace: testNS, Binding: testBinding, Output: "table"}, want: "--revision is required"},
		{name: "bad output", params: GetParams{Namespace: testNS, Binding: testBinding, Revision: 1, Output: "json"}, want: "unsupported output format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(mocks.NewMockInterface(t)).Get(tt.params)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

// --- Rollback ---

// --- Delete ---

func deleteParams(dryRun bool) any {
	return mock.MatchedBy(func(p *gen.DeleteReleaseBindingRevisionParams) bool {
		return p.Binding != nil && *p.Binding == testBinding && p.DryRun != nil && *p.DryRun == dryRun
	})
}

func TestDelete_ConfirmedDeletesTheRevision(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)
	display := "api-r2"
	mc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, testNS, "api-dev-r2", deleteParams(true)).
		Return(&gen.DeleteBindingRevisionResponse{RevisionName: "api-dev-r2", Revision: 2, DryRun: true}, nil).Once()
	mc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, testNS, "api-dev-r2", deleteParams(false)).
		Return(&gen.DeleteBindingRevisionResponse{RevisionName: "api-dev-r2", RevisionDisplayName: &display, Revision: 2}, nil).Once()

	r := New(mc)
	var question string
	r.confirm = func(_ io.Writer, q string) (bool, error) { question = q; return true, nil }
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Delete(DeleteParams{Namespace: testNS, Binding: testBinding, Revision: 2}))
	})
	assert.Contains(t, question, "Delete this revision?")
	assert.Contains(t, out, "Revision:    api-r2")
	assert.Contains(t, out, "Status:      Inactive")
	assert.Contains(t, out, "Successfully deleted revision 2: api-r2")
}

func TestDelete_RevisionNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)

	err := New(mc).Delete(DeleteParams{Namespace: testNS, Binding: testBinding, Revision: 9, Yes: true})
	require.EqualError(t, err, `revision 9 of release binding "api-dev" not found; it may have been deleted`)
}

func TestDelete_ActiveRevisionIsRefused(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)
	mc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, testNS, "api-dev-r3", deleteParams(true)).
		Return(nil, errors.New("Cannot delete revision 3 because it is currently active")).Once()

	err := New(mc).Delete(DeleteParams{Namespace: testNS, Binding: testBinding, Revision: 3, Yes: true})
	require.EqualError(t, err, "Cannot delete revision 3 because it is currently active")
}

func TestDelete_InvalidArgs(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	require.Error(t, New(mc).Delete(DeleteParams{Binding: testBinding, Revision: 2}), "namespace is required")
	require.Error(t, New(mc).Delete(DeleteParams{Namespace: testNS, Revision: 2}), "binding is required")
	require.ErrorContains(t, New(mc).Delete(DeleteParams{Namespace: testNS, Binding: testBinding}), "--revision is required")
}
