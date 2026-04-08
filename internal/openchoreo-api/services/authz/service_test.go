// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	authzcoremocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace   = "test-ns"
	testRoleName    = "test-role"
	testBindingName = "test-binding"
)

// newService constructs an authzService backed by mockery-generated PAP and PDP mocks.
func newService(t *testing.T) (Service, *authzcoremocks.MockPAP, *authzcoremocks.MockPDP) {
	t.Helper()
	pap := authzcoremocks.NewMockPAP(t)
	pdp := authzcoremocks.NewMockPDP(t)
	svc := NewService(pap, pdp, testutil.TestLogger())
	return svc, pap, pdp
}

// newInvalidErr returns a k8s Invalid StatusError that triggers the
// apierrors.IsInvalid → ValidationError branch.
func newInvalidErr() error {
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "openchoreo.dev", Kind: "AuthzRole"},
		"bad request",
		field.ErrorList{field.Invalid(field.NewPath("spec"), "x", "must be set")},
	)
}

// --- Cluster Roles ---

func TestCreateClusterRole(t *testing.T) {
	ctx := context.Background()
	role := &openchoreov1alpha1.ClusterAuthzRole{ObjectMeta: metav1.ObjectMeta{Name: testRoleName}}

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("CreateClusterRole", mock.Anything, role).Return(role.DeepCopy(), nil)

		result, err := svc.CreateClusterRole(ctx, role)
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.CreateClusterRole(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid error wrapped as ValidationError", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("CreateClusterRole", mock.Anything, role).Return(nil, newInvalidErr())

		_, err := svc.CreateClusterRole(ctx, role)
		var ve *services.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("non-invalid error returned as-is", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("CreateClusterRole", mock.Anything, role).Return(nil, errFake)

		_, err := svc.CreateClusterRole(ctx, role)
		require.ErrorIs(t, err, errFake)
	})
}

func TestGetClusterRole(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		stored := &openchoreov1alpha1.ClusterAuthzRole{ObjectMeta: metav1.ObjectMeta{Name: testRoleName}}
		pap.On("GetClusterRole", mock.Anything, testRoleName).Return(stored, nil)

		result, err := svc.GetClusterRole(ctx, testRoleName)
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleTypeMeta, result.TypeMeta)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("GetClusterRole", mock.Anything, testRoleName).Return(nil, errFake)

		_, err := svc.GetClusterRole(ctx, testRoleName)
		require.ErrorIs(t, err, errFake)
	})
}

func TestUpdateClusterRole(t *testing.T) {
	ctx := context.Background()
	role := &openchoreov1alpha1.ClusterAuthzRole{ObjectMeta: metav1.ObjectMeta{Name: testRoleName}}

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("UpdateClusterRole", mock.Anything, role).Return(role.DeepCopy(), nil)

		result, err := svc.UpdateClusterRole(ctx, role)
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.UpdateClusterRole(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid error wrapped as ValidationError", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("UpdateClusterRole", mock.Anything, role).Return(nil, newInvalidErr())

		_, err := svc.UpdateClusterRole(ctx, role)
		var ve *services.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("non-invalid error returned as-is", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("UpdateClusterRole", mock.Anything, role).Return(nil, errFake)

		_, err := svc.UpdateClusterRole(ctx, role)
		require.ErrorIs(t, err, errFake)
	})
}

func TestDeleteClusterRole(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("DeleteClusterRole", mock.Anything, testRoleName).Return(nil)

		require.NoError(t, svc.DeleteClusterRole(ctx, testRoleName))
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("DeleteClusterRole", mock.Anything, testRoleName).Return(errFake)

		require.ErrorIs(t, svc.DeleteClusterRole(ctx, testRoleName), errFake)
	})
}

func TestListClusterRoles(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		items := []openchoreov1alpha1.ClusterAuthzRole{
			{ObjectMeta: metav1.ObjectMeta{Name: "r1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "r2"}},
		}
		pap.On("ListClusterRoles", mock.Anything, 0, "").
			Return(&authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRole]{Items: items, NextCursor: "next-cr"}, nil)

		result, err := svc.ListClusterRoles(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterAuthzRoleTypeMeta, item.TypeMeta)
		}
		assert.Equal(t, "next-cr", result.NextCursor)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("ListClusterRoles", mock.Anything, 0, "").Return(nil, errFake)

		_, err := svc.ListClusterRoles(ctx, services.ListOptions{})
		require.ErrorIs(t, err, errFake)
	})
}

// --- Namespace Roles ---

func TestCreateNamespaceRole(t *testing.T) {
	ctx := context.Background()
	role := &openchoreov1alpha1.AuthzRole{ObjectMeta: metav1.ObjectMeta{Name: testRoleName}}

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("CreateNamespacedRole", mock.Anything, mock.MatchedBy(func(r *openchoreov1alpha1.AuthzRole) bool {
			return r.Namespace == testNamespace && r.Name == testRoleName
		})).Return(&openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName, Namespace: testNamespace},
		}, nil)

		result, err := svc.CreateNamespaceRole(ctx, testNamespace, role)
		require.NoError(t, err)
		assert.Equal(t, authzRoleTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.CreateNamespaceRole(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid error wrapped as ValidationError", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("CreateNamespacedRole", mock.Anything, mock.Anything).Return(nil, newInvalidErr())

		_, err := svc.CreateNamespaceRole(ctx, testNamespace, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName},
		})
		var ve *services.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("non-invalid error returned as-is", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("CreateNamespacedRole", mock.Anything, mock.Anything).Return(nil, errFake)

		_, err := svc.CreateNamespaceRole(ctx, testNamespace, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName},
		})
		require.ErrorIs(t, err, errFake)
	})
}

func TestGetNamespaceRole(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		stored := &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName, Namespace: testNamespace},
		}
		pap.On("GetNamespacedRole", mock.Anything, testRoleName, testNamespace).Return(stored, nil)

		result, err := svc.GetNamespaceRole(ctx, testNamespace, testRoleName)
		require.NoError(t, err)
		assert.Equal(t, authzRoleTypeMeta, result.TypeMeta)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("GetNamespacedRole", mock.Anything, testRoleName, testNamespace).Return(nil, errFake)

		_, err := svc.GetNamespaceRole(ctx, testNamespace, testRoleName)
		require.ErrorIs(t, err, errFake)
	})
}

func TestUpdateNamespaceRole(t *testing.T) {
	ctx := context.Background()
	role := &openchoreov1alpha1.AuthzRole{ObjectMeta: metav1.ObjectMeta{Name: testRoleName}}

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("UpdateNamespacedRole", mock.Anything, mock.MatchedBy(func(r *openchoreov1alpha1.AuthzRole) bool {
			return r.Namespace == testNamespace && r.Name == testRoleName
		})).Return(&openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName, Namespace: testNamespace},
		}, nil)

		result, err := svc.UpdateNamespaceRole(ctx, testNamespace, role)
		require.NoError(t, err)
		assert.Equal(t, authzRoleTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.UpdateNamespaceRole(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid error wrapped as ValidationError", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("UpdateNamespacedRole", mock.Anything, mock.Anything).Return(nil, newInvalidErr())

		_, err := svc.UpdateNamespaceRole(ctx, testNamespace, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName},
		})
		var ve *services.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("non-invalid error returned as-is", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("UpdateNamespacedRole", mock.Anything, mock.Anything).Return(nil, errFake)

		_, err := svc.UpdateNamespaceRole(ctx, testNamespace, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName},
		})
		require.ErrorIs(t, err, errFake)
	})
}

func TestDeleteNamespaceRole(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("DeleteNamespacedRole", mock.Anything, testRoleName, testNamespace).Return(nil)

		require.NoError(t, svc.DeleteNamespaceRole(ctx, testNamespace, testRoleName))
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("DeleteNamespacedRole", mock.Anything, testRoleName, testNamespace).Return(errFake)

		require.ErrorIs(t, svc.DeleteNamespaceRole(ctx, testNamespace, testRoleName), errFake)
	})
}

func TestListNamespaceRoles(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		items := []openchoreov1alpha1.AuthzRole{
			{ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: testNamespace}},
		}
		pap.On("ListNamespacedRoles", mock.Anything, testNamespace, 0, "").
			Return(&authzcore.PaginatedList[openchoreov1alpha1.AuthzRole]{Items: items, NextCursor: "next-nr"}, nil)

		result, err := svc.ListNamespaceRoles(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, authzRoleTypeMeta, result.Items[0].TypeMeta)
		assert.Equal(t, "next-nr", result.NextCursor)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("ListNamespacedRoles", mock.Anything, testNamespace, 0, "").Return(nil, errFake)

		_, err := svc.ListNamespaceRoles(ctx, testNamespace, services.ListOptions{})
		require.ErrorIs(t, err, errFake)
	})
}

// --- Cluster Role Bindings ---

func TestCreateClusterRoleBinding(t *testing.T) {
	ctx := context.Background()
	binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: testBindingName}}

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("CreateClusterRoleBinding", mock.Anything, binding).Return(binding.DeepCopy(), nil)

		result, err := svc.CreateClusterRoleBinding(ctx, binding)
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleBindingTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.CreateClusterRoleBinding(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid error wrapped as ValidationError", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("CreateClusterRoleBinding", mock.Anything, binding).Return(nil, newInvalidErr())

		_, err := svc.CreateClusterRoleBinding(ctx, binding)
		var ve *services.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("non-invalid error returned as-is", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("CreateClusterRoleBinding", mock.Anything, binding).Return(nil, errFake)

		_, err := svc.CreateClusterRoleBinding(ctx, binding)
		require.ErrorIs(t, err, errFake)
	})
}

func TestGetClusterRoleBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		stored := &openchoreov1alpha1.ClusterAuthzRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: testBindingName}}
		pap.On("GetClusterRoleBinding", mock.Anything, testBindingName).Return(stored, nil)

		result, err := svc.GetClusterRoleBinding(ctx, testBindingName)
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleBindingTypeMeta, result.TypeMeta)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("GetClusterRoleBinding", mock.Anything, testBindingName).Return(nil, errFake)

		_, err := svc.GetClusterRoleBinding(ctx, testBindingName)
		require.ErrorIs(t, err, errFake)
	})
}

func TestUpdateClusterRoleBinding(t *testing.T) {
	ctx := context.Background()
	binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: testBindingName}}

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("UpdateClusterRoleBinding", mock.Anything, binding).Return(binding.DeepCopy(), nil)

		result, err := svc.UpdateClusterRoleBinding(ctx, binding)
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleBindingTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.UpdateClusterRoleBinding(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid error wrapped as ValidationError", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("UpdateClusterRoleBinding", mock.Anything, binding).Return(nil, newInvalidErr())

		_, err := svc.UpdateClusterRoleBinding(ctx, binding)
		var ve *services.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("non-invalid error returned as-is", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("UpdateClusterRoleBinding", mock.Anything, binding).Return(nil, errFake)

		_, err := svc.UpdateClusterRoleBinding(ctx, binding)
		require.ErrorIs(t, err, errFake)
	})
}

func TestDeleteClusterRoleBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("DeleteClusterRoleBinding", mock.Anything, testBindingName).Return(nil)

		require.NoError(t, svc.DeleteClusterRoleBinding(ctx, testBindingName))
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("DeleteClusterRoleBinding", mock.Anything, testBindingName).Return(errFake)

		require.ErrorIs(t, svc.DeleteClusterRoleBinding(ctx, testBindingName), errFake)
	})
}

func TestListClusterRoleBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		items := []openchoreov1alpha1.ClusterAuthzRoleBinding{
			{ObjectMeta: metav1.ObjectMeta{Name: "b1"}},
		}
		pap.On("ListClusterRoleBindings", mock.Anything, 0, "").
			Return(&authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRoleBinding]{Items: items, NextCursor: "next-crb"}, nil)

		result, err := svc.ListClusterRoleBindings(ctx, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, clusterAuthzRoleBindingTypeMeta, result.Items[0].TypeMeta)
		assert.Equal(t, "next-crb", result.NextCursor)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("ListClusterRoleBindings", mock.Anything, 0, "").Return(nil, errFake)

		_, err := svc.ListClusterRoleBindings(ctx, services.ListOptions{})
		require.ErrorIs(t, err, errFake)
	})
}

// --- Namespace Role Bindings ---

func TestCreateNamespaceRoleBinding(t *testing.T) {
	ctx := context.Background()
	binding := &openchoreov1alpha1.AuthzRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: testBindingName}}

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("CreateNamespacedRoleBinding", mock.Anything, mock.MatchedBy(func(b *openchoreov1alpha1.AuthzRoleBinding) bool {
			return b.Namespace == testNamespace && b.Name == testBindingName
		})).Return(&openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName, Namespace: testNamespace},
		}, nil)

		result, err := svc.CreateNamespaceRoleBinding(ctx, testNamespace, binding)
		require.NoError(t, err)
		assert.Equal(t, authzRoleBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.CreateNamespaceRoleBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid error wrapped as ValidationError", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("CreateNamespacedRoleBinding", mock.Anything, mock.Anything).Return(nil, newInvalidErr())

		_, err := svc.CreateNamespaceRoleBinding(ctx, testNamespace, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName},
		})
		var ve *services.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("non-invalid error returned as-is", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("CreateNamespacedRoleBinding", mock.Anything, mock.Anything).Return(nil, errFake)

		_, err := svc.CreateNamespaceRoleBinding(ctx, testNamespace, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName},
		})
		require.ErrorIs(t, err, errFake)
	})
}

func TestGetNamespaceRoleBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		stored := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName, Namespace: testNamespace},
		}
		pap.On("GetNamespacedRoleBinding", mock.Anything, testBindingName, testNamespace).Return(stored, nil)

		result, err := svc.GetNamespaceRoleBinding(ctx, testNamespace, testBindingName)
		require.NoError(t, err)
		assert.Equal(t, authzRoleBindingTypeMeta, result.TypeMeta)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("GetNamespacedRoleBinding", mock.Anything, testBindingName, testNamespace).Return(nil, errFake)

		_, err := svc.GetNamespaceRoleBinding(ctx, testNamespace, testBindingName)
		require.ErrorIs(t, err, errFake)
	})
}

func TestUpdateNamespaceRoleBinding(t *testing.T) {
	ctx := context.Background()
	binding := &openchoreov1alpha1.AuthzRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: testBindingName}}

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("UpdateNamespacedRoleBinding", mock.Anything, mock.MatchedBy(func(b *openchoreov1alpha1.AuthzRoleBinding) bool {
			return b.Namespace == testNamespace && b.Name == testBindingName
		})).Return(&openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName, Namespace: testNamespace},
		}, nil)

		result, err := svc.UpdateNamespaceRoleBinding(ctx, testNamespace, binding)
		require.NoError(t, err)
		assert.Equal(t, authzRoleBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.UpdateNamespaceRoleBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("invalid error wrapped as ValidationError", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("UpdateNamespacedRoleBinding", mock.Anything, mock.Anything).Return(nil, newInvalidErr())

		_, err := svc.UpdateNamespaceRoleBinding(ctx, testNamespace, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName},
		})
		var ve *services.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("non-invalid error returned as-is", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("UpdateNamespacedRoleBinding", mock.Anything, mock.Anything).Return(nil, errFake)

		_, err := svc.UpdateNamespaceRoleBinding(ctx, testNamespace, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName},
		})
		require.ErrorIs(t, err, errFake)
	})
}

func TestDeleteNamespaceRoleBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		pap.On("DeleteNamespacedRoleBinding", mock.Anything, testBindingName, testNamespace).Return(nil)

		require.NoError(t, svc.DeleteNamespaceRoleBinding(ctx, testNamespace, testBindingName))
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("DeleteNamespacedRoleBinding", mock.Anything, testBindingName, testNamespace).Return(errFake)

		require.ErrorIs(t, svc.DeleteNamespaceRoleBinding(ctx, testNamespace, testBindingName), errFake)
	})
}

func TestListNamespaceRoleBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, pap, _ := newService(t)
		items := []openchoreov1alpha1.AuthzRoleBinding{
			{ObjectMeta: metav1.ObjectMeta{Name: "b1", Namespace: testNamespace}},
		}
		pap.On("ListNamespacedRoleBindings", mock.Anything, testNamespace, 0, "").
			Return(&authzcore.PaginatedList[openchoreov1alpha1.AuthzRoleBinding]{Items: items, NextCursor: "next-nrb"}, nil)

		result, err := svc.ListNamespaceRoleBindings(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, authzRoleBindingTypeMeta, result.Items[0].TypeMeta)
		assert.Equal(t, "next-nrb", result.NextCursor)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("ListNamespacedRoleBindings", mock.Anything, testNamespace, 0, "").Return(nil, errFake)

		_, err := svc.ListNamespaceRoleBindings(ctx, testNamespace, services.ListOptions{})
		require.ErrorIs(t, err, errFake)
	})
}

// --- Evaluate / ListActions / GetSubjectProfile ---

func TestEvaluate(t *testing.T) {
	ctx := context.Background()
	requests := []authzcore.EvaluateRequest{{Action: "project:view"}}

	t.Run("returns decisions from PDP", func(t *testing.T) {
		svc, _, pdp := newService(t)
		decisions := []authzcore.Decision{{Decision: true}}
		pdp.On("BatchEvaluate", mock.Anything, &authzcore.BatchEvaluateRequest{Requests: requests}).
			Return(&authzcore.BatchEvaluateResponse{Decisions: decisions}, nil)

		result, err := svc.Evaluate(ctx, requests)
		require.NoError(t, err)
		assert.Equal(t, decisions, result)
	})

	t.Run("wraps PDP error", func(t *testing.T) {
		svc, _, pdp := newService(t)
		errFake := fmt.Errorf("pdp unavailable")
		pdp.On("BatchEvaluate", mock.Anything, &authzcore.BatchEvaluateRequest{Requests: requests}).
			Return(nil, errFake)

		_, err := svc.Evaluate(ctx, requests)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to evaluate")
		assert.ErrorIs(t, err, errFake)
	})
}

func TestListActions(t *testing.T) {
	ctx := context.Background()

	t.Run("delegates to PAP", func(t *testing.T) {
		svc, pap, _ := newService(t)
		actions := []authzcore.Action{{Name: "project:view"}}
		pap.On("ListActions", mock.Anything).Return(actions, nil)

		result, err := svc.ListActions(ctx)
		require.NoError(t, err)
		assert.Equal(t, actions, result)
	})

	t.Run("pap error propagated", func(t *testing.T) {
		svc, pap, _ := newService(t)
		errFake := errors.New("fake error")
		pap.On("ListActions", mock.Anything).Return(nil, errFake)

		_, err := svc.ListActions(ctx)
		require.ErrorIs(t, err, errFake)
	})
}

func TestGetSubjectProfile(t *testing.T) {
	ctx := context.Background()
	req := &authzcore.ProfileRequest{
		SubjectContext: &authzcore.SubjectContext{Type: "user"},
	}

	t.Run("delegates to PDP", func(t *testing.T) {
		svc, _, pdp := newService(t)
		resp := &authzcore.UserCapabilitiesResponse{User: req.SubjectContext}
		pdp.On("GetSubjectProfile", mock.Anything, req).Return(resp, nil)

		result, err := svc.GetSubjectProfile(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, resp, result)
	})

	t.Run("pdp error propagated", func(t *testing.T) {
		svc, _, pdp := newService(t)
		errFake := errors.New("fake error")
		pdp.On("GetSubjectProfile", mock.Anything, req).Return(nil, errFake)

		_, err := svc.GetSubjectProfile(ctx, req)
		require.ErrorIs(t, err, errFake)
	})
}

// TestNewServiceWithAuthz verifies the constructor returns a non-nil Service.
func TestNewServiceWithAuthz(t *testing.T) {
	pap := authzcoremocks.NewMockPAP(t)
	pdp := authzcoremocks.NewMockPDP(t)
	svc := NewServiceWithAuthz(pap, pdp, testutil.TestLogger())
	require.NotNil(t, svc)
}
