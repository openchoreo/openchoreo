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
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newEnvironmentService(t *testing.T, objects []client.Object, pdp authzcore.PDP) environmentsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return environmentsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithEnvironmentService(svc environmentsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{EnvironmentService: svc},
		logger:   slog.Default(),
	}
}

func testEnvObj(name string) *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: controller.DefaultPlaneName,
			},
		},
	}
}

func testDataPlaneObj() *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.DefaultPlaneName,
			Namespace: "test-ns",
		},
	}
}

// --- ListEnvironments Handler ---

func TestListEnvironmentsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("env-1")}, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.ListEnvironments(ctx, gen.ListEnvironmentsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEnvironments200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "env-1", typed.Items[0].Metadata.Name)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.ListEnvironments(ctx, gen.ListEnvironmentsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListEnvironmentsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListEnvironments400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.ListEnvironments(ctx, gen.ListEnvironmentsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEnvironments200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("env-1")}, &denyAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.ListEnvironments(ctx, gen.ListEnvironmentsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEnvironments200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("partial authorization filters only denied items", func(t *testing.T) {
		pdp := &selectivePDP{allowedIDs: map[string]bool{"env-allowed": true}}
		svc := newEnvironmentService(t, []client.Object{
			testEnvObj("env-allowed"),
			testEnvObj("env-denied"),
		}, pdp)
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.ListEnvironments(ctx, gen.ListEnvironmentsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEnvironments200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "env-allowed", typed.Items[0].Metadata.Name)
	})
}

// --- GetEnvironment Handler ---

func TestGetEnvironmentHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("dev")}, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.GetEnvironment(ctx, gen.GetEnvironmentRequestObject{NamespaceName: ns, EnvName: "dev"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetEnvironment200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "dev", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.GetEnvironment(ctx, gen.GetEnvironmentRequestObject{NamespaceName: ns, EnvName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEnvironment404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("dev")}, &denyAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.GetEnvironment(ctx, gen.GetEnvironmentRequestObject{NamespaceName: ns, EnvName: "dev"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEnvironment403JSONResponse{}, resp)
	})
}

// --- CreateEnvironment Handler ---

func TestCreateEnvironmentHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	validBody := &gen.Environment{
		Metadata: gen.ObjectMeta{Name: "new-env"},
	}

	t.Run("success", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testDataPlaneObj()}, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.CreateEnvironment(ctx, gen.CreateEnvironmentRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateEnvironment201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-env", typed.Metadata.Name)
		require.NotNil(t, typed.Metadata.Namespace)
		assert.Equal(t, ns, *typed.Metadata.Namespace)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.CreateEnvironment(ctx, gen.CreateEnvironmentRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEnvironment400JSONResponse{}, resp)
	})

	t.Run("empty name returns 400", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.CreateEnvironment(ctx, gen.CreateEnvironmentRequestObject{
			NamespaceName: ns,
			Body:          &gen.Environment{Metadata: gen.ObjectMeta{Name: "  "}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEnvironment400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testEnvObj("new-env")
		svc := newEnvironmentService(t, []client.Object{existing, testDataPlaneObj()}, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.CreateEnvironment(ctx, gen.CreateEnvironmentRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEnvironment409JSONResponse{}, resp)
	})

	t.Run("dataplane not found returns 400", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{}) // no DataPlane seeded
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.CreateEnvironment(ctx, gen.CreateEnvironmentRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEnvironment400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testDataPlaneObj()}, &denyAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.CreateEnvironment(ctx, gen.CreateEnvironmentRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEnvironment403JSONResponse{}, resp)
	})
}

// --- UpdateEnvironment Handler ---

func TestUpdateEnvironmentHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("dev")}, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.UpdateEnvironment(ctx, gen.UpdateEnvironmentRequestObject{
			NamespaceName: ns,
			EnvName:       "dev",
			Body:          &gen.Environment{Metadata: gen.ObjectMeta{Name: "dev"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateEnvironment200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "dev", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.UpdateEnvironment(ctx, gen.UpdateEnvironmentRequestObject{
			NamespaceName: ns,
			EnvName:       "dev",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEnvironment400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.UpdateEnvironment(ctx, gen.UpdateEnvironmentRequestObject{
			NamespaceName: ns,
			EnvName:       "nonexistent",
			Body:          &gen.Environment{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEnvironment404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("dev")}, &denyAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.UpdateEnvironment(ctx, gen.UpdateEnvironmentRequestObject{
			NamespaceName: ns,
			EnvName:       "dev",
			Body:          &gen.Environment{Metadata: gen.ObjectMeta{Name: "dev"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEnvironment403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("dev")}, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		// Body has different name than URL path
		resp, err := h.UpdateEnvironment(ctx, gen.UpdateEnvironmentRequestObject{
			NamespaceName: ns,
			EnvName:       "dev",
			Body:          &gen.Environment{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		// Should succeed using the URL path name "dev", not the body name
		typed, ok := resp.(gen.UpdateEnvironment200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "dev", typed.Metadata.Name)
	})
}

// --- DeleteEnvironment Handler ---

func TestDeleteEnvironmentHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("dev")}, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.DeleteEnvironment(ctx, gen.DeleteEnvironmentRequestObject{NamespaceName: ns, EnvName: "dev"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEnvironment204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEnvironmentService(t, nil, &allowAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.DeleteEnvironment(ctx, gen.DeleteEnvironmentRequestObject{NamespaceName: ns, EnvName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEnvironment404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEnvironmentService(t, []client.Object{testEnvObj("dev")}, &denyAllPDP{})
		h := newHandlerWithEnvironmentService(svc)

		resp, err := h.DeleteEnvironment(ctx, gen.DeleteEnvironmentRequestObject{NamespaceName: ns, EnvName: "dev"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEnvironment403JSONResponse{}, resp)
	})
}
