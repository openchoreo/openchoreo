// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// Resource type strings must match unexported constants in service_authz.go.
const (
	rtClusterAuthzRole        = "clusterAuthzRole"
	rtAuthzRole               = "authzRole"
	rtClusterAuthzRoleBinding = "clusterAuthzRoleBinding"
	rtAuthzRoleBinding        = "authzRoleBinding"
)

var (
	emptyHierarchy = authzcore.ResourceHierarchy{}
	nsHierarchy    = authzcore.ResourceHierarchy{Namespace: "ns-1"}
)

// mockService is a local testify mock for the Service interface.
type mockService struct {
	mock.Mock
}

func newMockService(t *testing.T) *mockService {
	m := &mockService{}
	m.Mock.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func newTestAuthzService(t *testing.T, pdp *testutil.CapturingPDP) (*authzServiceWithAuthz, *mockService) {
	t.Helper()
	mockSvc := newMockService(t)
	svc := &authzServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}
	return svc, mockSvc
}

func (m *mockService) CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	args := m.Called(ctx, role)
	res, _ := args.Get(0).(*openchoreov1alpha1.ClusterAuthzRole)
	return res, args.Error(1)
}

func (m *mockService) GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	args := m.Called(ctx, name)
	res, _ := args.Get(0).(*openchoreov1alpha1.ClusterAuthzRole)
	return res, args.Error(1)
}

func (m *mockService) ListClusterRoles(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterAuthzRole], error) {
	args := m.Called(ctx, opts)
	res, _ := args.Get(0).(*services.ListResult[openchoreov1alpha1.ClusterAuthzRole])
	return res, args.Error(1)
}

func (m *mockService) UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	args := m.Called(ctx, role)
	res, _ := args.Get(0).(*openchoreov1alpha1.ClusterAuthzRole)
	return res, args.Error(1)
}

func (m *mockService) DeleteClusterRole(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

func (m *mockService) CreateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	args := m.Called(ctx, namespace, role)
	res, _ := args.Get(0).(*openchoreov1alpha1.AuthzRole)
	return res, args.Error(1)
}

func (m *mockService) GetNamespaceRole(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRole, error) {
	args := m.Called(ctx, namespace, name)
	res, _ := args.Get(0).(*openchoreov1alpha1.AuthzRole)
	return res, args.Error(1)
}

func (m *mockService) ListNamespaceRoles(ctx context.Context, namespace string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.AuthzRole], error) {
	args := m.Called(ctx, namespace, opts)
	res, _ := args.Get(0).(*services.ListResult[openchoreov1alpha1.AuthzRole])
	return res, args.Error(1)
}

func (m *mockService) UpdateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	args := m.Called(ctx, namespace, role)
	res, _ := args.Get(0).(*openchoreov1alpha1.AuthzRole)
	return res, args.Error(1)
}

func (m *mockService) DeleteNamespaceRole(ctx context.Context, namespace, name string) error {
	args := m.Called(ctx, namespace, name)
	return args.Error(0)
}

func (m *mockService) CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	args := m.Called(ctx, binding)
	res, _ := args.Get(0).(*openchoreov1alpha1.ClusterAuthzRoleBinding)
	return res, args.Error(1)
}

func (m *mockService) GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	args := m.Called(ctx, name)
	res, _ := args.Get(0).(*openchoreov1alpha1.ClusterAuthzRoleBinding)
	return res, args.Error(1)
}

func (m *mockService) ListClusterRoleBindings(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterAuthzRoleBinding], error) {
	args := m.Called(ctx, opts)
	res, _ := args.Get(0).(*services.ListResult[openchoreov1alpha1.ClusterAuthzRoleBinding])
	return res, args.Error(1)
}

func (m *mockService) UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	args := m.Called(ctx, binding)
	res, _ := args.Get(0).(*openchoreov1alpha1.ClusterAuthzRoleBinding)
	return res, args.Error(1)
}

func (m *mockService) DeleteClusterRoleBinding(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

func (m *mockService) CreateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	args := m.Called(ctx, namespace, binding)
	res, _ := args.Get(0).(*openchoreov1alpha1.AuthzRoleBinding)
	return res, args.Error(1)
}

func (m *mockService) GetNamespaceRoleBinding(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	args := m.Called(ctx, namespace, name)
	res, _ := args.Get(0).(*openchoreov1alpha1.AuthzRoleBinding)
	return res, args.Error(1)
}

func (m *mockService) ListNamespaceRoleBindings(ctx context.Context, namespace string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.AuthzRoleBinding], error) {
	args := m.Called(ctx, namespace, opts)
	res, _ := args.Get(0).(*services.ListResult[openchoreov1alpha1.AuthzRoleBinding])
	return res, args.Error(1)
}

func (m *mockService) UpdateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	args := m.Called(ctx, namespace, binding)
	res, _ := args.Get(0).(*openchoreov1alpha1.AuthzRoleBinding)
	return res, args.Error(1)
}

func (m *mockService) DeleteNamespaceRoleBinding(ctx context.Context, namespace, name string) error {
	args := m.Called(ctx, namespace, name)
	return args.Error(0)
}

func (m *mockService) Evaluate(ctx context.Context, requests []authzcore.EvaluateRequest) ([]authzcore.Decision, error) {
	args := m.Called(ctx, requests)
	res, _ := args.Get(0).([]authzcore.Decision)
	return res, args.Error(1)
}

func (m *mockService) ListActions(ctx context.Context) ([]authzcore.Action, error) {
	args := m.Called(ctx)
	res, _ := args.Get(0).([]authzcore.Action)
	return res, args.Error(1)
}

func (m *mockService) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	args := m.Called(ctx, request)
	res, _ := args.Get(0).(*authzcore.UserCapabilitiesResponse)
	return res, args.Error(1)
}

func testClusterAuthzRole(name string) *openchoreov1alpha1.ClusterAuthzRole {
	return &openchoreov1alpha1.ClusterAuthzRole{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func testAuthzRole(name string) *openchoreov1alpha1.AuthzRole {
	return &openchoreov1alpha1.AuthzRole{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func testClusterAuthzRoleBinding(name string) *openchoreov1alpha1.ClusterAuthzRoleBinding {
	return &openchoreov1alpha1.ClusterAuthzRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func testAuthzRoleBinding(name string) *openchoreov1alpha1.AuthzRoleBinding {
	return &openchoreov1alpha1.AuthzRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

// --- Cluster roles ---

func TestCreateClusterRole_AuthzCheck(t *testing.T) {
	role := testClusterAuthzRole("cr-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("CreateClusterRole", mock.Anything, role).Return(role, nil)
		result, err := svc.CreateClusterRole(testutil.AuthzContext(), role)
		require.NoError(t, err)
		require.Equal(t, role, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionCreateClusterAuthzRole, rtClusterAuthzRole, "cr-1", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.CreateClusterRole(testutil.AuthzContext(), role)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterRole_AuthzCheck(t *testing.T) {
	fetched := testClusterAuthzRole("cr-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("GetClusterRole", mock.Anything, "cr-1").Return(fetched, nil)
		result, err := svc.GetClusterRole(testutil.AuthzContext(), "cr-1")
		require.NoError(t, err)
		require.Equal(t, fetched, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionViewClusterAuthzRole, rtClusterAuthzRole, "cr-1", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.GetClusterRole(testutil.AuthzContext(), "cr-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterRoles_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterAuthzRole{*testClusterAuthzRole("a"), *testClusterAuthzRole("b")}
	opts := services.ListOptions{Limit: 10}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("ListClusterRoles", mock.Anything, opts).Return(&services.ListResult[openchoreov1alpha1.ClusterAuthzRole]{Items: items}, nil)
		result, err := svc.ListClusterRoles(testutil.AuthzContext(), opts)
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionViewClusterAuthzRole, rtClusterAuthzRole, "", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.ListClusterRoles(testutil.AuthzContext(), opts)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterRole_AuthzCheck(t *testing.T) {
	role := testClusterAuthzRole("cr-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("UpdateClusterRole", mock.Anything, role).Return(role, nil)
		result, err := svc.UpdateClusterRole(testutil.AuthzContext(), role)
		require.NoError(t, err)
		require.Equal(t, role, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionUpdateClusterAuthzRole, rtClusterAuthzRole, "cr-1", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.UpdateClusterRole(testutil.AuthzContext(), role)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterRole_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("DeleteClusterRole", mock.Anything, "cr-1").Return(nil)
		err := svc.DeleteClusterRole(testutil.AuthzContext(), "cr-1")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionDeleteClusterAuthzRole, rtClusterAuthzRole, "cr-1", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		err := svc.DeleteClusterRole(testutil.AuthzContext(), "cr-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- Namespace roles ---

func TestCreateNamespaceRole_AuthzCheck(t *testing.T) {
	role := testAuthzRole("nr-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("CreateNamespaceRole", mock.Anything, "ns-1", role).Return(role, nil)
		result, err := svc.CreateNamespaceRole(testutil.AuthzContext(), "ns-1", role)
		require.NoError(t, err)
		require.Equal(t, role, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionCreateAuthzRole, rtAuthzRole, "nr-1", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.CreateNamespaceRole(testutil.AuthzContext(), "ns-1", role)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetNamespaceRole_AuthzCheck(t *testing.T) {
	fetched := testAuthzRole("nr-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("GetNamespaceRole", mock.Anything, "ns-1", "nr-1").Return(fetched, nil)
		result, err := svc.GetNamespaceRole(testutil.AuthzContext(), "ns-1", "nr-1")
		require.NoError(t, err)
		require.Equal(t, fetched, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionViewAuthzRole, rtAuthzRole, "nr-1", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.GetNamespaceRole(testutil.AuthzContext(), "ns-1", "nr-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListNamespaceRoles_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.AuthzRole{*testAuthzRole("a"), *testAuthzRole("b")}
	opts := services.ListOptions{Limit: 10}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("ListNamespaceRoles", mock.Anything, "ns-1", opts).Return(&services.ListResult[openchoreov1alpha1.AuthzRole]{Items: items}, nil)
		result, err := svc.ListNamespaceRoles(testutil.AuthzContext(), "ns-1", opts)
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionViewAuthzRole, rtAuthzRole, "", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.ListNamespaceRoles(testutil.AuthzContext(), "ns-1", opts)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateNamespaceRole_AuthzCheck(t *testing.T) {
	role := testAuthzRole("nr-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("UpdateNamespaceRole", mock.Anything, "ns-1", role).Return(role, nil)
		result, err := svc.UpdateNamespaceRole(testutil.AuthzContext(), "ns-1", role)
		require.NoError(t, err)
		require.Equal(t, role, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionUpdateAuthzRole, rtAuthzRole, "nr-1", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.UpdateNamespaceRole(testutil.AuthzContext(), "ns-1", role)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteNamespaceRole_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("DeleteNamespaceRole", mock.Anything, "ns-1", "nr-1").Return(nil)
		err := svc.DeleteNamespaceRole(testutil.AuthzContext(), "ns-1", "nr-1")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionDeleteAuthzRole, rtAuthzRole, "nr-1", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		err := svc.DeleteNamespaceRole(testutil.AuthzContext(), "ns-1", "nr-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- Cluster role bindings ---

func TestCreateClusterRoleBinding_AuthzCheck(t *testing.T) {
	binding := testClusterAuthzRoleBinding("cb-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("CreateClusterRoleBinding", mock.Anything, binding).Return(binding, nil)
		result, err := svc.CreateClusterRoleBinding(testutil.AuthzContext(), binding)
		require.NoError(t, err)
		require.Equal(t, binding, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionCreateClusterAuthzRoleBinding, rtClusterAuthzRoleBinding, "cb-1", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.CreateClusterRoleBinding(testutil.AuthzContext(), binding)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterRoleBinding_AuthzCheck(t *testing.T) {
	fetched := testClusterAuthzRoleBinding("cb-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("GetClusterRoleBinding", mock.Anything, "cb-1").Return(fetched, nil)
		result, err := svc.GetClusterRoleBinding(testutil.AuthzContext(), "cb-1")
		require.NoError(t, err)
		require.Equal(t, fetched, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionViewClusterAuthzRoleBinding, rtClusterAuthzRoleBinding, "cb-1", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.GetClusterRoleBinding(testutil.AuthzContext(), "cb-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterRoleBindings_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterAuthzRoleBinding{*testClusterAuthzRoleBinding("a"), *testClusterAuthzRoleBinding("b")}
	opts := services.ListOptions{Limit: 10}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("ListClusterRoleBindings", mock.Anything, opts).Return(&services.ListResult[openchoreov1alpha1.ClusterAuthzRoleBinding]{Items: items}, nil)
		result, err := svc.ListClusterRoleBindings(testutil.AuthzContext(), opts)
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionViewClusterAuthzRoleBinding, rtClusterAuthzRoleBinding, "", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.ListClusterRoleBindings(testutil.AuthzContext(), opts)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterRoleBinding_AuthzCheck(t *testing.T) {
	binding := testClusterAuthzRoleBinding("cb-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("UpdateClusterRoleBinding", mock.Anything, binding).Return(binding, nil)
		result, err := svc.UpdateClusterRoleBinding(testutil.AuthzContext(), binding)
		require.NoError(t, err)
		require.Equal(t, binding, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionUpdateClusterAuthzRoleBinding, rtClusterAuthzRoleBinding, "cb-1", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.UpdateClusterRoleBinding(testutil.AuthzContext(), binding)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterRoleBinding_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("DeleteClusterRoleBinding", mock.Anything, "cb-1").Return(nil)
		err := svc.DeleteClusterRoleBinding(testutil.AuthzContext(), "cb-1")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionDeleteClusterAuthzRoleBinding, rtClusterAuthzRoleBinding, "cb-1", emptyHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		err := svc.DeleteClusterRoleBinding(testutil.AuthzContext(), "cb-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- Namespace role bindings ---

func TestCreateNamespaceRoleBinding_AuthzCheck(t *testing.T) {
	binding := testAuthzRoleBinding("nb-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("CreateNamespaceRoleBinding", mock.Anything, "ns-1", binding).Return(binding, nil)
		result, err := svc.CreateNamespaceRoleBinding(testutil.AuthzContext(), "ns-1", binding)
		require.NoError(t, err)
		require.Equal(t, binding, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionCreateAuthzRoleBinding, rtAuthzRoleBinding, "nb-1", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.CreateNamespaceRoleBinding(testutil.AuthzContext(), "ns-1", binding)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetNamespaceRoleBinding_AuthzCheck(t *testing.T) {
	fetched := testAuthzRoleBinding("nb-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("GetNamespaceRoleBinding", mock.Anything, "ns-1", "nb-1").Return(fetched, nil)
		result, err := svc.GetNamespaceRoleBinding(testutil.AuthzContext(), "ns-1", "nb-1")
		require.NoError(t, err)
		require.Equal(t, fetched, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionViewAuthzRoleBinding, rtAuthzRoleBinding, "nb-1", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.GetNamespaceRoleBinding(testutil.AuthzContext(), "ns-1", "nb-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListNamespaceRoleBindings_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.AuthzRoleBinding{*testAuthzRoleBinding("a"), *testAuthzRoleBinding("b")}
	opts := services.ListOptions{Limit: 10}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("ListNamespaceRoleBindings", mock.Anything, "ns-1", opts).Return(&services.ListResult[openchoreov1alpha1.AuthzRoleBinding]{Items: items}, nil)
		result, err := svc.ListNamespaceRoleBindings(testutil.AuthzContext(), "ns-1", opts)
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionViewAuthzRoleBinding, rtAuthzRoleBinding, "", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.ListNamespaceRoleBindings(testutil.AuthzContext(), "ns-1", opts)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateNamespaceRoleBinding_AuthzCheck(t *testing.T) {
	binding := testAuthzRoleBinding("nb-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("UpdateNamespaceRoleBinding", mock.Anything, "ns-1", binding).Return(binding, nil)
		result, err := svc.UpdateNamespaceRoleBinding(testutil.AuthzContext(), "ns-1", binding)
		require.NoError(t, err)
		require.Equal(t, binding, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionUpdateAuthzRoleBinding, rtAuthzRoleBinding, "nb-1", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		_, err := svc.UpdateNamespaceRoleBinding(testutil.AuthzContext(), "ns-1", binding)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteNamespaceRoleBinding_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		svc, mockSvc := newTestAuthzService(t, pdp)
		mockSvc.On("DeleteNamespaceRoleBinding", mock.Anything, "ns-1", "nb-1").Return(nil)
		err := svc.DeleteNamespaceRoleBinding(testutil.AuthzContext(), "ns-1", "nb-1")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], authzcore.ActionDeleteAuthzRoleBinding, rtAuthzRoleBinding, "nb-1", nsHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		svc, _ := newTestAuthzService(t, pdp)
		err := svc.DeleteNamespaceRoleBinding(testutil.AuthzContext(), "ns-1", "nb-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- Evaluation & profile (no wrapper authz) ---

func TestEvaluate_NoAuthzCheck(t *testing.T) {
	pdp := testutil.AllowPDP()
	svc, mockSvc := newTestAuthzService(t, pdp)
	reqs := []authzcore.EvaluateRequest{{Action: authzcore.ActionViewAuthzRole}}
	decisions := []authzcore.Decision{{Decision: true}}
	mockSvc.On("Evaluate", mock.Anything, reqs).Return(decisions, nil)
	out, err := svc.Evaluate(testutil.AuthzContext(), reqs)
	require.NoError(t, err)
	require.Equal(t, decisions, out)
	require.Empty(t, pdp.Captured, "wrapper should not run PDP for Evaluate")
}

func TestListActions_NoAuthzCheck(t *testing.T) {
	pdp := testutil.AllowPDP()
	svc, mockSvc := newTestAuthzService(t, pdp)
	actions := []authzcore.Action{{Name: authzcore.ActionViewAuthzRole}}
	mockSvc.On("ListActions", mock.Anything).Return(actions, nil)
	out, err := svc.ListActions(testutil.AuthzContext())
	require.NoError(t, err)
	require.Equal(t, actions, out)
	require.Empty(t, pdp.Captured, "wrapper should not run PDP for ListActions")
}

func TestGetSubjectProfile_NoAuthzCheck(t *testing.T) {
	pdp := testutil.AllowPDP()
	svc, mockSvc := newTestAuthzService(t, pdp)
	req := &authzcore.ProfileRequest{}
	profile := &authzcore.UserCapabilitiesResponse{}
	mockSvc.On("GetSubjectProfile", mock.Anything, req).Return(profile, nil)
	out, err := svc.GetSubjectProfile(testutil.AuthzContext(), req)
	require.NoError(t, err)
	require.Equal(t, profile, out)
	require.Empty(t, pdp.Captured, "wrapper should not run PDP for GetSubjectProfile")
}
