// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// expectedEnvAttr is the Context.Resource.Environment value the authz wrapper
// should pass for a binding whose spec.environment is "dev". It mirrors the
// FormatDualScopedResourceName encoding used by the wrapper so tests catch
// regressions if the encoding (or the call) changes.
var expectedEnvAttr = services.FormatDualScopedResourceName(authzNamespace, "dev", false)

const (
	authzNamespace = "ns-a"
	authzProject   = "proj-1"
)

func newAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &resourceReleaseBindingServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func projectHierarchy() authzcore.ResourceHierarchy {
	return authzcore.ResourceHierarchy{
		Namespace: authzNamespace,
		Project:   authzProject,
	}
}

func newBindingFixture(name string) *openchoreov1alpha1.ResourceReleaseBinding {
	return &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: authzNamespace},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
				ProjectName:  authzProject,
				ResourceName: "my-r",
			},
			Environment: "dev",
		},
	}
}

func TestCreateResourceReleaseBinding_AuthzCheck(t *testing.T) {
	rb := newBindingFixture("my-rb")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateResourceReleaseBinding", mock.Anything, authzNamespace, rb).Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcereleasebinding:create", "resourcereleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Create authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateResourceReleaseBinding_AuthzCheck(t *testing.T) {
	rb := newBindingFixture("my-rb")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		// Update fetches the existing binding first so the authz check uses the
		// on-disk owner/environment rather than whatever the client sent.
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		mockSvc.On("UpdateResourceReleaseBinding", mock.Anything, authzNamespace, rb).Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcereleasebinding:update", "resourcereleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Update authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(nil, ErrResourceReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.ErrorIs(t, err, ErrResourceReleaseBindingNotFound)
		require.Len(t, pdp.Captured, 0, "PDP should not be queried when binding is not found")
	})

	t.Run("authz uses existing owner — body cannot escalate scope", func(t *testing.T) {
		// Existing binding belongs to project A; client sends body claiming project B.
		// The authz check must use existing.Spec.Owner.ProjectName (project A), not
		// the body's claim, so the project-scoped policy is honored.
		existing := newBindingFixture("my-rb")
		bodyClaiming := newBindingFixture("my-rb")
		bodyClaiming.Spec.Owner.ProjectName = "different-project"

		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(existing, nil)
		mockSvc.On("UpdateResourceReleaseBinding", mock.Anything, authzNamespace, bodyClaiming).Return(existing, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, bodyClaiming)
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		// Hardened: assert the captured request specifically uses the existing project,
		// not the body's claim. This guards against future refactors that might
		// accidentally read from the body again.
		require.Equal(t, authzProject, pdp.Captured[0].Resource.Hierarchy.Project, "authz must use existing.Spec.Owner.ProjectName, not the body's claim")
		require.NotEqual(t, "different-project", pdp.Captured[0].Resource.Hierarchy.Project, "body's project claim must not leak into authz hierarchy")
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcereleasebinding:update", "resourcereleasebinding", "my-rb", projectHierarchy())
	})
}

func TestGetResourceReleaseBinding_AuthzCheck(t *testing.T) {
	rb := newBindingFixture("my-rb")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.GetResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, "my-rb")
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcereleasebinding:view", "resourcereleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Get authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "missing").Return(nil, ErrResourceReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrResourceReleaseBindingNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestDeleteResourceReleaseBinding_AuthzCheck(t *testing.T) {
	rb := newBindingFixture("my-rb")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		mockSvc.On("DeleteResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, "my-rb")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcereleasebinding:delete", "resourcereleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Delete authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceReleaseBinding", mock.Anything, authzNamespace, "missing").Return(nil, ErrResourceReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResourceReleaseBinding(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrResourceReleaseBindingNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestListResourceReleaseBindings_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ResourceReleaseBinding{
		*newBindingFixture("rb-1"),
		*newBindingFixture("rb-2"),
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListResourceReleaseBindings", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ResourceReleaseBinding]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListResourceReleaseBindings(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcereleasebinding:view", "resourcereleasebinding", "rb-1", projectHierarchy())
		testutil.RequireEvalRequest(t, pdp.Captured[1], "resourcereleasebinding:view", "resourcereleasebinding", "rb-2", projectHierarchy())
		// List propagates per-item environment to the authz check.
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "List authz should attach per-item environment to ABAC context")
		require.Equal(t, expectedEnvAttr, pdp.Captured[1].Context.Resource.Environment, "List authz should attach per-item environment to ABAC context")
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListResourceReleaseBindings", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ResourceReleaseBinding]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListResourceReleaseBindings(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
