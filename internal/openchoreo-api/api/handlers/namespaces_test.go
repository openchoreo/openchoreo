// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	namespacesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace"
)

func newTestSchemeWithCoreV1(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return scheme
}

func newNamespaceService(t *testing.T, objects []client.Object, pdp authzcore.PDP) namespacesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestSchemeWithCoreV1(t)).
		WithObjects(objects...).
		Build()
	return namespacesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithNamespaceService(svc namespacesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{NamespaceService: svc},
		logger:   slog.Default(),
	}
}

func testNamespaceObj(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"openchoreo.dev/control-plane": "true",
			},
		},
	}
}

// --- ListNamespaces Handler ---

func TestListNamespacesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.ListNamespaces(ctx, gen.ListNamespacesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListNamespaces200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "ns-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.ListNamespaces(ctx, gen.ListNamespacesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListNamespaces200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.ListNamespaces(ctx, gen.ListNamespacesRequestObject{
			Params: gen.ListNamespacesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListNamespaces400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &denyAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.ListNamespaces(ctx, gen.ListNamespacesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListNamespaces200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetNamespace Handler ---

func TestGetNamespaceHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.GetNamespace(ctx, gen.GetNamespaceRequestObject{NamespaceName: "ns-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetNamespace200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "ns-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.GetNamespace(ctx, gen.GetNamespaceRequestObject{NamespaceName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetNamespace404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &denyAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.GetNamespace(ctx, gen.GetNamespaceRequestObject{NamespaceName: "ns-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetNamespace403JSONResponse{}, resp)
	})
}

// --- CreateNamespace Handler ---

func TestCreateNamespaceHandler(t *testing.T) {
	ctx := testContext()

	validBody := &gen.Namespace{
		Metadata: gen.ObjectMeta{Name: "new-ns"},
	}

	t.Run("success", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.CreateNamespace(ctx, gen.CreateNamespaceRequestObject{
			Body: validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateNamespace201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-ns", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.CreateNamespace(ctx, gen.CreateNamespaceRequestObject{
			Body: nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespace400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testNamespaceObj("new-ns")
		svc := newNamespaceService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.CreateNamespace(ctx, gen.CreateNamespaceRequestObject{
			Body: validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespace409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &denyAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.CreateNamespace(ctx, gen.CreateNamespaceRequestObject{
			Body: validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespace403JSONResponse{}, resp)
	})
}

// --- UpdateNamespace Handler ---

func TestUpdateNamespaceHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.UpdateNamespace(ctx, gen.UpdateNamespaceRequestObject{
			NamespaceName: "ns-1",
			Body:          &gen.Namespace{Metadata: gen.ObjectMeta{Name: "ns-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateNamespace200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "ns-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.UpdateNamespace(ctx, gen.UpdateNamespaceRequestObject{
			NamespaceName: "ns-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespace400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.UpdateNamespace(ctx, gen.UpdateNamespaceRequestObject{
			NamespaceName: "nonexistent",
			Body:          &gen.Namespace{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespace404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &denyAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.UpdateNamespace(ctx, gen.UpdateNamespaceRequestObject{
			NamespaceName: "ns-1",
			Body:          &gen.Namespace{Metadata: gen.ObjectMeta{Name: "ns-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespace403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		// Body has different name than URL path
		resp, err := h.UpdateNamespace(ctx, gen.UpdateNamespaceRequestObject{
			NamespaceName: "ns-1",
			Body:          &gen.Namespace{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		// Should succeed using the URL path name "ns-1", not the body name
		typed, ok := resp.(gen.UpdateNamespace200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "ns-1", typed.Metadata.Name)
	})
}

// --- DeleteNamespace Handler ---

func TestDeleteNamespaceHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.DeleteNamespace(ctx, gen.DeleteNamespaceRequestObject{NamespaceName: "ns-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespace204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newNamespaceService(t, nil, &allowAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.DeleteNamespace(ctx, gen.DeleteNamespaceRequestObject{NamespaceName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespace404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newNamespaceService(t, []client.Object{testNamespaceObj("ns-1")}, &denyAllPDP{})
		h := newHandlerWithNamespaceService(svc)

		resp, err := h.DeleteNamespace(ctx, gen.DeleteNamespaceRequestObject{NamespaceName: "ns-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespace403JSONResponse{}, resp)
	})
}
