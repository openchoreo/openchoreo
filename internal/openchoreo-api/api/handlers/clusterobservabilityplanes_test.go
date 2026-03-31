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
	clusterobservabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterobservabilityplane"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newClusterObservabilityPlaneService(
	t *testing.T, objects []client.Object, pdp authzcore.PDP,
) clusterobservabilityplanesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clusterobservabilityplanesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithClusterObservabilityPlaneService(svc clusterobservabilityplanesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterObservabilityPlaneService: svc},
		logger:   slog.Default(),
	}
}

func testClusterObservabilityPlaneObj(name string) *openchoreov1alpha1.ClusterObservabilityPlane {
	return &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			PlaneID: name,
		},
	}
}

// --- ListClusterObservabilityPlanes Handler ---

func TestListClusterObservabilityPlanesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		objs := []client.Object{testClusterObservabilityPlaneObj("cop-1")}
		svc := newClusterObservabilityPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.ListClusterObservabilityPlanes(ctx, gen.ListClusterObservabilityPlanesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterObservabilityPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.ListClusterObservabilityPlanes(ctx, gen.ListClusterObservabilityPlanesRequestObject{
			Params: gen.ListClusterObservabilityPlanesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListClusterObservabilityPlanes400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.ListClusterObservabilityPlanes(ctx, gen.ListClusterObservabilityPlanesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterObservabilityPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetClusterObservabilityPlane Handler ---

func TestGetClusterObservabilityPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterObservabilityPlaneObj("cop-1")}
		svc := newClusterObservabilityPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.GetClusterObservabilityPlane(ctx, gen.GetClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "cop-1",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetClusterObservabilityPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.GetClusterObservabilityPlane(ctx, gen.GetClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterObservabilityPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterObservabilityPlaneObj("cop-1")}
		svc := newClusterObservabilityPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.GetClusterObservabilityPlane(ctx, gen.GetClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "cop-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterObservabilityPlane403JSONResponse{}, resp)
	})
}

// --- CreateClusterObservabilityPlane Handler ---

func TestCreateClusterObservabilityPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.CreateClusterObservabilityPlane(ctx, gen.CreateClusterObservabilityPlaneRequestObject{
			Body: &gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "new-cop"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterObservabilityPlane201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.CreateClusterObservabilityPlane(ctx, gen.CreateClusterObservabilityPlaneRequestObject{
			Body: nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterObservabilityPlane400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		objs := []client.Object{testClusterObservabilityPlaneObj("new-cop")}
		svc := newClusterObservabilityPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.CreateClusterObservabilityPlane(ctx, gen.CreateClusterObservabilityPlaneRequestObject{
			Body: &gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "new-cop"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterObservabilityPlane409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &denyAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.CreateClusterObservabilityPlane(ctx, gen.CreateClusterObservabilityPlaneRequestObject{
			Body: &gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "new-cop"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterObservabilityPlane403JSONResponse{}, resp)
	})
}

// --- UpdateClusterObservabilityPlane Handler ---

func TestUpdateClusterObservabilityPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterObservabilityPlaneObj("cop-1")}
		svc := newClusterObservabilityPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.UpdateClusterObservabilityPlane(ctx, gen.UpdateClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "cop-1",
			Body:                          &gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "cop-1"}},
		})
		require.NoError(t, err)
		_, ok := resp.(gen.UpdateClusterObservabilityPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.UpdateClusterObservabilityPlane(ctx, gen.UpdateClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "cop-1",
			Body:                          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterObservabilityPlane400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.UpdateClusterObservabilityPlane(ctx, gen.UpdateClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "nonexistent",
			Body:                          &gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterObservabilityPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterObservabilityPlaneObj("cop-1")}
		svc := newClusterObservabilityPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.UpdateClusterObservabilityPlane(ctx, gen.UpdateClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "cop-1",
			Body:                          &gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "cop-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterObservabilityPlane403JSONResponse{}, resp)
	})
}

// --- DeleteClusterObservabilityPlane Handler ---

func TestDeleteClusterObservabilityPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterObservabilityPlaneObj("cop-1")}
		svc := newClusterObservabilityPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.DeleteClusterObservabilityPlane(ctx, gen.DeleteClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "cop-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterObservabilityPlane204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.DeleteClusterObservabilityPlane(ctx, gen.DeleteClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterObservabilityPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterObservabilityPlaneObj("cop-1")}
		svc := newClusterObservabilityPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterObservabilityPlaneService(svc)

		resp, err := h.DeleteClusterObservabilityPlane(ctx, gen.DeleteClusterObservabilityPlaneRequestObject{
			ClusterObservabilityPlaneName: "cop-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterObservabilityPlane403JSONResponse{}, resp)
	})
}
