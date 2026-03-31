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
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newComponentTypeService(t *testing.T, objects []client.Object, pdp authzcore.PDP) componenttypesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return componenttypesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithComponentTypeService(svc componenttypesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ComponentTypeService: svc},
		logger:   slog.Default(),
	}
}

func testComponentTypeObj(name string) *openchoreov1alpha1.ComponentType {
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
			Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
		},
	}
}

// --- ListComponentTypes Handler ---

func TestListComponentTypesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.ListComponentTypes(ctx, gen.ListComponentTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListComponentTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "ct-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.ListComponentTypes(ctx, gen.ListComponentTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListComponentTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.ListComponentTypes(ctx, gen.ListComponentTypesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListComponentTypesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListComponentTypes400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &denyAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.ListComponentTypes(ctx, gen.ListComponentTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListComponentTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetComponentType Handler ---

func TestGetComponentTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.GetComponentType(ctx, gen.GetComponentTypeRequestObject{NamespaceName: ns, CtName: "ct-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetComponentType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "ct-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.GetComponentType(ctx, gen.GetComponentTypeRequestObject{NamespaceName: ns, CtName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetComponentType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &denyAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.GetComponentType(ctx, gen.GetComponentTypeRequestObject{NamespaceName: ns, CtName: "ct-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetComponentType403JSONResponse{}, resp)
	})
}

// --- CreateComponentType Handler ---

func TestCreateComponentTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.CreateComponentType(ctx, gen.CreateComponentTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "new-ct"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateComponentType201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-ct", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.CreateComponentType(ctx, gen.CreateComponentTypeRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentType400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testComponentTypeObj("new-ct")
		svc := newComponentTypeService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.CreateComponentType(ctx, gen.CreateComponentTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "new-ct"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentType409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &denyAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.CreateComponentType(ctx, gen.CreateComponentTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "new-ct"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentType403JSONResponse{}, resp)
	})
}

// --- UpdateComponentType Handler ---

func TestUpdateComponentTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.UpdateComponentType(ctx, gen.UpdateComponentTypeRequestObject{
			NamespaceName: ns,
			CtName:        "ct-1",
			Body:          &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "ct-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateComponentType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "ct-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.UpdateComponentType(ctx, gen.UpdateComponentTypeRequestObject{
			NamespaceName: ns,
			CtName:        "ct-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentType400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.UpdateComponentType(ctx, gen.UpdateComponentTypeRequestObject{
			NamespaceName: ns,
			CtName:        "nonexistent",
			Body:          &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &denyAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.UpdateComponentType(ctx, gen.UpdateComponentTypeRequestObject{
			NamespaceName: ns,
			CtName:        "ct-1",
			Body:          &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "ct-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentType403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.UpdateComponentType(ctx, gen.UpdateComponentTypeRequestObject{
			NamespaceName: ns,
			CtName:        "ct-1",
			Body:          &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateComponentType200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "ct-1", typed.Metadata.Name)
	})
}

// --- DeleteComponentType Handler ---

func TestDeleteComponentTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.DeleteComponentType(ctx, gen.DeleteComponentTypeRequestObject{NamespaceName: ns, CtName: "ct-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentType204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.DeleteComponentType(ctx, gen.DeleteComponentTypeRequestObject{NamespaceName: ns, CtName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{testComponentTypeObj("ct-1")}, &denyAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.DeleteComponentType(ctx, gen.DeleteComponentTypeRequestObject{NamespaceName: ns, CtName: "ct-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentType403JSONResponse{}, resp)
	})
}

// --- GetComponentTypeSchema Handler ---

func TestGetComponentTypeSchemaHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	paramsRaw, _ := json.Marshal(map[string]any{"replicas": "integer"})
	ct := &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "go-service", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
			Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{ct}, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.GetComponentTypeSchema(ctx, gen.GetComponentTypeSchemaRequestObject{
			NamespaceName: ns,
			CtName:        "go-service",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetComponentTypeSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.GetComponentTypeSchema(ctx, gen.GetComponentTypeSchemaRequestObject{
			NamespaceName: ns,
			CtName:        "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetComponentTypeSchema404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentTypeService(t, []client.Object{ct}, &denyAllPDP{})
		h := newHandlerWithComponentTypeService(svc)

		resp, err := h.GetComponentTypeSchema(ctx, gen.GetComponentTypeSchemaRequestObject{
			NamespaceName: ns,
			CtName:        "go-service",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetComponentTypeSchema403JSONResponse{}, resp)
	})
}
