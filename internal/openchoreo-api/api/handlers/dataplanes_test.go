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
	dataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newDataPlaneService(t *testing.T, objects []client.Object, pdp authzcore.PDP) dataplanesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return dataplanesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithDataPlaneService(svc dataplanesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{DataPlaneService: svc},
		logger:   slog.Default(),
	}
}

func testDataPlane(name string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
	}
}

// --- ListDataPlanes Handler ---

func TestListDataPlanesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.ListDataPlanes(ctx, gen.ListDataPlanesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListDataPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "dp-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.ListDataPlanes(ctx, gen.ListDataPlanesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListDataPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.ListDataPlanes(ctx, gen.ListDataPlanesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListDataPlanesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListDataPlanes400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &denyAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.ListDataPlanes(ctx, gen.ListDataPlanesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListDataPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetDataPlane Handler ---

func TestGetDataPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.GetDataPlane(ctx, gen.GetDataPlaneRequestObject{NamespaceName: ns, DpName: "dp-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetDataPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "dp-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.GetDataPlane(ctx, gen.GetDataPlaneRequestObject{NamespaceName: ns, DpName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetDataPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &denyAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.GetDataPlane(ctx, gen.GetDataPlaneRequestObject{NamespaceName: ns, DpName: "dp-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetDataPlane403JSONResponse{}, resp)
	})
}

// --- CreateDataPlane Handler ---

func TestCreateDataPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	validBody := &gen.DataPlane{
		Metadata: gen.ObjectMeta{Name: "new-dp"},
	}

	t.Run("success", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.CreateDataPlane(ctx, gen.CreateDataPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateDataPlane201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-dp", typed.Metadata.Name)
		require.NotNil(t, typed.Metadata.Namespace)
		assert.Equal(t, ns, *typed.Metadata.Namespace)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.CreateDataPlane(ctx, gen.CreateDataPlaneRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateDataPlane400JSONResponse{}, resp)
	})

	t.Run("empty name returns 400", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.CreateDataPlane(ctx, gen.CreateDataPlaneRequestObject{
			NamespaceName: ns,
			Body:          &gen.DataPlane{Metadata: gen.ObjectMeta{Name: "  "}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateDataPlane400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testDataPlane("new-dp")
		svc := newDataPlaneService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.CreateDataPlane(ctx, gen.CreateDataPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateDataPlane409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &denyAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.CreateDataPlane(ctx, gen.CreateDataPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateDataPlane403JSONResponse{}, resp)
	})
}

// --- UpdateDataPlane Handler ---

func TestUpdateDataPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.UpdateDataPlane(ctx, gen.UpdateDataPlaneRequestObject{
			NamespaceName: ns,
			DpName:        "dp-1",
			Body:          &gen.DataPlane{Metadata: gen.ObjectMeta{Name: "dp-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateDataPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "dp-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.UpdateDataPlane(ctx, gen.UpdateDataPlaneRequestObject{
			NamespaceName: ns,
			DpName:        "dp-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateDataPlane400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.UpdateDataPlane(ctx, gen.UpdateDataPlaneRequestObject{
			NamespaceName: ns,
			DpName:        "nonexistent",
			Body:          &gen.DataPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateDataPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &denyAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.UpdateDataPlane(ctx, gen.UpdateDataPlaneRequestObject{
			NamespaceName: ns,
			DpName:        "dp-1",
			Body:          &gen.DataPlane{Metadata: gen.ObjectMeta{Name: "dp-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateDataPlane403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		// Body has different name than URL path
		resp, err := h.UpdateDataPlane(ctx, gen.UpdateDataPlaneRequestObject{
			NamespaceName: ns,
			DpName:        "dp-1",
			Body:          &gen.DataPlane{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		// Should succeed using the URL path name "dp-1", not the body name
		typed, ok := resp.(gen.UpdateDataPlane200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "dp-1", typed.Metadata.Name)
	})
}

// --- DeleteDataPlane Handler ---

func TestDeleteDataPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.DeleteDataPlane(ctx, gen.DeleteDataPlaneRequestObject{NamespaceName: ns, DpName: "dp-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteDataPlane204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newDataPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.DeleteDataPlane(ctx, gen.DeleteDataPlaneRequestObject{NamespaceName: ns, DpName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteDataPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newDataPlaneService(t, []client.Object{testDataPlane("dp-1")}, &denyAllPDP{})
		h := newHandlerWithDataPlaneService(svc)

		resp, err := h.DeleteDataPlane(ctx, gen.DeleteDataPlaneRequestObject{NamespaceName: ns, DpName: "dp-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteDataPlane403JSONResponse{}, resp)
	})
}
