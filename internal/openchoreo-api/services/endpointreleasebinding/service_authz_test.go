// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding/mocks"
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
	return &endpointReleaseBindingServiceWithAuthz{
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

func newBindingFixture(name, ownerEndpointName string) *openchoreov1alpha1.EndpointReleaseBinding {
	return &openchoreov1alpha1.EndpointReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: authzNamespace},
		Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
			Owner: openchoreov1alpha1.EndpointReleaseBindingOwner{
				ProjectName:  authzProject,
				EndpointName: ownerEndpointName,
			},
			Environment: "dev",
		},
	}
}

func TestCreateEndpointReleaseBinding_AuthzCheck(t *testing.T) {
	rb := newBindingFixture("my-rb", "my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateEndpointReleaseBinding", mock.Anything, authzNamespace, rb).Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointreleasebinding:create", "endpointreleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Create authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateEndpointReleaseBinding_AuthzCheck(t *testing.T) {
	rb := newBindingFixture("my-rb", "my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		// Update fetches the existing binding first so the authz check uses the
		// on-disk owner/environment rather than whatever the client sent.
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		mockSvc.On("UpdateEndpointReleaseBinding", mock.Anything, authzNamespace, rb).Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointreleasebinding:update", "endpointreleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Update authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(nil, ErrEndpointReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, rb)
		require.ErrorIs(t, err, ErrEndpointReleaseBindingNotFound)
		require.Len(t, pdp.Captured, 0, "PDP should not be queried when binding is not found")
	})

	t.Run("authz uses existing owner — body cannot escalate scope", func(t *testing.T) {
		// Existing binding belongs to project A; client sends body claiming project B.
		// The authz check must use existing.Spec.Owner.ProjectName (project A), not
		// the body's claim, so the project-scoped policy is honored.
		existing := newBindingFixture("my-rb", "my-r")
		bodyClaiming := newBindingFixture("my-rb", "my-r")
		bodyClaiming.Spec.Owner.ProjectName = "different-project"

		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(existing, nil)
		mockSvc.On("UpdateEndpointReleaseBinding", mock.Anything, authzNamespace, bodyClaiming).Return(existing, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, bodyClaiming)
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		// Hardened: assert the captured request specifically uses the existing project,
		// not the body's claim. This guards against future refactors that might
		// accidentally read from the body again.
		require.Equal(t, authzProject, pdp.Captured[0].Resource.Hierarchy.Project, "authz must use existing.Spec.Owner.ProjectName, not the body's claim")
		require.NotEqual(t, "different-project", pdp.Captured[0].Resource.Hierarchy.Project, "body's project claim must not leak into authz hierarchy")
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointreleasebinding:update", "endpointreleasebinding", "my-rb", projectHierarchy())
	})
}

func TestGetEndpointReleaseBinding_AuthzCheck(t *testing.T) {
	rb := newBindingFixture("my-rb", "my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.GetEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, "my-rb")
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointreleasebinding:view", "endpointreleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Get authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "missing").Return(nil, ErrEndpointReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrEndpointReleaseBindingNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestDeleteEndpointReleaseBinding_AuthzCheck(t *testing.T) {
	rb := newBindingFixture("my-rb", "my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		mockSvc.On("DeleteEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, "my-rb")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointreleasebinding:delete", "endpointreleasebinding", "my-rb", projectHierarchy())
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "Delete authz should attach binding's environment to ABAC context")
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "my-rb").Return(rb, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointReleaseBinding", mock.Anything, authzNamespace, "missing").Return(nil, ErrEndpointReleaseBindingNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpointReleaseBinding(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrEndpointReleaseBindingNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestListEndpointReleaseBindings_AuthzCheck(t *testing.T) {
	// Use distinct owner.endpointName values so the per-item authz hierarchy
	// assertion proves Resource is sourced from each binding, not a constant.
	items := []openchoreov1alpha1.EndpointReleaseBinding{
		*newBindingFixture("rb-1", "owner-r1"),
		*newBindingFixture("rb-2", "owner-r2"),
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEndpointReleaseBindings", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.EndpointReleaseBinding]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListEndpointReleaseBindings(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointreleasebinding:view", "endpointreleasebinding", "rb-1", projectHierarchy())
		testutil.RequireEvalRequest(t, pdp.Captured[1], "endpointreleasebinding:view", "endpointreleasebinding", "rb-2", projectHierarchy())
		// List propagates per-item environment to the authz check.
		require.Equal(t, expectedEnvAttr, pdp.Captured[0].Context.Resource.Environment, "List authz should attach per-item environment to ABAC context")
		require.Equal(t, expectedEnvAttr, pdp.Captured[1].Context.Resource.Environment, "List authz should attach per-item environment to ABAC context")
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEndpointReleaseBindings", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.EndpointReleaseBinding]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListEndpointReleaseBindings(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
