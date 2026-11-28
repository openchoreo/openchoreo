// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	k8sresourcessvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
	k8sresourcesmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources/mocks"
)

func endpointK8sHandler(svc *k8sresourcesmocks.MockService) *Handler {
	return &Handler{
		services: &handlerservices.Services{K8sResourcesService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestGetEndpointReleaseBindingK8sResourceTreeHandler(t *testing.T) {
	ctx := testContext()

	t.Run("returns the tree of the endpoint binding", func(t *testing.T) {
		svc := k8sresourcesmocks.NewMockService(t)
		svc.EXPECT().GetEndpointResourceTree(mock.Anything, "test-ns", "ep-1-dev").Return(&k8sresourcessvc.K8sResourceTreeResult{
			RenderedReleases: []k8sresourcessvc.ReleaseResourceTree{
				{Name: "ep-1-dev", TargetPlane: "dataplane", Nodes: []models.ResourceNode{{Kind: "HTTPRoute", Name: "ep-1"}}},
			},
		}, nil)

		resp, err := endpointK8sHandler(svc).GetEndpointReleaseBindingK8sResourceTree(ctx,
			gen.GetEndpointReleaseBindingK8sResourceTreeRequestObject{NamespaceName: "test-ns", EndpointReleaseBindingName: "ep-1-dev"})
		require.NoError(t, err)
		ok, isOK := resp.(gen.GetEndpointReleaseBindingK8sResourceTree200JSONResponse)
		require.True(t, isOK, "got %T", resp)
		require.Len(t, ok.RenderedReleases, 1)
		assert.Equal(t, "HTTPRoute", ok.RenderedReleases[0].Nodes[0].Kind)
	})

	for _, tt := range []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetEndpointReleaseBindingK8sResourceTree403JSONResponse{}},
		{"not found -> 404", k8sresourcessvc.ErrEndpointReleaseBindingNotFound, gen.GetEndpointReleaseBindingK8sResourceTree404JSONResponse{}},
		{"environment not found -> 404", k8sresourcessvc.ErrEnvironmentNotFound, gen.GetEndpointReleaseBindingK8sResourceTree404JSONResponse{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := k8sresourcesmocks.NewMockService(t)
			svc.EXPECT().GetEndpointResourceTree(mock.Anything, mock.Anything, mock.Anything).Return(nil, tt.svcErr)

			resp, err := endpointK8sHandler(svc).GetEndpointReleaseBindingK8sResourceTree(ctx,
				gen.GetEndpointReleaseBindingK8sResourceTreeRequestObject{NamespaceName: "test-ns", EndpointReleaseBindingName: "ep-1-dev"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestGetEndpointReleaseBindingK8sResourceEventsHandler(t *testing.T) {
	ctx := testContext()

	t.Run("passes the resource identity through", func(t *testing.T) {
		svc := k8sresourcesmocks.NewMockService(t)
		group := "gateway.networking.k8s.io"
		svc.EXPECT().GetEndpointResourceEvents(mock.Anything, "test-ns", "ep-1-dev", group, "v1", "HTTPRoute", "ep-1").
			Return(&models.ResourceEventsResponse{Events: []models.ResourceEvent{{Reason: "Accepted"}}}, nil)

		resp, err := endpointK8sHandler(svc).GetEndpointReleaseBindingK8sResourceEvents(ctx,
			gen.GetEndpointReleaseBindingK8sResourceEventsRequestObject{
				NamespaceName: "test-ns", EndpointReleaseBindingName: "ep-1-dev",
				Params: gen.GetEndpointReleaseBindingK8sResourceEventsParams{Group: &group, Version: "v1", Kind: "HTTPRoute", Name: "ep-1"},
			})
		require.NoError(t, err)
		ok, isOK := resp.(gen.GetEndpointReleaseBindingK8sResourceEvents200JSONResponse)
		require.True(t, isOK, "got %T", resp)
		require.Len(t, ok.Events, 1)
	})

	t.Run("a missing resource is a 404", func(t *testing.T) {
		svc := k8sresourcesmocks.NewMockService(t)
		svc.EXPECT().GetEndpointResourceEvents(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, k8sresourcessvc.ErrResourceNotFound)

		resp, err := endpointK8sHandler(svc).GetEndpointReleaseBindingK8sResourceEvents(ctx,
			gen.GetEndpointReleaseBindingK8sResourceEventsRequestObject{
				NamespaceName: "test-ns", EndpointReleaseBindingName: "ep-1-dev",
				Params: gen.GetEndpointReleaseBindingK8sResourceEventsParams{Version: "v1", Kind: "Service", Name: "x"},
			})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointReleaseBindingK8sResourceEvents404JSONResponse{}, resp)
	})
}
