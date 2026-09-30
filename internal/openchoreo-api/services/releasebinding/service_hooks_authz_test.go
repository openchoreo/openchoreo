// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newRBAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &releaseBindingServiceWithAuthz{internal: internal, authz: testutil.NewTestAuthzChecker(pdp)}
}

// ListHooks is a read and must be gated by releasebinding:view, using the owner hierarchy
// fetched from the binding (not trusted from the caller).
func TestListHooks_AuthzCheck(t *testing.T) {
	rb := testRB()
	gate := &openchoreov1alpha1.DeploymentGateStatus{Key: "k"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		mockSvc.On("ListHooks", mock.Anything, "ns-1", "my-rb").Return(gate, nil)
		result, err := newRBAuthzSvc(pdp, mockSvc).ListHooks(testutil.AuthzContext(), "ns-1", "my-rb")
		require.NoError(t, err)
		require.Equal(t, gate, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:view", "releasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		_, err := newRBAuthzSvc(pdp, mockSvc).ListHooks(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// Retrying re-runs a hook against the environment, which is an update of the binding's
// deployment; it must be gated by releasebinding:update.
func TestRetryHook_AuthzCheck(t *testing.T) {
	rb := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		mockSvc.On("RetryHook", mock.Anything, "ns-1", "my-rb", HookPhasePreDeploy, "scan").Return(rb, nil)
		_, err := newRBAuthzSvc(pdp, mockSvc).RetryHook(testutil.AuthzContext(), "ns-1", "my-rb", HookPhasePreDeploy, "scan")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:update", "releasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		_, err := newRBAuthzSvc(pdp, mockSvc).RetryHook(testutil.AuthzContext(), "ns-1", "my-rb", HookPhasePreDeploy, "scan")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found skips authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil, ErrReleaseBindingNotFound)
		_, err := newRBAuthzSvc(pdp, mockSvc).RetryHook(testutil.AuthzContext(), "ns-1", "my-rb", HookPhasePreDeploy, "scan")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
		require.Empty(t, pdp.Captured)
	})
}

// Acknowledging silences an alert without changing the spec, so it has its own action:
// an SRE can be granted releasebinding:acknowledge-gate without releasebinding:update.
func TestAcknowledgeGate_AuthzCheck(t *testing.T) {
	rb := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		mockSvc.On("AcknowledgeGate", mock.Anything, "ns-1", "my-rb", "k").Return(rb, nil)
		_, err := newRBAuthzSvc(pdp, mockSvc).AcknowledgeGate(testutil.AuthzContext(), "ns-1", "my-rb", "k")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:acknowledge-gate", "releasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		_, err := newRBAuthzSvc(pdp, mockSvc).AcknowledgeGate(testutil.AuthzContext(), "ns-1", "my-rb", "k")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}
