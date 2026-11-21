// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func testRevision(name, environment string) *openchoreov1alpha1.ReleaseBindingRevision {
	return &openchoreov1alpha1.ReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns-1"},
		Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{
			Revision: 1,
			BindingSpec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
				Environment: environment,
			},
		},
	}
}

// --- RollbackReleaseBinding ---

func TestRollbackReleaseBinding_AuthzCheck(t *testing.T) {
	rb := testRB()
	req := bindingrevision.RollbackRequest{Revision: 1}

	t.Run("allowed: checks releasebinding:rollback on the binding", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		want := &bindingrevision.RollbackResult{RevisionName: "my-rb-r4", Revision: 4, TargetRevision: 1}
		mockSvc.On("RollbackReleaseBinding", mock.Anything, "ns-1", "my-rb", req).Return(want, nil)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		got, err := svc.RollbackReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb", req)
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:rollback", "releasebinding", "my-rb", rbHierarchy)
		require.Equal(t, "ns-1/dev", pdp.Captured[0].Context.Resource.Environment,
			"the environment attribute lets a role allow rollback in some environments only")
	})

	t.Run("denied: nothing is rolled back", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		_, err := svc.RollbackReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb", req)
		require.ErrorIs(t, err, services.ErrForbidden)
		mockSvc.AssertNotCalled(t, "RollbackReleaseBinding", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("binding not found: no authz call", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil, ErrReleaseBindingNotFound)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		_, err := svc.RollbackReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb", req)
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
		require.Empty(t, pdp.Captured)
	})
}

// --- GetReleaseBindingRevision ---

func TestGetReleaseBindingRevision_AuthzCheck(t *testing.T) {
	rev := testRevision("my-rb-r1", "dev")

	t.Run("allowed: checks releasebinding:view scoped like the binding", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBindingRevision", mock.Anything, "ns-1", "my-rb-r1").Return(rev, nil)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		got, err := svc.GetReleaseBindingRevision(testutil.AuthzContext(), "ns-1", "my-rb-r1")
		require.NoError(t, err)
		require.Equal(t, rev, got)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:view", "releasebinding", "my-rb-r1", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBindingRevision", mock.Anything, "ns-1", "my-rb-r1").Return(rev, nil)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		_, err := svc.GetReleaseBindingRevision(testutil.AuthzContext(), "ns-1", "my-rb-r1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- ListReleaseBindingRevisions ---

func TestListReleaseBindingRevisions_AuthzCheck(t *testing.T) {
	revs := []openchoreov1alpha1.ReleaseBindingRevision{*testRevision("my-rb-dev-r1", "dev"), *testRevision("my-rb-prod-r1", "prod")}

	t.Run("each revision is checked against its own environment", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListReleaseBindingRevisions", mock.Anything, "ns-1", "my-comp", "", "", mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ReleaseBindingRevision]{Items: revs}, nil)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		result, err := svc.ListReleaseBindingRevisions(testutil.AuthzContext(), "ns-1", "my-comp", "", "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
	})

	t.Run("denied revisions are filtered out", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListReleaseBindingRevisions", mock.Anything, "ns-1", "my-comp", "", "", mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ReleaseBindingRevision]{Items: revs}, nil)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		result, err := svc.ListReleaseBindingRevisions(testutil.AuthzContext(), "ns-1", "my-comp", "", "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}

// --- DeleteReleaseBindingRevision ---

func TestDeleteReleaseBindingRevision_AuthzCheck(t *testing.T) {
	rev := testRevision("my-rb-r2", "dev")
	req := bindingrevision.DeleteRequest{Binding: "my-rb"}

	t.Run("allowed: checks releasebinding:delete-revision on the binding", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBindingRevision", mock.Anything, "ns-1", "my-rb-r2").Return(rev, nil)
		want := &bindingrevision.DeleteResult{RevisionName: "my-rb-r2", Revision: 2}
		mockSvc.On("DeleteReleaseBindingRevision", mock.Anything, "ns-1", "my-rb-r2", req).Return(want, nil)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		got, err := svc.DeleteReleaseBindingRevision(testutil.AuthzContext(), "ns-1", "my-rb-r2", req)
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:delete-revision", "releasebinding", "my-rb-r2", rbHierarchy)
		require.Equal(t, "ns-1/dev", pdp.Captured[0].Context.Resource.Environment)
	})

	t.Run("denied: nothing is deleted", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBindingRevision", mock.Anything, "ns-1", "my-rb-r2").Return(rev, nil)
		svc := &releaseBindingServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

		_, err := svc.DeleteReleaseBindingRevision(testutil.AuthzContext(), "ns-1", "my-rb-r2", req)
		require.ErrorIs(t, err, services.ErrForbidden)
		mockSvc.AssertNotCalled(t, "DeleteReleaseBindingRevision", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}
