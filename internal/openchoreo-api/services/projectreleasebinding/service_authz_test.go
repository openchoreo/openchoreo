// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectreleasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// expectedEnvAttr is the Context.Resource.Environment value the authz wrapper
// should pass for a binding whose spec.environment is "dev". Mirrors the
// FormatDualScopedResourceName encoding used by the wrapper.
var expectedEnvAttr = services.FormatDualScopedResourceName(testNamespace, "dev", false)

func newAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &projectReleaseBindingServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func projectHierarchy() authzcore.ResourceHierarchy {
	return authzcore.ResourceHierarchy{
		Namespace: testNamespace,
		Project:   testProjectName,
	}
}

func bindingFixture(name string) *openchoreov1alpha1.ProjectReleaseBinding {
	return &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: testProjectName},
			Environment: "dev",
		},
	}
}

func TestCreateProjectReleaseBinding_AuthzCheck(t *testing.T) {
	rb := bindingFixture("my-rb")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateProjectReleaseBinding", mock.Anything, testNamespace, rb).Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateProjectReleaseBinding(testutil.AuthzContext(), testNamespace, rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectreleasebinding:create", "projectreleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Create authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateProjectReleaseBinding(testutil.AuthzContext(), testNamespace, rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateProjectReleaseBinding_AuthzCheck(t *testing.T) {
	rb := bindingFixture("my-rb")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(rb, nil)
		mockSvc.On("UpdateProjectReleaseBinding", mock.Anything, testNamespace, rb).Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateProjectReleaseBinding(testutil.AuthzContext(), testNamespace, rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectreleasebinding:update", "projectreleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Update authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateProjectReleaseBinding(testutil.AuthzContext(), testNamespace, rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(nil, ErrProjectReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateProjectReleaseBinding(testutil.AuthzContext(), testNamespace, rb)
		require.ErrorIs(t, err, ErrProjectReleaseBindingNotFound)
		require.Len(t, pdp.Captured, 0, "PDP should not be queried when binding is not found")
	})

	t.Run("authz uses existing owner — body cannot escalate scope", func(t *testing.T) {
		existing := bindingFixture("my-rb")
		bodyClaiming := bindingFixture("my-rb")
		bodyClaiming.Spec.Owner.ProjectName = "different-project"

		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(existing, nil)
		mockSvc.On("UpdateProjectReleaseBinding", mock.Anything, testNamespace, bodyClaiming).Return(existing, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateProjectReleaseBinding(testutil.AuthzContext(), testNamespace, bodyClaiming)
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		require.Equal(t, testProjectName, pdp.Captured[0].Resource.Hierarchy.Project, "authz must use existing.Spec.Owner.ProjectName, not the body's claim")
		require.NotEqual(t, "different-project", pdp.Captured[0].Resource.Hierarchy.Project, "body's project claim must not leak into authz hierarchy")
	})
}

func TestGetProjectReleaseBinding_AuthzCheck(t *testing.T) {
	rb := bindingFixture("my-rb")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.GetProjectReleaseBinding(testutil.AuthzContext(), testNamespace, "my-rb")
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectreleasebinding:view", "projectreleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetProjectReleaseBinding(testutil.AuthzContext(), testNamespace, "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteProjectReleaseBinding_AuthzCheck(t *testing.T) {
	rb := bindingFixture("my-rb")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(rb, nil)
		mockSvc.On("DeleteProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProjectReleaseBinding(testutil.AuthzContext(), testNamespace, "my-rb")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectreleasebinding:delete", "projectreleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProjectReleaseBinding(testutil.AuthzContext(), testNamespace, "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListProjectReleaseBindings_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ProjectReleaseBinding{
		*bindingFixture("rb-1"),
		*bindingFixture("rb-2"),
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListProjectReleaseBindings", mock.Anything, testNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ProjectReleaseBinding]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListProjectReleaseBindings(testutil.AuthzContext(), testNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projectreleasebinding:view", "projectreleasebinding", "rb-1", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment)
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListProjectReleaseBindings", mock.Anything, testNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ProjectReleaseBinding]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListProjectReleaseBindings(testutil.AuthzContext(), testNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}

func TestProjectReleaseBindingAuthz_FetchFirstPropagatesError(t *testing.T) {
	t.Run("get propagates internal error before authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(nil, ErrProjectReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetProjectReleaseBinding(testutil.AuthzContext(), testNamespace, "my-rb")
		require.ErrorIs(t, err, ErrProjectReleaseBindingNotFound)
		require.Empty(t, pdp.Captured, "authz must not be queried when the fetch fails")
	})

	t.Run("delete propagates internal error before authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectReleaseBinding", mock.Anything, testNamespace, "my-rb").Return(nil, ErrProjectReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProjectReleaseBinding(testutil.AuthzContext(), testNamespace, "my-rb")
		require.ErrorIs(t, err, ErrProjectReleaseBindingNotFound)
		require.Empty(t, pdp.Captured)
	})
}
