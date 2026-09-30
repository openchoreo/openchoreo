// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterhook/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterHookServiceWithAuthz{internal: internal, authz: testutil.NewTestAuthzChecker(pdp)}
}

// Cluster hooks are cluster-scoped: every check must carry an empty hierarchy and the
// clusterhook:<verb> action, so namespace-level roles cannot reach them.
func TestClusterHookService_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterHook{ObjectMeta: metav1.ObjectMeta{Name: "my-ch"}}

	tests := []struct {
		name   string
		action string
		call   func(svc Service) error
		expect func(m *mocks.MockService)
	}{
		{"create", "clusterhook:create",
			func(svc Service) error {
				_, err := svc.CreateClusterHook(testutil.AuthzContext(), resource)
				return err
			},
			func(m *mocks.MockService) { m.On("CreateClusterHook", mock.Anything, resource).Return(resource, nil) }},
		{"update", "clusterhook:update",
			func(svc Service) error {
				_, err := svc.UpdateClusterHook(testutil.AuthzContext(), resource)
				return err
			},
			func(m *mocks.MockService) { m.On("UpdateClusterHook", mock.Anything, resource).Return(resource, nil) }},
		{"get", "clusterhook:view",
			func(svc Service) error { _, err := svc.GetClusterHook(testutil.AuthzContext(), "my-ch"); return err },
			func(m *mocks.MockService) { m.On("GetClusterHook", mock.Anything, "my-ch").Return(resource, nil) }},
		{"delete", "clusterhook:delete",
			func(svc Service) error { return svc.DeleteClusterHook(testutil.AuthzContext(), "my-ch") },
			func(m *mocks.MockService) { m.On("DeleteClusterHook", mock.Anything, "my-ch").Return(nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name+" allowed", func(t *testing.T) {
			pdp := testutil.AllowPDP()
			mockSvc := mocks.NewMockService(t)
			tt.expect(mockSvc)
			require.NoError(t, tt.call(newAuthzSvc(pdp, mockSvc)))
			require.Len(t, pdp.Captured, 1)
			testutil.RequireEvalRequest(t, pdp.Captured[0], tt.action, "clusterHook", "my-ch", authzcore.ResourceHierarchy{})
		})
		t.Run(tt.name+" denied", func(t *testing.T) {
			pdp := testutil.DenyPDP()
			mockSvc := mocks.NewMockService(t)
			require.ErrorIs(t, tt.call(newAuthzSvc(pdp, mockSvc)), services.ErrForbidden)
		})
	}

	t.Run("list filters denied items", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterHooks", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterHook]{Items: []openchoreov1alpha1.ClusterHook{*resource}}, nil)
		result, err := newAuthzSvc(pdp, mockSvc).ListClusterHooks(testutil.AuthzContext(), services.ListOptions{})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
