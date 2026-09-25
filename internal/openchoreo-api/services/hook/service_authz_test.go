// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/hook/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

var hookHierarchy = authzcore.ResourceHierarchy{Namespace: "ns-1"}

func newAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &hookServiceWithAuthz{internal: internal, authz: testutil.NewTestAuthzChecker(pdp)}
}

func testHook() *openchoreov1alpha1.Hook {
	return &openchoreov1alpha1.Hook{ObjectMeta: metav1.ObjectMeta{Name: "my-hook", Namespace: "ns-1"}}
}

// Every mutation must be gated by its own hook:<verb> action scoped to the namespace,
// so a role that can view hooks cannot silently create or change them.
func TestHookService_AuthzCheck(t *testing.T) {
	resource := testHook()

	tests := []struct {
		name   string
		action string
		call   func(svc Service) error
		expect func(m *mocks.MockService)
	}{
		{"create", "hook:create",
			func(svc Service) error {
				_, err := svc.CreateHook(testutil.AuthzContext(), "ns-1", resource)
				return err
			},
			func(m *mocks.MockService) { m.On("CreateHook", mock.Anything, "ns-1", resource).Return(resource, nil) }},
		{"update", "hook:update",
			func(svc Service) error {
				_, err := svc.UpdateHook(testutil.AuthzContext(), "ns-1", resource)
				return err
			},
			func(m *mocks.MockService) { m.On("UpdateHook", mock.Anything, "ns-1", resource).Return(resource, nil) }},
		{"get", "hook:view",
			func(svc Service) error { _, err := svc.GetHook(testutil.AuthzContext(), "ns-1", "my-hook"); return err },
			func(m *mocks.MockService) { m.On("GetHook", mock.Anything, "ns-1", "my-hook").Return(resource, nil) }},
		{"delete", "hook:delete",
			func(svc Service) error { return svc.DeleteHook(testutil.AuthzContext(), "ns-1", "my-hook") },
			func(m *mocks.MockService) { m.On("DeleteHook", mock.Anything, "ns-1", "my-hook").Return(nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name+" allowed", func(t *testing.T) {
			pdp := testutil.AllowPDP()
			mockSvc := mocks.NewMockService(t)
			tt.expect(mockSvc)
			require.NoError(t, tt.call(newAuthzSvc(pdp, mockSvc)))
			require.Len(t, pdp.Captured, 1)
			testutil.RequireEvalRequest(t, pdp.Captured[0], tt.action, "hook", "my-hook", hookHierarchy)
		})
		t.Run(tt.name+" denied", func(t *testing.T) {
			pdp := testutil.DenyPDP()
			mockSvc := mocks.NewMockService(t) // no expectations: the internal service must not be reached
			require.ErrorIs(t, tt.call(newAuthzSvc(pdp, mockSvc)), services.ErrForbidden)
		})
	}
}

func TestListHooks_AuthzFilters(t *testing.T) {
	items := []openchoreov1alpha1.Hook{*testHook()}

	t.Run("allowed items pass through", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListHooks", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Hook]{Items: items}, nil)
		result, err := newAuthzSvc(pdp, mockSvc).ListHooks(testutil.AuthzContext(), "ns-1", services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "hook:view", "hook", "my-hook", hookHierarchy)
	})

	t.Run("denied items are filtered out, not an error", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListHooks", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Hook]{Items: items}, nil)
		result, err := newAuthzSvc(pdp, mockSvc).ListHooks(testutil.AuthzContext(), "ns-1", services.ListOptions{})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
