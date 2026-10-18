// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	endpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointtype"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newEndpointTypeService(t *testing.T, objects []client.Object, pdp authzcore.PDP) endpointtypesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return endpointtypesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithEndpointTypeService(svc endpointtypesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{EndpointTypeService: svc},
		logger:   slog.Default(),
	}
}

func testEndpointTypeObj(name string) *openchoreov1alpha1.EndpointType {
	return &openchoreov1alpha1.EndpointType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: openchoreov1alpha1.EndpointTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTemplate{{
				ID:       "route",
				Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"x"}}`)},
			}},
		},
	}
}

// --- ListEndpointTypes Handler ---

func TestListEndpointTypesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.ListEndpointTypes(ctx, gen.ListEndpointTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "rt-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.ListEndpointTypes(ctx, gen.ListEndpointTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.ListEndpointTypes(ctx, gen.ListEndpointTypesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListEndpointTypesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListEndpointTypes400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.ListEndpointTypes(ctx, gen.ListEndpointTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpointTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetEndpointType Handler ---

func TestGetEndpointTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.GetEndpointType(ctx, gen.GetEndpointTypeRequestObject{NamespaceName: ns, EtName: "rt-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetEndpointType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rt-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.GetEndpointType(ctx, gen.GetEndpointTypeRequestObject{NamespaceName: ns, EtName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.GetEndpointType(ctx, gen.GetEndpointTypeRequestObject{NamespaceName: ns, EtName: "rt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointType403JSONResponse{}, resp)
	})
}

// --- CreateEndpointType Handler ---

func TestCreateEndpointTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.CreateEndpointType(ctx, gen.CreateEndpointTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.EndpointType{Metadata: gen.ObjectMeta{Name: "new-rt"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateEndpointType201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-rt", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.CreateEndpointType(ctx, gen.CreateEndpointTypeRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpointType400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testEndpointTypeObj("new-rt")
		svc := newEndpointTypeService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.CreateEndpointType(ctx, gen.CreateEndpointTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.EndpointType{Metadata: gen.ObjectMeta{Name: "new-rt"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpointType409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &denyAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.CreateEndpointType(ctx, gen.CreateEndpointTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.EndpointType{Metadata: gen.ObjectMeta{Name: "new-rt"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpointType403JSONResponse{}, resp)
	})
}

// --- UpdateEndpointType Handler ---

func TestUpdateEndpointTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.UpdateEndpointType(ctx, gen.UpdateEndpointTypeRequestObject{
			NamespaceName: ns,
			EtName:        "rt-1",
			Body:          &gen.EndpointType{Metadata: gen.ObjectMeta{Name: "rt-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateEndpointType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rt-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.UpdateEndpointType(ctx, gen.UpdateEndpointTypeRequestObject{
			NamespaceName: ns,
			EtName:        "rt-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEndpointType400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.UpdateEndpointType(ctx, gen.UpdateEndpointTypeRequestObject{
			NamespaceName: ns,
			EtName:        "nonexistent",
			Body:          &gen.EndpointType{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEndpointType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.UpdateEndpointType(ctx, gen.UpdateEndpointTypeRequestObject{
			NamespaceName: ns,
			EtName:        "rt-1",
			Body:          &gen.EndpointType{Metadata: gen.ObjectMeta{Name: "rt-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEndpointType403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.UpdateEndpointType(ctx, gen.UpdateEndpointTypeRequestObject{
			NamespaceName: ns,
			EtName:        "rt-1",
			Body:          &gen.EndpointType{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateEndpointType200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "rt-1", typed.Metadata.Name)
	})
}

// --- DeleteEndpointType Handler ---

func TestDeleteEndpointTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.DeleteEndpointType(ctx, gen.DeleteEndpointTypeRequestObject{NamespaceName: ns, EtName: "rt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointType204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.DeleteEndpointType(ctx, gen.DeleteEndpointTypeRequestObject{NamespaceName: ns, EtName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{testEndpointTypeObj("rt-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.DeleteEndpointType(ctx, gen.DeleteEndpointTypeRequestObject{NamespaceName: ns, EtName: "rt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpointType403JSONResponse{}, resp)
	})
}

// --- GetEndpointTypeSchema Handler ---

func TestGetEndpointTypeSchemaHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	paramsRaw, _ := json.Marshal(map[string]any{"version": "string"})
	rt := testEndpointTypeObj("http-gateway")
	rt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
	}

	t.Run("success", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{rt}, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.GetEndpointTypeSchema(ctx, gen.GetEndpointTypeSchemaRequestObject{
			NamespaceName: ns,
			EtName:        "http-gateway",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetEndpointTypeSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.GetEndpointTypeSchema(ctx, gen.GetEndpointTypeSchemaRequestObject{
			NamespaceName: ns,
			EtName:        "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointTypeSchema404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointTypeService(t, []client.Object{rt}, &denyAllPDP{})
		h := newHandlerWithEndpointTypeService(svc)

		resp, err := h.GetEndpointTypeSchema(ctx, gen.GetEndpointTypeSchemaRequestObject{
			NamespaceName: ns,
			EtName:        "http-gateway",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpointTypeSchema403JSONResponse{}, resp)
	})
}
