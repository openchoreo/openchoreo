// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectrelease/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newPRAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &projectReleaseServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func prFixture(name string) *openchoreov1alpha1.ProjectRelease {
	return &openchoreov1alpha1.ProjectRelease{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.ProjectReleaseSpec{
			Owner: openchoreov1alpha1.ProjectReleaseOwner{ProjectName: testProjectName},
		},
	}
}

func expectedHierarchy() authzcore.ResourceHierarchy {
	return authzcore.ResourceHierarchy{Namespace: testNamespace, Project: testProjectName}
}

func TestCreateProjectRelease_AuthzCheck(t *testing.T) {
	resource := prFixture("my-pr")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateProjectRelease", mock.Anything, testNamespace, resource).Return(resource, nil)
		svc := newPRAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateProjectRelease(testutil.AuthzContext(), testNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectrelease:create", "projectrelease", "my-pr", expectedHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newPRAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateProjectRelease(testutil.AuthzContext(), testNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetProjectRelease_AuthzCheck(t *testing.T) {
	resource := prFixture("my-pr")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectRelease", mock.Anything, testNamespace, "my-pr").Return(resource, nil)
		svc := newPRAuthzSvc(pdp, mockSvc)
		result, err := svc.GetProjectRelease(testutil.AuthzContext(), testNamespace, "my-pr")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectrelease:view", "projectrelease", "my-pr", expectedHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectRelease", mock.Anything, testNamespace, "my-pr").Return(resource, nil)
		svc := newPRAuthzSvc(pdp, mockSvc)
		_, err := svc.GetProjectRelease(testutil.AuthzContext(), testNamespace, "my-pr")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteProjectRelease_AuthzCheck(t *testing.T) {
	resource := prFixture("my-pr")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectRelease", mock.Anything, testNamespace, "my-pr").Return(resource, nil)
		mockSvc.On("DeleteProjectRelease", mock.Anything, testNamespace, "my-pr").Return(nil)
		svc := newPRAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProjectRelease(testutil.AuthzContext(), testNamespace, "my-pr")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectrelease:delete", "projectrelease", "my-pr", expectedHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectRelease", mock.Anything, testNamespace, "my-pr").Return(resource, nil)
		svc := newPRAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProjectRelease(testutil.AuthzContext(), testNamespace, "my-pr")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListProjectReleases_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ProjectRelease{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-1"},
			Spec:       openchoreov1alpha1.ProjectReleaseSpec{Owner: openchoreov1alpha1.ProjectReleaseOwner{ProjectName: testProjectName}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-2"},
			Spec:       openchoreov1alpha1.ProjectReleaseSpec{Owner: openchoreov1alpha1.ProjectReleaseOwner{ProjectName: testProjectName}},
		},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListProjectReleases", mock.Anything, testNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ProjectRelease]{Items: items}, nil)
		svc := newPRAuthzSvc(pdp, mockSvc)
		result, err := svc.ListProjectReleases(testutil.AuthzContext(), testNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectrelease:view", "projectrelease", "pr-1", expectedHierarchy())
		testutil.RequireEvalRequest(t, pdp.Captured[1], "projectrelease:view", "projectrelease", "pr-2", expectedHierarchy())
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListProjectReleases", mock.Anything, testNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ProjectRelease]{Items: items}, nil)
		svc := newPRAuthzSvc(pdp, mockSvc)
		result, err := svc.ListProjectReleases(testutil.AuthzContext(), testNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
