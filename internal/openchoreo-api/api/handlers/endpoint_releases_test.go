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
	endpointreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrelease"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newEndpointReleaseService(t *testing.T, objects []client.Object, pdp authzcore.PDP) endpointreleasesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return endpointreleasesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithEndpointReleaseService(svc endpointreleasesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{EndpointReleaseService: svc},
		logger:   slog.Default(),
	}
}

func testEndpointReleaseObj() *openchoreov1alpha1.EndpointRelease {
	return &openchoreov1alpha1.EndpointRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rr-1",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.EndpointReleaseSpec{
			Owner: openchoreov1alpha1.EndpointReleaseOwner{
				ProjectName:  "test-project",
				EndpointName: "test-r",
			},
		},
	}
}

// --- ListEndpointReleases Handler ---

func TestListEndpointReleasesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newEndpointReleaseService(t, []client.Object{testEndpointReleaseObj()}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.ListEndpointReleases(ctx, gen.ListEndpointReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("filtered by resource", func(t *testing.T) {
		svc := newEndpointReleaseService(t, []client.Object{testEndpointReleaseObj()}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.ListEndpointReleases(ctx, gen.ListEndpointReleasesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListEndpointReleasesParams{Endpoint: ptr.To("test-r")},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newEndpointReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.ListEndpointReleases(ctx, gen.ListEndpointReleasesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListEndpointReleasesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListEndpointReleases400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newEndpointReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.ListEndpointReleases(ctx, gen.ListEndpointReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newEndpointReleaseService(t, []client.Object{testEndpointReleaseObj()}, &denyAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.ListEndpointReleases(ctx, gen.ListEndpointReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetEndpointRelease Handler ---

func TestGetEndpointReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEndpointReleaseService(t, []client.Object{testEndpointReleaseObj()}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.GetEndpointRelease(ctx, gen.GetEndpointReleaseRequestObject{NamespaceName: ns, EndpointReleaseName: "rr-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetEndpointRelease200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rr-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.GetEndpointRelease(ctx, gen.GetEndpointReleaseRequestObject{NamespaceName: ns, EndpointReleaseName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointRelease404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointReleaseService(t, []client.Object{testEndpointReleaseObj()}, &denyAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.GetEndpointRelease(ctx, gen.GetEndpointReleaseRequestObject{NamespaceName: ns, EndpointReleaseName: "rr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointRelease403JSONResponse{}, resp)
	})
}

// --- CreateEndpointRelease Handler ---

// --- DeleteEndpointRelease Handler ---

func TestDeleteEndpointReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEndpointReleaseService(t, []client.Object{testEndpointReleaseObj()}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.DeleteEndpointRelease(ctx, gen.DeleteEndpointReleaseRequestObject{NamespaceName: ns, EndpointReleaseName: "rr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointRelease204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.DeleteEndpointRelease(ctx, gen.DeleteEndpointReleaseRequestObject{NamespaceName: ns, EndpointReleaseName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointRelease404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointReleaseService(t, []client.Object{testEndpointReleaseObj()}, &denyAllPDP{})
		h := newHandlerWithEndpointReleaseService(svc)

		resp, err := h.DeleteEndpointRelease(ctx, gen.DeleteEndpointReleaseRequestObject{NamespaceName: ns, EndpointReleaseName: "rr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointRelease403JSONResponse{}, resp)
	})
}
