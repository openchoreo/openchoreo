// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace   = "test-ns"
	testRoleName    = "test-role"
	testBindingName = "test-binding"
)

// --- Mock PAP ---

type mockPAP struct {
	clusterRoles           map[string]*openchoreov1alpha1.ClusterAuthzRole
	namespacedRoles        map[string]*openchoreov1alpha1.AuthzRole
	clusterRoleBindings    map[string]*openchoreov1alpha1.ClusterAuthzRoleBinding
	namespacedRoleBindings map[string]*openchoreov1alpha1.AuthzRoleBinding
}

func newMockPAP() *mockPAP {
	return &mockPAP{
		clusterRoles:           make(map[string]*openchoreov1alpha1.ClusterAuthzRole),
		namespacedRoles:        make(map[string]*openchoreov1alpha1.AuthzRole),
		clusterRoleBindings:    make(map[string]*openchoreov1alpha1.ClusterAuthzRoleBinding),
		namespacedRoleBindings: make(map[string]*openchoreov1alpha1.AuthzRoleBinding),
	}
}

func nsKey(namespace, name string) string { return namespace + "/" + name }

// Cluster Roles

func (m *mockPAP) CreateClusterRole(_ context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	stored := role.DeepCopy()
	m.clusterRoles[role.Name] = stored
	return stored.DeepCopy(), nil
}

func (m *mockPAP) GetClusterRole(context.Context, string) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	return nil, nil
}

func (m *mockPAP) ListClusterRoles(_ context.Context, _ int, _ string) (*authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRole], error) {
	items := make([]openchoreov1alpha1.ClusterAuthzRole, 0, len(m.clusterRoles))
	for _, r := range m.clusterRoles {
		items = append(items, *r.DeepCopy())
	}
	return &authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRole]{Items: items, NextCursor: "next-cr"}, nil
}

func (m *mockPAP) UpdateClusterRole(_ context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	stored := role.DeepCopy()
	m.clusterRoles[role.Name] = stored
	return stored.DeepCopy(), nil
}

func (m *mockPAP) DeleteClusterRole(context.Context, string) error { return nil }

// Namespaced Roles

func (m *mockPAP) CreateNamespacedRole(_ context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	stored := role.DeepCopy()
	m.namespacedRoles[nsKey(role.Namespace, role.Name)] = stored
	return stored.DeepCopy(), nil
}

func (m *mockPAP) GetNamespacedRole(context.Context, string, string) (*openchoreov1alpha1.AuthzRole, error) {
	return nil, nil
}

func (m *mockPAP) ListNamespacedRoles(_ context.Context, namespace string, _ int, _ string) (*authzcore.PaginatedList[openchoreov1alpha1.AuthzRole], error) {
	items := make([]openchoreov1alpha1.AuthzRole, 0)
	for k, r := range m.namespacedRoles {
		if len(k) > len(namespace) && k[:len(namespace)] == namespace && k[len(namespace)] == '/' {
			items = append(items, *r.DeepCopy())
		}
	}
	return &authzcore.PaginatedList[openchoreov1alpha1.AuthzRole]{Items: items, NextCursor: "next-nr"}, nil
}

func (m *mockPAP) UpdateNamespacedRole(_ context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	stored := role.DeepCopy()
	m.namespacedRoles[nsKey(role.Namespace, role.Name)] = stored
	return stored.DeepCopy(), nil
}

func (m *mockPAP) DeleteNamespacedRole(context.Context, string, string) error { return nil }

// Cluster Role Bindings

func (m *mockPAP) CreateClusterRoleBinding(_ context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	stored := binding.DeepCopy()
	m.clusterRoleBindings[binding.Name] = stored
	return stored.DeepCopy(), nil
}

func (m *mockPAP) GetClusterRoleBinding(context.Context, string) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	return nil, nil
}

func (m *mockPAP) ListClusterRoleBindings(_ context.Context, _ int, _ string) (*authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRoleBinding], error) {
	items := make([]openchoreov1alpha1.ClusterAuthzRoleBinding, 0, len(m.clusterRoleBindings))
	for _, b := range m.clusterRoleBindings {
		items = append(items, *b.DeepCopy())
	}
	return &authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRoleBinding]{Items: items, NextCursor: "next-crb"}, nil
}

func (m *mockPAP) UpdateClusterRoleBinding(_ context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	stored := binding.DeepCopy()
	m.clusterRoleBindings[binding.Name] = stored
	return stored.DeepCopy(), nil
}

func (m *mockPAP) DeleteClusterRoleBinding(context.Context, string) error { return nil }

// Namespaced Role Bindings

func (m *mockPAP) CreateNamespacedRoleBinding(_ context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	stored := binding.DeepCopy()
	m.namespacedRoleBindings[nsKey(binding.Namespace, binding.Name)] = stored
	return stored.DeepCopy(), nil
}

func (m *mockPAP) GetNamespacedRoleBinding(context.Context, string, string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	return nil, nil
}

func (m *mockPAP) ListNamespacedRoleBindings(_ context.Context, namespace string, _ int, _ string) (*authzcore.PaginatedList[openchoreov1alpha1.AuthzRoleBinding], error) {
	items := make([]openchoreov1alpha1.AuthzRoleBinding, 0)
	for k, b := range m.namespacedRoleBindings {
		if len(k) > len(namespace) && k[:len(namespace)] == namespace && k[len(namespace)] == '/' {
			items = append(items, *b.DeepCopy())
		}
	}
	return &authzcore.PaginatedList[openchoreov1alpha1.AuthzRoleBinding]{Items: items, NextCursor: "next-nrb"}, nil
}

func (m *mockPAP) UpdateNamespacedRoleBinding(_ context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	stored := binding.DeepCopy()
	m.namespacedRoleBindings[nsKey(binding.Namespace, binding.Name)] = stored
	return stored.DeepCopy(), nil
}

func (m *mockPAP) DeleteNamespacedRoleBinding(context.Context, string, string) error { return nil }

func (m *mockPAP) ListActions(context.Context) ([]authzcore.Action, error) { return nil, nil }

// --- Mock PDP ---

type mockPDP struct {
	decisions []authzcore.Decision
	evalErr   error
}

func (m *mockPDP) Evaluate(context.Context, *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	panic("not used by service")
}

func (m *mockPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	if m.evalErr != nil {
		return nil, m.evalErr
	}
	return &authzcore.BatchEvaluateResponse{Decisions: m.decisions}, nil
}

func (m *mockPDP) GetSubjectProfile(context.Context, *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

// --- Helper ---

func newService(t *testing.T) (Service, *mockPAP, *mockPDP) {
	t.Helper()
	pap := newMockPAP()
	pdp := &mockPDP{}
	svc := NewService(pap, pdp, testutil.TestLogger())
	return svc, pap, pdp
}

// --- Cluster Roles: TypeMeta stamping + nil guard ---

func TestCreateClusterRole(t *testing.T) {
	ctx := context.Background()

	t.Run("success stamps TypeMeta", func(t *testing.T) {
		svc, _, _ := newService(t)
		result, err := svc.CreateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName},
		})
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.CreateClusterRole(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

func TestUpdateClusterRole(t *testing.T) {
	ctx := context.Background()

	t.Run("success stamps TypeMeta", func(t *testing.T) {
		svc, _, _ := newService(t)
		result, err := svc.UpdateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName},
		})
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.UpdateClusterRole(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

func TestListClusterRoles(t *testing.T) {
	ctx := context.Background()
	svc, pap, _ := newService(t)
	pap.clusterRoles["r1"] = &openchoreov1alpha1.ClusterAuthzRole{ObjectMeta: metav1.ObjectMeta{Name: "r1"}}
	pap.clusterRoles["r2"] = &openchoreov1alpha1.ClusterAuthzRole{ObjectMeta: metav1.ObjectMeta{Name: "r2"}}

	result, err := svc.ListClusterRoles(ctx, services.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
	for _, item := range result.Items {
		assert.Equal(t, clusterAuthzRoleTypeMeta, item.TypeMeta)
	}
	assert.Equal(t, "next-cr", result.NextCursor)
}

// --- Namespace Roles: TypeMeta + namespace injection + nil guard ---

func TestCreateNamespaceRole(t *testing.T) {
	ctx := context.Background()

	t.Run("sets namespace and stamps TypeMeta", func(t *testing.T) {
		svc, _, _ := newService(t)
		result, err := svc.CreateNamespaceRole(ctx, testNamespace, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName},
		})
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
}

func TestUpdateNamespaceRole(t *testing.T) {
	ctx := context.Background()

	t.Run("sets namespace and stamps TypeMeta", func(t *testing.T) {
		svc, _, _ := newService(t)
		result, err := svc.UpdateNamespaceRole(ctx, testNamespace, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: testRoleName},
		})
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
}

func TestListNamespaceRoles(t *testing.T) {
	ctx := context.Background()
	svc, pap, _ := newService(t)
	pap.namespacedRoles[nsKey(testNamespace, "r1")] = &openchoreov1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: testNamespace},
	}

	result, err := svc.ListNamespaceRoles(ctx, testNamespace, services.ListOptions{})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, authzRoleTypeMeta, result.Items[0].TypeMeta)
	assert.Equal(t, "next-nr", result.NextCursor)
}

// --- Cluster Role Bindings: TypeMeta stamping + nil guard ---

func TestCreateClusterRoleBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success stamps TypeMeta", func(t *testing.T) {
		svc, _, _ := newService(t)
		result, err := svc.CreateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName},
		})
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleBindingTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.CreateClusterRoleBinding(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

func TestUpdateClusterRoleBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success stamps TypeMeta", func(t *testing.T) {
		svc, _, _ := newService(t)
		result, err := svc.UpdateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName},
		})
		require.NoError(t, err)
		assert.Equal(t, clusterAuthzRoleBindingTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc, _, _ := newService(t)
		_, err := svc.UpdateClusterRoleBinding(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

func TestListClusterRoleBindings(t *testing.T) {
	ctx := context.Background()
	svc, pap, _ := newService(t)
	pap.clusterRoleBindings["b1"] = &openchoreov1alpha1.ClusterAuthzRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "b1"}}

	result, err := svc.ListClusterRoleBindings(ctx, services.ListOptions{})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, clusterAuthzRoleBindingTypeMeta, result.Items[0].TypeMeta)
	assert.Equal(t, "next-crb", result.NextCursor)
}

// --- Namespace Role Bindings: TypeMeta + namespace injection + nil guard ---

func TestCreateNamespaceRoleBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("sets namespace and stamps TypeMeta", func(t *testing.T) {
		svc, _, _ := newService(t)
		result, err := svc.CreateNamespaceRoleBinding(ctx, testNamespace, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName},
		})
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
}

func TestUpdateNamespaceRoleBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("sets namespace and stamps TypeMeta", func(t *testing.T) {
		svc, _, _ := newService(t)
		result, err := svc.UpdateNamespaceRoleBinding(ctx, testNamespace, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testBindingName},
		})
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
}

func TestListNamespaceRoleBindings(t *testing.T) {
	ctx := context.Background()
	svc, pap, _ := newService(t)
	pap.namespacedRoleBindings[nsKey(testNamespace, "b1")] = &openchoreov1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b1", Namespace: testNamespace},
	}

	result, err := svc.ListNamespaceRoleBindings(ctx, testNamespace, services.ListOptions{})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, authzRoleBindingTypeMeta, result.Items[0].TypeMeta)
	assert.Equal(t, "next-nrb", result.NextCursor)
}

// --- Evaluate: error wrapping ---

func TestEvaluate(t *testing.T) {
	ctx := context.Background()

	t.Run("wraps PDP error", func(t *testing.T) {
		svc, _, pdp := newService(t)
		pdp.evalErr = fmt.Errorf("pdp unavailable")
		_, err := svc.Evaluate(ctx, []authzcore.EvaluateRequest{{Action: "project:view"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to evaluate")
		assert.ErrorIs(t, err, pdp.evalErr)
	})
}
