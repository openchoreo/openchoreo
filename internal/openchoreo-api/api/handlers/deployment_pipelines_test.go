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
	deploymentpipelinesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newDeploymentPipelineService(
	t *testing.T, objects []client.Object, pdp authzcore.PDP,
) deploymentpipelinesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return deploymentpipelinesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithDeploymentPipelineService(svc deploymentpipelinesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{DeploymentPipelineService: svc},
		logger:   slog.Default(),
	}
}

func testDeploymentPipelineObj(name string) *openchoreov1alpha1.DeploymentPipeline {
	return &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
	}
}

// --- ListDeploymentPipelines Handler ---

func TestListDeploymentPipelinesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, []client.Object{testDeploymentPipelineObj("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.ListDeploymentPipelines(ctx, gen.ListDeploymentPipelinesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListDeploymentPipelines200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "dp-1", typed.Items[0].Metadata.Name)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.ListDeploymentPipelines(ctx, gen.ListDeploymentPipelinesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListDeploymentPipelinesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListDeploymentPipelines400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.ListDeploymentPipelines(ctx, gen.ListDeploymentPipelinesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListDeploymentPipelines200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetDeploymentPipeline Handler ---

func TestGetDeploymentPipelineHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, []client.Object{testDeploymentPipelineObj("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.GetDeploymentPipeline(ctx, gen.GetDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "dp-1",
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetDeploymentPipeline200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "dp-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.GetDeploymentPipeline(ctx, gen.GetDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetDeploymentPipeline404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, []client.Object{testDeploymentPipelineObj("dp-1")}, &denyAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.GetDeploymentPipeline(ctx, gen.GetDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "dp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetDeploymentPipeline403JSONResponse{}, resp)
	})
}

// --- CreateDeploymentPipeline Handler ---

func TestCreateDeploymentPipelineHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	validBody := &gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "new-dp"},
	}

	t.Run("success", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.CreateDeploymentPipeline(ctx, gen.CreateDeploymentPipelineRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateDeploymentPipeline201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-dp", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.CreateDeploymentPipeline(ctx, gen.CreateDeploymentPipelineRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateDeploymentPipeline400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testDeploymentPipelineObj("new-dp")
		svc := newDeploymentPipelineService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.CreateDeploymentPipeline(ctx, gen.CreateDeploymentPipelineRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateDeploymentPipeline409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &denyAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.CreateDeploymentPipeline(ctx, gen.CreateDeploymentPipelineRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateDeploymentPipeline403JSONResponse{}, resp)
	})
}

// --- UpdateDeploymentPipeline Handler ---

func TestUpdateDeploymentPipelineHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, []client.Object{testDeploymentPipelineObj("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.UpdateDeploymentPipeline(ctx, gen.UpdateDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "dp-1",
			Body:                   &gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: "dp-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateDeploymentPipeline200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "dp-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.UpdateDeploymentPipeline(ctx, gen.UpdateDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "dp-1",
			Body:                   nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateDeploymentPipeline400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.UpdateDeploymentPipeline(ctx, gen.UpdateDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "nonexistent",
			Body:                   &gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateDeploymentPipeline404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, []client.Object{testDeploymentPipelineObj("dp-1")}, &denyAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.UpdateDeploymentPipeline(ctx, gen.UpdateDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "dp-1",
			Body:                   &gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: "dp-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateDeploymentPipeline403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, []client.Object{testDeploymentPipelineObj("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.UpdateDeploymentPipeline(ctx, gen.UpdateDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "dp-1",
			Body:                   &gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateDeploymentPipeline200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "dp-1", typed.Metadata.Name)
	})
}

// --- DeleteDeploymentPipeline Handler ---

func TestDeleteDeploymentPipelineHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, []client.Object{testDeploymentPipelineObj("dp-1")}, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.DeleteDeploymentPipeline(ctx, gen.DeleteDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "dp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteDeploymentPipeline204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, nil, &allowAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.DeleteDeploymentPipeline(ctx, gen.DeleteDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteDeploymentPipeline404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newDeploymentPipelineService(t, []client.Object{testDeploymentPipelineObj("dp-1")}, &denyAllPDP{})
		h := newHandlerWithDeploymentPipelineService(svc)

		resp, err := h.DeleteDeploymentPipeline(ctx, gen.DeleteDeploymentPipelineRequestObject{
			NamespaceName:          ns,
			DeploymentPipelineName: "dp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteDeploymentPipeline403JSONResponse{}, resp)
	})
}
