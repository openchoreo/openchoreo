// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	clusterworkflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflowplane"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newClusterWorkflowPlaneService(
	t *testing.T, objects []client.Object, pdp authzcore.PDP,
) clusterworkflowplanesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clusterworkflowplanesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithClusterWorkflowPlaneService(svc clusterworkflowplanesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterWorkflowPlaneService: svc},
		logger:   slog.Default(),
	}
}

func testClusterWorkflowPlaneObj(name string) *openchoreov1alpha1.ClusterWorkflowPlane {
	return &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID: name,
		},
	}
}

// --- ListClusterWorkflowPlanes Handler ---

func TestListClusterWorkflowPlanesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, []client.Object{testClusterWorkflowPlaneObj("cwp-1")}, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.ListClusterWorkflowPlanes(ctx, gen.ListClusterWorkflowPlanesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterWorkflowPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.ListClusterWorkflowPlanes(ctx, gen.ListClusterWorkflowPlanesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterWorkflowPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetClusterWorkflowPlane Handler ---

func TestGetClusterWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.GetClusterWorkflowPlane(ctx, gen.GetClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetClusterWorkflowPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.GetClusterWorkflowPlane(ctx, gen.GetClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.GetClusterWorkflowPlane(ctx, gen.GetClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflowPlane403JSONResponse{}, resp)
	})
}

// --- CreateClusterWorkflowPlane Handler ---

func TestCreateClusterWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{
			Body: &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "new-cwp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflowPlane201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflowPlane400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("new-cwp")}
		svc := newClusterWorkflowPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{
			Body: &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "new-cwp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflowPlane409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{
			Body: &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "new-cwp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflowPlane403JSONResponse{}, resp)
	})
}

// --- UpdateClusterWorkflowPlane Handler ---

func TestUpdateClusterWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
			Body:                     &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "cwp-1"}},
		})
		require.NoError(t, err)
		_, ok := resp.(gen.UpdateClusterWorkflowPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
			Body:                     nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterWorkflowPlane400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "nonexistent",
			Body:                     &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
			Body:                     &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "cwp-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterWorkflowPlane403JSONResponse{}, resp)
	})
}

// --- DeleteClusterWorkflowPlane Handler ---

func TestDeleteClusterWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.DeleteClusterWorkflowPlane(ctx, gen.DeleteClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflowPlane204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.DeleteClusterWorkflowPlane(ctx, gen.DeleteClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.DeleteClusterWorkflowPlane(ctx, gen.DeleteClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflowPlane403JSONResponse{}, resp)
	})
}
