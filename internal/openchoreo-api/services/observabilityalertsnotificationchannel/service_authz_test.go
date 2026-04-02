// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityalertsnotificationchannel/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newObservabilityAlertsNotificationChannelAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &observabilityAlertsNotificationChannelServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func testObservabilityAlertsNotificationChannel(ns, name string) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	return &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
}

func TestCreateObservabilityAlertsNotificationChannel_AuthzCheck(t *testing.T) {
	nc := testObservabilityAlertsNotificationChannel("ns-1", "my-channel")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateObservabilityAlertsNotificationChannel", mock.Anything, "ns-1", nc).Return(nc, nil)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateObservabilityAlertsNotificationChannel(testutil.AuthzContext(), "ns-1", nc)
		require.NoError(t, err)
		require.Equal(t, nc, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityalertsnotificationchannel:create", "observabilityAlertsNotificationChannel", "my-channel",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateObservabilityAlertsNotificationChannel(testutil.AuthzContext(), "ns-1", nc)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateObservabilityAlertsNotificationChannel_AuthzCheck(t *testing.T) {
	nc := testObservabilityAlertsNotificationChannel("ns-1", "my-channel")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateObservabilityAlertsNotificationChannel", mock.Anything, "ns-1", nc).Return(nc, nil)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateObservabilityAlertsNotificationChannel(testutil.AuthzContext(), "ns-1", nc)
		require.NoError(t, err)
		require.Equal(t, nc, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityalertsnotificationchannel:update", "observabilityAlertsNotificationChannel", "my-channel",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateObservabilityAlertsNotificationChannel(testutil.AuthzContext(), "ns-1", nc)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetObservabilityAlertsNotificationChannel_AuthzCheck(t *testing.T) {
	nc := testObservabilityAlertsNotificationChannel("ns-1", "my-channel")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetObservabilityAlertsNotificationChannel", mock.Anything, "ns-1", "my-channel").Return(nc, nil)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		result, err := svc.GetObservabilityAlertsNotificationChannel(testutil.AuthzContext(), "ns-1", "my-channel")
		require.NoError(t, err)
		require.Equal(t, nc, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityalertsnotificationchannel:view", "observabilityAlertsNotificationChannel", "my-channel",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		_, err := svc.GetObservabilityAlertsNotificationChannel(testutil.AuthzContext(), "ns-1", "my-channel")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteObservabilityAlertsNotificationChannel_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteObservabilityAlertsNotificationChannel", mock.Anything, "ns-1", "my-channel").Return(nil)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		err := svc.DeleteObservabilityAlertsNotificationChannel(testutil.AuthzContext(), "ns-1", "my-channel")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityalertsnotificationchannel:delete", "observabilityAlertsNotificationChannel", "my-channel",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		err := svc.DeleteObservabilityAlertsNotificationChannel(testutil.AuthzContext(), "ns-1", "my-channel")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListObservabilityAlertsNotificationChannels_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		{ObjectMeta: metav1.ObjectMeta{Name: "channel-1", Namespace: "ns-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "channel-2", Namespace: "ns-1"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListObservabilityAlertsNotificationChannels", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ObservabilityAlertsNotificationChannel]{Items: items}, nil)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		result, err := svc.ListObservabilityAlertsNotificationChannels(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityalertsnotificationchannel:view", "observabilityAlertsNotificationChannel", "channel-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "observabilityalertsnotificationchannel:view", "observabilityAlertsNotificationChannel", "channel-2",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListObservabilityAlertsNotificationChannels", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ObservabilityAlertsNotificationChannel]{Items: items}, nil)
		svc := newObservabilityAlertsNotificationChannelAuthzSvc(pdp, mockSvc)
		result, err := svc.ListObservabilityAlertsNotificationChannels(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
