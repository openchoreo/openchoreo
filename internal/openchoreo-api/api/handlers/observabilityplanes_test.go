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
	observabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane"
)

func newObservabilityPlaneService(t *testing.T, objects []client.Object, pdp authzcore.PDP) observabilityplanesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return observabilityplanesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithObservabilityPlaneService(svc observabilityplanesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ObservabilityPlaneService: svc},
		logger:   slog.Default(),
	}
}

func testObservabilityPlaneObj(name string) *openchoreov1alpha1.ObservabilityPlane {
	return &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
	}
}

// --- ListObservabilityPlanes Handler ---

func TestListObservabilityPlanesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, []client.Object{testObservabilityPlaneObj("op-1")}, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.ListObservabilityPlanes(ctx, gen.ListObservabilityPlanesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListObservabilityPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "op-1", typed.Items[0].Metadata.Name)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.ListObservabilityPlanes(ctx, gen.ListObservabilityPlanesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListObservabilityPlanesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListObservabilityPlanes400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.ListObservabilityPlanes(ctx, gen.ListObservabilityPlanesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListObservabilityPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetObservabilityPlane Handler ---

func TestGetObservabilityPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, []client.Object{testObservabilityPlaneObj("op-1")}, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.GetObservabilityPlane(ctx, gen.GetObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "op-1",
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetObservabilityPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "op-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.GetObservabilityPlane(ctx, gen.GetObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetObservabilityPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, []client.Object{testObservabilityPlaneObj("op-1")}, &denyAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.GetObservabilityPlane(ctx, gen.GetObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "op-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetObservabilityPlane403JSONResponse{}, resp)
	})
}

// --- CreateObservabilityPlane Handler ---

func TestCreateObservabilityPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	validBody := &gen.ObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "new-op"},
	}

	t.Run("success", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.CreateObservabilityPlane(ctx, gen.CreateObservabilityPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateObservabilityPlane201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-op", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.CreateObservabilityPlane(ctx, gen.CreateObservabilityPlaneRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateObservabilityPlane400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testObservabilityPlaneObj("new-op")
		svc := newObservabilityPlaneService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.CreateObservabilityPlane(ctx, gen.CreateObservabilityPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateObservabilityPlane409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &denyAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.CreateObservabilityPlane(ctx, gen.CreateObservabilityPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateObservabilityPlane403JSONResponse{}, resp)
	})
}

// --- UpdateObservabilityPlane Handler ---

func TestUpdateObservabilityPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, []client.Object{testObservabilityPlaneObj("op-1")}, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.UpdateObservabilityPlane(ctx, gen.UpdateObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "op-1",
			Body:                   &gen.ObservabilityPlane{Metadata: gen.ObjectMeta{Name: "op-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateObservabilityPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "op-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.UpdateObservabilityPlane(ctx, gen.UpdateObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "op-1",
			Body:                   nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateObservabilityPlane400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.UpdateObservabilityPlane(ctx, gen.UpdateObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "nonexistent",
			Body:                   &gen.ObservabilityPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateObservabilityPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, []client.Object{testObservabilityPlaneObj("op-1")}, &denyAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.UpdateObservabilityPlane(ctx, gen.UpdateObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "op-1",
			Body:                   &gen.ObservabilityPlane{Metadata: gen.ObjectMeta{Name: "op-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateObservabilityPlane403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, []client.Object{testObservabilityPlaneObj("op-1")}, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.UpdateObservabilityPlane(ctx, gen.UpdateObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "op-1",
			Body:                   &gen.ObservabilityPlane{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateObservabilityPlane200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "op-1", typed.Metadata.Name)
	})
}

// --- DeleteObservabilityPlane Handler ---

func TestDeleteObservabilityPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, []client.Object{testObservabilityPlaneObj("op-1")}, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.DeleteObservabilityPlane(ctx, gen.DeleteObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "op-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteObservabilityPlane204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.DeleteObservabilityPlane(ctx, gen.DeleteObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteObservabilityPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newObservabilityPlaneService(t, []client.Object{testObservabilityPlaneObj("op-1")}, &denyAllPDP{})
		h := newHandlerWithObservabilityPlaneService(svc)

		resp, err := h.DeleteObservabilityPlane(ctx, gen.DeleteObservabilityPlaneRequestObject{
			NamespaceName:          ns,
			ObservabilityPlaneName: "op-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteObservabilityPlane403JSONResponse{}, resp)
	})
}
