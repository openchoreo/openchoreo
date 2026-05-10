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
	resourcereleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcerelease"
)

func newResourceReleaseService(t *testing.T, objects []client.Object, pdp authzcore.PDP) resourcereleasesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return resourcereleasesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithResourceReleaseService(svc resourcereleasesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ResourceReleaseService: svc},
		logger:   slog.Default(),
	}
}

func testResourceReleaseObj() *openchoreov1alpha1.ResourceRelease {
	return &openchoreov1alpha1.ResourceRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rr-1",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.ResourceReleaseSpec{
			Owner: openchoreov1alpha1.ResourceReleaseOwner{
				ProjectName:  "test-project",
				ResourceName: "test-r",
			},
		},
	}
}

// --- ListResourceReleases Handler ---

func TestListResourceReleasesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newResourceReleaseService(t, []client.Object{testResourceReleaseObj()}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.ListResourceReleases(ctx, gen.ListResourceReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("filtered by resource", func(t *testing.T) {
		svc := newResourceReleaseService(t, []client.Object{testResourceReleaseObj()}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.ListResourceReleases(ctx, gen.ListResourceReleasesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListResourceReleasesParams{Resource: ptr.To("test-r")},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newResourceReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.ListResourceReleases(ctx, gen.ListResourceReleasesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListResourceReleasesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListResourceReleases400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newResourceReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.ListResourceReleases(ctx, gen.ListResourceReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newResourceReleaseService(t, []client.Object{testResourceReleaseObj()}, &denyAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.ListResourceReleases(ctx, gen.ListResourceReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetResourceRelease Handler ---

func TestGetResourceReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newResourceReleaseService(t, []client.Object{testResourceReleaseObj()}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.GetResourceRelease(ctx, gen.GetResourceReleaseRequestObject{NamespaceName: ns, ResourceReleaseName: "rr-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetResourceRelease200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rr-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.GetResourceRelease(ctx, gen.GetResourceReleaseRequestObject{NamespaceName: ns, ResourceReleaseName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResourceRelease404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceReleaseService(t, []client.Object{testResourceReleaseObj()}, &denyAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.GetResourceRelease(ctx, gen.GetResourceReleaseRequestObject{NamespaceName: ns, ResourceReleaseName: "rr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResourceRelease403JSONResponse{}, resp)
	})
}

// --- CreateResourceRelease Handler ---

func TestCreateResourceReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newResourceReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		body := gen.ResourceRelease{
			Metadata: gen.ObjectMeta{Name: "rr-new"},
			Spec: &gen.ResourceReleaseSpec{
				Owner: struct {
					ProjectName  string `json:"projectName"`
					ResourceName string `json:"resourceName"`
				}{ProjectName: "test-project", ResourceName: "test-r"},
				ResourceType: struct {
					Kind gen.ResourceReleaseSpecResourceTypeKind `json:"kind"`
					Name string                                  `json:"name"`
					Spec gen.ResourceTypeSpec                    `json:"spec"`
				}{
					Kind: gen.ResourceReleaseSpecResourceTypeKindResourceType,
					Name: "mysql",
				},
			},
		}
		resp, err := h.CreateResourceRelease(ctx, gen.CreateResourceReleaseRequestObject{
			NamespaceName: ns,
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateResourceRelease201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "rr-new", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newResourceReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.CreateResourceRelease(ctx, gen.CreateResourceReleaseRequestObject{NamespaceName: ns, Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceRelease400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceReleaseService(t, nil, &denyAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		body := gen.ResourceRelease{
			Metadata: gen.ObjectMeta{Name: "rr-new"},
			Spec: &gen.ResourceReleaseSpec{
				Owner: struct {
					ProjectName  string `json:"projectName"`
					ResourceName string `json:"resourceName"`
				}{ProjectName: "test-project", ResourceName: "test-r"},
				ResourceType: struct {
					Kind gen.ResourceReleaseSpecResourceTypeKind `json:"kind"`
					Name string                                  `json:"name"`
					Spec gen.ResourceTypeSpec                    `json:"spec"`
				}{
					Kind: gen.ResourceReleaseSpecResourceTypeKindResourceType,
					Name: "mysql",
				},
			},
		}
		resp, err := h.CreateResourceRelease(ctx, gen.CreateResourceReleaseRequestObject{NamespaceName: ns, Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceRelease403JSONResponse{}, resp)
	})
}

// --- DeleteResourceRelease Handler ---

func TestDeleteResourceReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newResourceReleaseService(t, []client.Object{testResourceReleaseObj()}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.DeleteResourceRelease(ctx, gen.DeleteResourceReleaseRequestObject{NamespaceName: ns, ResourceReleaseName: "rr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceRelease204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.DeleteResourceRelease(ctx, gen.DeleteResourceReleaseRequestObject{NamespaceName: ns, ResourceReleaseName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceRelease404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceReleaseService(t, []client.Object{testResourceReleaseObj()}, &denyAllPDP{})
		h := newHandlerWithResourceReleaseService(svc)

		resp, err := h.DeleteResourceRelease(ctx, gen.DeleteResourceReleaseRequestObject{NamespaceName: ns, ResourceReleaseName: "rr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceRelease403JSONResponse{}, resp)
	})
}
