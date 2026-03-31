// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	observabilityalertsnotificationchannelsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityalertsnotificationchannel"
)

func newObservabilityAlertsNotificationChannelService(
	t *testing.T, objects []client.Object, pdp authzcore.PDP,
) observabilityalertsnotificationchannelsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return observabilityalertsnotificationchannelsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithObservabilityAlertsNotificationChannelService(
	svc observabilityalertsnotificationchannelsvc.Service,
) *Handler {
	return &Handler{
		services: &handlerservices.Services{ObservabilityAlertsNotificationChannelService: svc},
		logger:   slog.Default(),
	}
}

func testObservabilityAlertsNotificationChannelObj(
	name string,
) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	return &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
	}
}

// --- ListObservabilityAlertsNotificationChannels Handler ---

func TestListObservabilityAlertsNotificationChannelsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		objs := []client.Object{testObservabilityAlertsNotificationChannelObj("nc-1")}
		svc := newObservabilityAlertsNotificationChannelService(t, objs, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.ListObservabilityAlertsNotificationChannels(ctx,
			gen.ListObservabilityAlertsNotificationChannelsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListObservabilityAlertsNotificationChannels200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.ListObservabilityAlertsNotificationChannels(ctx,
			gen.ListObservabilityAlertsNotificationChannelsRequestObject{
				NamespaceName: ns,
				Params: gen.ListObservabilityAlertsNotificationChannelsParams{
					LabelSelector: ptr.To("===invalid"),
				},
			})
		require.NoError(t, err)
		assert.IsType(t, gen.ListObservabilityAlertsNotificationChannels400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.ListObservabilityAlertsNotificationChannels(ctx,
			gen.ListObservabilityAlertsNotificationChannelsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListObservabilityAlertsNotificationChannels200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetObservabilityAlertsNotificationChannel Handler ---

func TestGetObservabilityAlertsNotificationChannelHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testObservabilityAlertsNotificationChannelObj("nc-1")}
		svc := newObservabilityAlertsNotificationChannelService(t, objs, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.GetObservabilityAlertsNotificationChannel(ctx,
			gen.GetObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nc-1",
			})
		require.NoError(t, err)
		_, ok := resp.(gen.GetObservabilityAlertsNotificationChannel200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.GetObservabilityAlertsNotificationChannel(ctx,
			gen.GetObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nonexistent",
			})
		require.NoError(t, err)
		assert.IsType(t, gen.GetObservabilityAlertsNotificationChannel404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testObservabilityAlertsNotificationChannelObj("nc-1")}
		svc := newObservabilityAlertsNotificationChannelService(t, objs, &denyAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.GetObservabilityAlertsNotificationChannel(ctx,
			gen.GetObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nc-1",
			})
		require.NoError(t, err)
		assert.IsType(t, gen.GetObservabilityAlertsNotificationChannel403JSONResponse{}, resp)
	})
}

// --- CreateObservabilityAlertsNotificationChannel Handler ---

func TestCreateObservabilityAlertsNotificationChannelHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.CreateObservabilityAlertsNotificationChannel(ctx,
			gen.CreateObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				Body: &gen.ObservabilityAlertsNotificationChannel{
					Metadata: gen.ObjectMeta{Name: "new-nc"},
				},
			})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateObservabilityAlertsNotificationChannel201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.CreateObservabilityAlertsNotificationChannel(ctx,
			gen.CreateObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				Body:          nil,
			})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateObservabilityAlertsNotificationChannel400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		objs := []client.Object{testObservabilityAlertsNotificationChannelObj("new-nc")}
		svc := newObservabilityAlertsNotificationChannelService(t, objs, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.CreateObservabilityAlertsNotificationChannel(ctx,
			gen.CreateObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				Body: &gen.ObservabilityAlertsNotificationChannel{
					Metadata: gen.ObjectMeta{Name: "new-nc"},
				},
			})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateObservabilityAlertsNotificationChannel409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &denyAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.CreateObservabilityAlertsNotificationChannel(ctx,
			gen.CreateObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				Body: &gen.ObservabilityAlertsNotificationChannel{
					Metadata: gen.ObjectMeta{Name: "new-nc"},
				},
			})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateObservabilityAlertsNotificationChannel403JSONResponse{}, resp)
	})
}

// --- UpdateObservabilityAlertsNotificationChannel Handler ---

func TestUpdateObservabilityAlertsNotificationChannelHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testObservabilityAlertsNotificationChannelObj("nc-1")}
		svc := newObservabilityAlertsNotificationChannelService(t, objs, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.UpdateObservabilityAlertsNotificationChannel(ctx,
			gen.UpdateObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nc-1",
				Body: &gen.ObservabilityAlertsNotificationChannel{
					Metadata: gen.ObjectMeta{Name: "nc-1"},
				},
			})
		require.NoError(t, err)
		_, ok := resp.(gen.UpdateObservabilityAlertsNotificationChannel200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.UpdateObservabilityAlertsNotificationChannel(ctx,
			gen.UpdateObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nc-1",
				Body: nil,
			})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateObservabilityAlertsNotificationChannel400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.UpdateObservabilityAlertsNotificationChannel(ctx,
			gen.UpdateObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nonexistent",
				Body: &gen.ObservabilityAlertsNotificationChannel{
					Metadata: gen.ObjectMeta{Name: "nonexistent"},
				},
			})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateObservabilityAlertsNotificationChannel404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testObservabilityAlertsNotificationChannelObj("nc-1")}
		svc := newObservabilityAlertsNotificationChannelService(t, objs, &denyAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.UpdateObservabilityAlertsNotificationChannel(ctx,
			gen.UpdateObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nc-1",
				Body: &gen.ObservabilityAlertsNotificationChannel{
					Metadata: gen.ObjectMeta{Name: "nc-1"},
				},
			})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateObservabilityAlertsNotificationChannel403JSONResponse{}, resp)
	})
}

// --- DeleteObservabilityAlertsNotificationChannel Handler ---

func TestDeleteObservabilityAlertsNotificationChannelHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testObservabilityAlertsNotificationChannelObj("nc-1")}
		svc := newObservabilityAlertsNotificationChannelService(t, objs, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.DeleteObservabilityAlertsNotificationChannel(ctx,
			gen.DeleteObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nc-1",
			})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteObservabilityAlertsNotificationChannel204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newObservabilityAlertsNotificationChannelService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.DeleteObservabilityAlertsNotificationChannel(ctx,
			gen.DeleteObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nonexistent",
			})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteObservabilityAlertsNotificationChannel404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testObservabilityAlertsNotificationChannelObj("nc-1")}
		svc := newObservabilityAlertsNotificationChannelService(t, objs, &denyAllPDP{})
		h := newHandlerWithObservabilityAlertsNotificationChannelService(svc)

		resp, err := h.DeleteObservabilityAlertsNotificationChannel(ctx,
			gen.DeleteObservabilityAlertsNotificationChannelRequestObject{
				NamespaceName: ns,
				ObservabilityAlertsNotificationChannelName: "nc-1",
			})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteObservabilityAlertsNotificationChannel403JSONResponse{}, resp)
	})
}
