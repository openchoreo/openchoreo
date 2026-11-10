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
	endpointreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

const (
	testERBNs       = "test-ns"
	testERBProject  = "test-project"
	testERBEndpoint = "test-r"
)

func newEndpointReleaseBindingService(t *testing.T, objects []client.Object, pdp authzcore.PDP) endpointreleasebindingsvc.Service {
	t.Helper()
	// Bootstrap with an Endpoint so create-time validation passes.
	bootstrap := make([]client.Object, 0, 1+len(objects))
	bootstrap = append(bootstrap, &openchoreov1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{Name: testERBEndpoint, Namespace: testERBNs},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner: openchoreov1alpha1.EndpointOwner{ProjectName: testERBProject},
			Type: openchoreov1alpha1.EndpointTypeRef{
				Kind: openchoreov1alpha1.EndpointTypeRefKindEndpointType,
				Name: "http-gateway",
			},
		},
	})
	bootstrap = append(bootstrap, objects...)

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(bootstrap...).
		Build()
	return endpointreleasebindingsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithEndpointReleaseBindingService(svc endpointreleasebindingsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{EndpointReleaseBindingService: svc},
		logger:   slog.Default(),
	}
}

func testEndpointReleaseBindingObj(name string) *openchoreov1alpha1.EndpointReleaseBinding {
	return &openchoreov1alpha1.EndpointReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testERBNs},
		Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
			Owner: openchoreov1alpha1.EndpointReleaseBindingOwner{
				ProjectName:  testERBProject,
				EndpointName: testERBEndpoint,
			},
			Environment: "dev",
		},
	}
}

// --- ListEndpointReleaseBindings Handler ---

func TestListEndpointReleaseBindingsHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.ListEndpointReleaseBindings(ctx, gen.ListEndpointReleaseBindingsRequestObject{NamespaceName: testERBNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("filtered by resource", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.ListEndpointReleaseBindings(ctx, gen.ListEndpointReleaseBindingsRequestObject{
			NamespaceName: testERBNs,
			Params:        gen.ListEndpointReleaseBindingsParams{Endpoint: ptr.To(testERBEndpoint)},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.ListEndpointReleaseBindings(ctx, gen.ListEndpointReleaseBindingsRequestObject{
			NamespaceName: testERBNs,
			Params:        gen.ListEndpointReleaseBindingsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListEndpointReleaseBindings400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.ListEndpointReleaseBindings(ctx, gen.ListEndpointReleaseBindingsRequestObject{NamespaceName: testERBNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.ListEndpointReleaseBindings(ctx, gen.ListEndpointReleaseBindingsRequestObject{NamespaceName: testERBNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetEndpointReleaseBinding Handler ---

func TestGetEndpointReleaseBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.GetEndpointReleaseBinding(ctx, gen.GetEndpointReleaseBindingRequestObject{NamespaceName: testERBNs, EndpointReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetEndpointReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rb-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.GetEndpointReleaseBinding(ctx, gen.GetEndpointReleaseBindingRequestObject{NamespaceName: testERBNs, EndpointReleaseBindingName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.GetEndpointReleaseBinding(ctx, gen.GetEndpointReleaseBindingRequestObject{NamespaceName: testERBNs, EndpointReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointReleaseBinding403JSONResponse{}, resp)
	})
}

// --- CreateEndpointReleaseBinding Handler ---

func newERBCreateBody(name string) gen.EndpointReleaseBinding {
	return gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.EndpointReleaseBindingSpec{
			Owner: struct {
				EndpointName string `json:"endpointName"`
				ProjectName  string `json:"projectName"`
			}{ProjectName: testERBProject, EndpointName: testERBEndpoint},
			Environment: "dev",
		},
	}
}

func TestCreateEndpointReleaseBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		body := newERBCreateBody("rb-new")
		resp, err := h.CreateEndpointReleaseBinding(ctx, gen.CreateEndpointReleaseBindingRequestObject{
			NamespaceName: testERBNs,
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateEndpointReleaseBinding201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "rb-new", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.CreateEndpointReleaseBinding(ctx, gen.CreateEndpointReleaseBindingRequestObject{NamespaceName: testERBNs, Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpointReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("missing resource returns 400", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		body := newERBCreateBody("rb-new")
		body.Spec.Owner.EndpointName = "missing-r"
		resp, err := h.CreateEndpointReleaseBinding(ctx, gen.CreateEndpointReleaseBindingRequestObject{
			NamespaceName: testERBNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpointReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testEndpointReleaseBindingObj("rb-new")
		svc := newEndpointReleaseBindingService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		body := newERBCreateBody("rb-new")
		resp, err := h.CreateEndpointReleaseBinding(ctx, gen.CreateEndpointReleaseBindingRequestObject{
			NamespaceName: testERBNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpointReleaseBinding409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &denyAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		body := newERBCreateBody("rb-new")
		resp, err := h.CreateEndpointReleaseBinding(ctx, gen.CreateEndpointReleaseBindingRequestObject{
			NamespaceName: testERBNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpointReleaseBinding403JSONResponse{}, resp)
	})
}

// --- UpdateEndpointReleaseBinding Handler ---

func TestUpdateEndpointReleaseBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		body := newERBCreateBody("rb-1")
		resp, err := h.UpdateEndpointReleaseBinding(ctx, gen.UpdateEndpointReleaseBindingRequestObject{
			NamespaceName:              testERBNs,
			EndpointReleaseBindingName: "rb-1",
			Body:                       &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateEndpointReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rb-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.UpdateEndpointReleaseBinding(ctx, gen.UpdateEndpointReleaseBindingRequestObject{
			NamespaceName:              testERBNs,
			EndpointReleaseBindingName: "rb-1",
			Body:                       nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEndpointReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		body := newERBCreateBody("nonexistent")
		resp, err := h.UpdateEndpointReleaseBinding(ctx, gen.UpdateEndpointReleaseBindingRequestObject{
			NamespaceName:              testERBNs,
			EndpointReleaseBindingName: "nonexistent",
			Body:                       &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEndpointReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		body := newERBCreateBody("different-name")
		resp, err := h.UpdateEndpointReleaseBinding(ctx, gen.UpdateEndpointReleaseBindingRequestObject{
			NamespaceName:              testERBNs,
			EndpointReleaseBindingName: "rb-1",
			Body:                       &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateEndpointReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "rb-1", typed.Metadata.Name)
	})
}

// --- DeleteEndpointReleaseBinding Handler ---

func TestDeleteEndpointReleaseBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.DeleteEndpointReleaseBinding(ctx, gen.DeleteEndpointReleaseBindingRequestObject{NamespaceName: testERBNs, EndpointReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointReleaseBinding204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.DeleteEndpointReleaseBinding(ctx, gen.DeleteEndpointReleaseBindingRequestObject{NamespaceName: testERBNs, EndpointReleaseBindingName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointReleaseBindingService(t, []client.Object{testEndpointReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointReleaseBindingService(svc)

		resp, err := h.DeleteEndpointReleaseBinding(ctx, gen.DeleteEndpointReleaseBindingRequestObject{NamespaceName: testERBNs, EndpointReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointReleaseBinding403JSONResponse{}, resp)
	})
}
