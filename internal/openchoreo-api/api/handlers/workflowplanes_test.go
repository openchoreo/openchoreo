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
	workflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowplane"
)

func newWorkflowPlaneService(t *testing.T, objects []client.Object, pdp authzcore.PDP) workflowplanesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return workflowplanesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithWorkflowPlaneService(svc workflowplanesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{WorkflowPlaneService: svc},
		logger:   slog.Default(),
	}
}

func testWorkflowPlaneObj(name string) *openchoreov1alpha1.WorkflowPlane {
	return &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
	}
}

// --- ListWorkflowPlanes Handler ---

func TestListWorkflowPlanesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, []client.Object{testWorkflowPlaneObj("wp-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.ListWorkflowPlanes(ctx, gen.ListWorkflowPlanesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListWorkflowPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "wp-1", typed.Items[0].Metadata.Name)
	})

	t.Run("invalid label selector still returns 200", func(t *testing.T) {
		// NOTE: ListWorkflowPlanes does not validate label selectors before calling the service,
		// so an invalid selector is silently ignored and an empty list is returned.
		svc := newWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.ListWorkflowPlanes(ctx, gen.ListWorkflowPlanesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListWorkflowPlanesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		_, ok := resp.(gen.ListWorkflowPlanes200JSONResponse)
		assert.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.ListWorkflowPlanes(ctx, gen.ListWorkflowPlanesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListWorkflowPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetWorkflowPlane Handler ---

func TestGetWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, []client.Object{testWorkflowPlaneObj("wp-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.GetWorkflowPlane(ctx, gen.GetWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "wp-1",
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetWorkflowPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "wp-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.GetWorkflowPlane(ctx, gen.GetWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, []client.Object{testWorkflowPlaneObj("wp-1")}, &denyAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.GetWorkflowPlane(ctx, gen.GetWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "wp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflowPlane403JSONResponse{}, resp)
	})
}

// --- CreateWorkflowPlane Handler ---

func TestCreateWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	validBody := &gen.WorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "new-wp"},
	}

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.CreateWorkflowPlane(ctx, gen.CreateWorkflowPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateWorkflowPlane201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-wp", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.CreateWorkflowPlane(ctx, gen.CreateWorkflowPlaneRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflowPlane400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testWorkflowPlaneObj("new-wp")
		svc := newWorkflowPlaneService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.CreateWorkflowPlane(ctx, gen.CreateWorkflowPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflowPlane409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, nil, &denyAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.CreateWorkflowPlane(ctx, gen.CreateWorkflowPlaneRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflowPlane403JSONResponse{}, resp)
	})
}

// --- UpdateWorkflowPlane Handler ---

func TestUpdateWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, []client.Object{testWorkflowPlaneObj("wp-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.UpdateWorkflowPlane(ctx, gen.UpdateWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "wp-1",
			Body:              &gen.WorkflowPlane{Metadata: gen.ObjectMeta{Name: "wp-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateWorkflowPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "wp-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.UpdateWorkflowPlane(ctx, gen.UpdateWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "wp-1",
			Body:              nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateWorkflowPlane400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.UpdateWorkflowPlane(ctx, gen.UpdateWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "nonexistent",
			Body:              &gen.WorkflowPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, []client.Object{testWorkflowPlaneObj("wp-1")}, &denyAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.UpdateWorkflowPlane(ctx, gen.UpdateWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "wp-1",
			Body:              &gen.WorkflowPlane{Metadata: gen.ObjectMeta{Name: "wp-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateWorkflowPlane403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, []client.Object{testWorkflowPlaneObj("wp-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.UpdateWorkflowPlane(ctx, gen.UpdateWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "wp-1",
			Body:              &gen.WorkflowPlane{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateWorkflowPlane200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "wp-1", typed.Metadata.Name)
	})
}

// --- DeleteWorkflowPlane Handler ---

func TestDeleteWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, []client.Object{testWorkflowPlaneObj("wp-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.DeleteWorkflowPlane(ctx, gen.DeleteWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "wp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflowPlane204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.DeleteWorkflowPlane(ctx, gen.DeleteWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowPlaneService(t, []client.Object{testWorkflowPlaneObj("wp-1")}, &denyAllPDP{})
		h := newHandlerWithWorkflowPlaneService(svc)

		resp, err := h.DeleteWorkflowPlane(ctx, gen.DeleteWorkflowPlaneRequestObject{
			NamespaceName:     ns,
			WorkflowPlaneName: "wp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflowPlane403JSONResponse{}, resp)
	})
}
