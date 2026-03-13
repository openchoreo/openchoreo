// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace       = "test-ns"
	testEnvironmentName = "dev"
	testChannelName     = "test-channel"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateObservabilityAlertsNotificationChannel(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		nc := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
			ObjectMeta: metav1.ObjectMeta{Name: testChannelName},
			Spec:       testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, testChannelName).Spec,
		}

		result, err := svc.CreateObservabilityAlertsNotificationChannel(ctx, testNamespace, nc)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ObservabilityAlertsNotificationChannelStatus{}, result.Status)
		assert.Equal(t, observabilityAlertsNotificationChannelTypeMeta, result.TypeMeta)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateObservabilityAlertsNotificationChannel(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, testChannelName)
		svc := newService(t, existing)
		nc := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
			ObjectMeta: metav1.ObjectMeta{Name: testChannelName},
			Spec:       existing.Spec,
		}

		_, err := svc.CreateObservabilityAlertsNotificationChannel(ctx, testNamespace, nc)
		require.ErrorIs(t, err, ErrObservabilityAlertsNotificationChannelAlreadyExists)
	})
}

func TestUpdateObservabilityAlertsNotificationChannel(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, testChannelName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testChannelName,
				Labels: map[string]string{"env": "prod"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateObservabilityAlertsNotificationChannel(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, observabilityAlertsNotificationChannelTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, openchoreov1alpha1.ObservabilityAlertsNotificationChannelStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateObservabilityAlertsNotificationChannel(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		nc := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateObservabilityAlertsNotificationChannel(ctx, testNamespace, nc)
		require.ErrorIs(t, err, ErrObservabilityAlertsNotificationChannelNotFound)
	})

	t.Run("immutable environment", func(t *testing.T) {
		existing := testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, testChannelName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
			ObjectMeta: metav1.ObjectMeta{Name: testChannelName},
			Spec:       existing.Spec,
		}
		update.Spec.Environment = "prod"

		_, err := svc.UpdateObservabilityAlertsNotificationChannel(ctx, testNamespace, update)
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
		assert.Contains(t, err.Error(), "spec.environment is immutable")
	})
}

func TestListObservabilityAlertsNotificationChannels(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		nc1 := testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, "channel-1")
		nc2 := testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, "channel-2")
		svc := newService(t, nc1, nc2)

		result, err := svc.ListObservabilityAlertsNotificationChannels(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, observabilityAlertsNotificationChannelTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListObservabilityAlertsNotificationChannels(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		ncInNs := testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, "channel-in")
		ncOtherNs := testutil.NewObservabilityAlertsNotificationChannel("other-ns", testEnvironmentName, "channel-out")
		svc := newService(t, ncInNs, ncOtherNs)

		result, err := svc.ListObservabilityAlertsNotificationChannels(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "channel-in", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListObservabilityAlertsNotificationChannels(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetObservabilityAlertsNotificationChannel(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		nc := testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, testChannelName)
		svc := newService(t, nc)

		result, err := svc.GetObservabilityAlertsNotificationChannel(ctx, testNamespace, testChannelName)
		require.NoError(t, err)
		assert.Equal(t, observabilityAlertsNotificationChannelTypeMeta, result.TypeMeta)
		assert.Equal(t, testChannelName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetObservabilityAlertsNotificationChannel(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrObservabilityAlertsNotificationChannelNotFound)
	})
}

func TestDeleteObservabilityAlertsNotificationChannel(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		nc := testutil.NewObservabilityAlertsNotificationChannel(testNamespace, testEnvironmentName, testChannelName)
		svc := newService(t, nc)

		err := svc.DeleteObservabilityAlertsNotificationChannel(ctx, testNamespace, testChannelName)
		require.NoError(t, err)

		_, err = svc.GetObservabilityAlertsNotificationChannel(ctx, testNamespace, testChannelName)
		require.ErrorIs(t, err, ErrObservabilityAlertsNotificationChannelNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteObservabilityAlertsNotificationChannel(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrObservabilityAlertsNotificationChannelNotFound)
	})
}
