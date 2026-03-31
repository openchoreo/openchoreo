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
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newComponentReleaseService(t *testing.T, objects []client.Object, pdp authzcore.PDP) componentreleasesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return componentreleasesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithComponentReleaseService(svc componentreleasesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ComponentReleaseService: svc},
		logger:   slog.Default(),
	}
}

func testComponentReleaseObj(name string) *openchoreov1alpha1.ComponentRelease {
	return &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
	}
}

// --- ListComponentReleases Handler ---

func TestListComponentReleasesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newComponentReleaseService(t, []client.Object{testComponentReleaseObj("cr-1")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.ListComponentReleases(ctx, gen.ListComponentReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListComponentReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newComponentReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.ListComponentReleases(ctx, gen.ListComponentReleasesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListComponentReleasesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListComponentReleases400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newComponentReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.ListComponentReleases(ctx, gen.ListComponentReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListComponentReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetComponentRelease Handler ---

func TestGetComponentReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentReleaseService(t, []client.Object{testComponentReleaseObj("cr-1")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.GetComponentRelease(ctx, gen.GetComponentReleaseRequestObject{
			NamespaceName: ns, ComponentReleaseName: "cr-1",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetComponentRelease200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.GetComponentRelease(ctx, gen.GetComponentReleaseRequestObject{
			NamespaceName: ns, ComponentReleaseName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetComponentRelease404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentReleaseService(t, []client.Object{testComponentReleaseObj("cr-1")}, &denyAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.GetComponentRelease(ctx, gen.GetComponentReleaseRequestObject{
			NamespaceName: ns, ComponentReleaseName: "cr-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetComponentRelease403JSONResponse{}, resp)
	})
}

// --- CreateComponentRelease Handler ---

func TestCreateComponentReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.CreateComponentRelease(ctx, gen.CreateComponentReleaseRequestObject{
			NamespaceName: ns,
			Body:          &gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "new-cr"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentRelease201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newComponentReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.CreateComponentRelease(ctx, gen.CreateComponentReleaseRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentRelease400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		svc := newComponentReleaseService(t, []client.Object{testComponentReleaseObj("new-cr")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.CreateComponentRelease(ctx, gen.CreateComponentReleaseRequestObject{
			NamespaceName: ns,
			Body:          &gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "new-cr"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentRelease409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentReleaseService(t, nil, &denyAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.CreateComponentRelease(ctx, gen.CreateComponentReleaseRequestObject{
			NamespaceName: ns,
			Body:          &gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "new-cr"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentRelease403JSONResponse{}, resp)
	})
}

// --- DeleteComponentRelease Handler ---

func TestDeleteComponentReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentReleaseService(t, []client.Object{testComponentReleaseObj("cr-1")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.DeleteComponentRelease(ctx, gen.DeleteComponentReleaseRequestObject{
			NamespaceName: ns, ComponentReleaseName: "cr-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentRelease204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.DeleteComponentRelease(ctx, gen.DeleteComponentReleaseRequestObject{
			NamespaceName: ns, ComponentReleaseName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentRelease404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentReleaseService(t, []client.Object{testComponentReleaseObj("cr-1")}, &denyAllPDP{})
		h := newHandlerWithComponentReleaseService(svc)

		resp, err := h.DeleteComponentRelease(ctx, gen.DeleteComponentReleaseRequestObject{
			NamespaceName: ns, ComponentReleaseName: "cr-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentRelease403JSONResponse{}, resp)
	})
}
