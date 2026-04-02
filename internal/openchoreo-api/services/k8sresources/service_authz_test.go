// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newAuthzSvcWithRB(t *testing.T, pdp *testutil.CapturingPDP) (*mocks.MockService, k8sresources.Service) {
	t.Helper()
	rb := testutil.NewReleaseBinding("ns-1", "proj-1", "comp-1", "dev", "rb-1")
	fakeClient := testutil.NewFakeClient(rb)
	mockSvc := mocks.NewMockService(t)
	svc := k8sresources.NewTestServiceWithAuthz(mockSvc, fakeClient, pdp, testutil.TestLogger())
	return mockSvc, svc
}

// --- GetResourceTree ---

func TestGetResourceTree_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc, svc := newAuthzSvcWithRB(t, pdp)
		expected := &k8sresources.K8sResourceTreeResult{}
		mockSvc.On("GetResourceTree", mock.Anything, "ns-1", "rb-1").Return(expected, nil)

		result, err := svc.GetResourceTree(testutil.AuthzContext(), "ns-1", "rb-1")
		require.NoError(t, err)
		require.Equal(t, expected, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0],
			"releasebinding:view", "releasebinding", "rb-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "proj-1", Component: "comp-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		_, svc := newAuthzSvcWithRB(t, pdp)

		_, err := svc.GetResourceTree(testutil.AuthzContext(), "ns-1", "rb-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("release binding not found skips authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		_, svc := newAuthzSvcWithRB(t, pdp)

		_, err := svc.GetResourceTree(testutil.AuthzContext(), "ns-1", "nonexistent")
		require.ErrorIs(t, err, k8sresources.ErrReleaseBindingNotFound)
		require.Empty(t, pdp.Captured)
	})

	t.Run("pdp error propagates", func(t *testing.T) {
		pdpErr := errors.New("pdp unavailable")
		pdp := testutil.ErrorPDP(pdpErr)
		_, svc := newAuthzSvcWithRB(t, pdp)

		_, err := svc.GetResourceTree(testutil.AuthzContext(), "ns-1", "rb-1")
		require.Error(t, err)
		require.NotErrorIs(t, err, k8sresources.ErrReleaseBindingNotFound)
	})
}

// --- GetResourceEvents ---

func TestGetResourceEvents_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc, svc := newAuthzSvcWithRB(t, pdp)
		expected := &models.ResourceEventsResponse{Events: []models.ResourceEvent{{Reason: "Scheduled"}}}
		mockSvc.On("GetResourceEvents", mock.Anything, "ns-1", "rb-1", "apps", "v1", "Deployment", "web").
			Return(expected, nil)

		result, err := svc.GetResourceEvents(testutil.AuthzContext(), "ns-1", "rb-1", "apps", "v1", "Deployment", "web")
		require.NoError(t, err)
		require.Equal(t, expected, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0],
			"releasebinding:view", "releasebinding", "rb-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "proj-1", Component: "comp-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		_, svc := newAuthzSvcWithRB(t, pdp)

		_, err := svc.GetResourceEvents(testutil.AuthzContext(), "ns-1", "rb-1", "apps", "v1", "Deployment", "web")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("release binding not found skips authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		_, svc := newAuthzSvcWithRB(t, pdp)

		_, err := svc.GetResourceEvents(testutil.AuthzContext(), "ns-1", "nonexistent", "apps", "v1", "Deployment", "web")
		require.ErrorIs(t, err, k8sresources.ErrReleaseBindingNotFound)
		require.Empty(t, pdp.Captured)
	})
}

// --- GetResourceLogs ---

func TestGetResourceLogs_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc, svc := newAuthzSvcWithRB(t, pdp)
		expected := &models.ResourcePodLogsResponse{LogEntries: []models.PodLogEntry{{Log: "hello"}}}
		mockSvc.On("GetResourceLogs", mock.Anything, "ns-1", "rb-1", "pod-1", (*int64)(nil)).
			Return(expected, nil)

		result, err := svc.GetResourceLogs(testutil.AuthzContext(), "ns-1", "rb-1", "pod-1", nil)
		require.NoError(t, err)
		require.Equal(t, expected, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0],
			"releasebinding:view", "releasebinding", "rb-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "proj-1", Component: "comp-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		_, svc := newAuthzSvcWithRB(t, pdp)

		_, err := svc.GetResourceLogs(testutil.AuthzContext(), "ns-1", "rb-1", "pod-1", nil)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("release binding not found skips authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		_, svc := newAuthzSvcWithRB(t, pdp)

		_, err := svc.GetResourceLogs(testutil.AuthzContext(), "ns-1", "nonexistent", "pod-1", nil)
		require.ErrorIs(t, err, k8sresources.ErrReleaseBindingNotFound)
		require.Empty(t, pdp.Captured)
	})

	t.Run("with sinceSeconds", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc, svc := newAuthzSvcWithRB(t, pdp)
		since := int64(300)
		expected := &models.ResourcePodLogsResponse{LogEntries: []models.PodLogEntry{{Log: "recent"}}}
		mockSvc.On("GetResourceLogs", mock.Anything, "ns-1", "rb-1", "pod-1", &since).
			Return(expected, nil)

		result, err := svc.GetResourceLogs(testutil.AuthzContext(), "ns-1", "rb-1", "pod-1", &since)
		require.NoError(t, err)
		require.Equal(t, expected, result)
	})
}
