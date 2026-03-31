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
	clusterdataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterdataplane"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newClusterDataPlaneService(
	t *testing.T, objects []client.Object, pdp authzcore.PDP,
) clusterdataplanesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clusterdataplanesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithClusterDataPlaneService(svc clusterdataplanesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterDataPlaneService: svc},
		logger:   slog.Default(),
	}
}

func testClusterDataPlaneObj(name string) *openchoreov1alpha1.ClusterDataPlane {
	return &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: name,
		},
	}
}

// --- ListClusterDataPlanes Handler ---

func TestListClusterDataPlanesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, []client.Object{testClusterDataPlaneObj("cdp-1")}, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.ListClusterDataPlanes(ctx, gen.ListClusterDataPlanesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterDataPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.ListClusterDataPlanes(ctx, gen.ListClusterDataPlanesRequestObject{
			Params: gen.ListClusterDataPlanesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListClusterDataPlanes400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.ListClusterDataPlanes(ctx, gen.ListClusterDataPlanesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterDataPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetClusterDataPlane Handler ---

func TestGetClusterDataPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, []client.Object{testClusterDataPlaneObj("cdp-1")}, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.GetClusterDataPlane(ctx, gen.GetClusterDataPlaneRequestObject{CdpName: "cdp-1"})
		require.NoError(t, err)
		_, ok := resp.(gen.GetClusterDataPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.GetClusterDataPlane(ctx, gen.GetClusterDataPlaneRequestObject{CdpName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterDataPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, []client.Object{testClusterDataPlaneObj("cdp-1")}, &denyAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.GetClusterDataPlane(ctx, gen.GetClusterDataPlaneRequestObject{CdpName: "cdp-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterDataPlane403JSONResponse{}, resp)
	})
}

// --- CreateClusterDataPlane Handler ---

func TestCreateClusterDataPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.CreateClusterDataPlane(ctx, gen.CreateClusterDataPlaneRequestObject{
			Body: &gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "new-cdp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterDataPlane201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.CreateClusterDataPlane(ctx, gen.CreateClusterDataPlaneRequestObject{Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterDataPlane400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, []client.Object{testClusterDataPlaneObj("new-cdp")}, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.CreateClusterDataPlane(ctx, gen.CreateClusterDataPlaneRequestObject{
			Body: &gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "new-cdp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterDataPlane409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &denyAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.CreateClusterDataPlane(ctx, gen.CreateClusterDataPlaneRequestObject{
			Body: &gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "new-cdp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterDataPlane403JSONResponse{}, resp)
	})
}

// --- UpdateClusterDataPlane Handler ---

func TestUpdateClusterDataPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, []client.Object{testClusterDataPlaneObj("cdp-1")}, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.UpdateClusterDataPlane(ctx, gen.UpdateClusterDataPlaneRequestObject{
			CdpName: "cdp-1",
			Body:    &gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "cdp-1"}},
		})
		require.NoError(t, err)
		_, ok := resp.(gen.UpdateClusterDataPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.UpdateClusterDataPlane(ctx, gen.UpdateClusterDataPlaneRequestObject{
			CdpName: "cdp-1",
			Body:    nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterDataPlane400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.UpdateClusterDataPlane(ctx, gen.UpdateClusterDataPlaneRequestObject{
			CdpName: "nonexistent",
			Body:    &gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterDataPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, []client.Object{testClusterDataPlaneObj("cdp-1")}, &denyAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.UpdateClusterDataPlane(ctx, gen.UpdateClusterDataPlaneRequestObject{
			CdpName: "cdp-1",
			Body:    &gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "cdp-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterDataPlane403JSONResponse{}, resp)
	})
}

// --- DeleteClusterDataPlane Handler ---

func TestDeleteClusterDataPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, []client.Object{testClusterDataPlaneObj("cdp-1")}, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.DeleteClusterDataPlane(ctx, gen.DeleteClusterDataPlaneRequestObject{CdpName: "cdp-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterDataPlane204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.DeleteClusterDataPlane(ctx, gen.DeleteClusterDataPlaneRequestObject{CdpName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterDataPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterDataPlaneService(t, []client.Object{testClusterDataPlaneObj("cdp-1")}, &denyAllPDP{})
		h := newHandlerWithClusterDataPlaneService(svc)

		resp, err := h.DeleteClusterDataPlane(ctx, gen.DeleteClusterDataPlaneRequestObject{CdpName: "cdp-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterDataPlane403JSONResponse{}, resp)
	})
}
