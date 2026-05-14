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
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	resourcetypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcetype"
)

func newResourceTypeService(t *testing.T, objects []client.Object, pdp authzcore.PDP) resourcetypesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return resourcetypesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithResourceTypeService(svc resourcetypesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ResourceTypeService: svc},
		logger:   slog.Default(),
	}
}

func testResourceTypeObj(name string) *openchoreov1alpha1.ResourceType {
	return &openchoreov1alpha1.ResourceType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ResourceTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTypeManifest{{
				ID:       "claim",
				Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"x"}}`)},
			}},
		},
	}
}

// --- ListResourceTypes Handler ---

func TestListResourceTypesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.ListResourceTypes(ctx, gen.ListResourceTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "rt-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.ListResourceTypes(ctx, gen.ListResourceTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.ListResourceTypes(ctx, gen.ListResourceTypesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListResourceTypesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListResourceTypes400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &denyAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.ListResourceTypes(ctx, gen.ListResourceTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetResourceType Handler ---

func TestGetResourceTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.GetResourceType(ctx, gen.GetResourceTypeRequestObject{NamespaceName: ns, RtName: "rt-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetResourceType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rt-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.GetResourceType(ctx, gen.GetResourceTypeRequestObject{NamespaceName: ns, RtName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResourceType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &denyAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.GetResourceType(ctx, gen.GetResourceTypeRequestObject{NamespaceName: ns, RtName: "rt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResourceType403JSONResponse{}, resp)
	})
}

// --- CreateResourceType Handler ---

func TestCreateResourceTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.CreateResourceType(ctx, gen.CreateResourceTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ResourceType{Metadata: gen.ObjectMeta{Name: "new-rt"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateResourceType201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-rt", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.CreateResourceType(ctx, gen.CreateResourceTypeRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceType400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testResourceTypeObj("new-rt")
		svc := newResourceTypeService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.CreateResourceType(ctx, gen.CreateResourceTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ResourceType{Metadata: gen.ObjectMeta{Name: "new-rt"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceType409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &denyAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.CreateResourceType(ctx, gen.CreateResourceTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ResourceType{Metadata: gen.ObjectMeta{Name: "new-rt"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceType403JSONResponse{}, resp)
	})
}

// --- UpdateResourceType Handler ---

func TestUpdateResourceTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.UpdateResourceType(ctx, gen.UpdateResourceTypeRequestObject{
			NamespaceName: ns,
			RtName:        "rt-1",
			Body:          &gen.ResourceType{Metadata: gen.ObjectMeta{Name: "rt-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateResourceType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rt-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.UpdateResourceType(ctx, gen.UpdateResourceTypeRequestObject{
			NamespaceName: ns,
			RtName:        "rt-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateResourceType400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.UpdateResourceType(ctx, gen.UpdateResourceTypeRequestObject{
			NamespaceName: ns,
			RtName:        "nonexistent",
			Body:          &gen.ResourceType{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateResourceType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &denyAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.UpdateResourceType(ctx, gen.UpdateResourceTypeRequestObject{
			NamespaceName: ns,
			RtName:        "rt-1",
			Body:          &gen.ResourceType{Metadata: gen.ObjectMeta{Name: "rt-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateResourceType403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.UpdateResourceType(ctx, gen.UpdateResourceTypeRequestObject{
			NamespaceName: ns,
			RtName:        "rt-1",
			Body:          &gen.ResourceType{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateResourceType200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "rt-1", typed.Metadata.Name)
	})
}

// --- DeleteResourceType Handler ---

func TestDeleteResourceTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.DeleteResourceType(ctx, gen.DeleteResourceTypeRequestObject{NamespaceName: ns, RtName: "rt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceType204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.DeleteResourceType(ctx, gen.DeleteResourceTypeRequestObject{NamespaceName: ns, RtName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{testResourceTypeObj("rt-1")}, &denyAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.DeleteResourceType(ctx, gen.DeleteResourceTypeRequestObject{NamespaceName: ns, RtName: "rt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceType403JSONResponse{}, resp)
	})
}

// --- GetResourceTypeSchema Handler ---

func TestGetResourceTypeSchemaHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	paramsRaw, _ := json.Marshal(map[string]any{"version": "string"})
	rt := testResourceTypeObj("mysql")
	rt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
	}

	t.Run("success", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{rt}, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.GetResourceTypeSchema(ctx, gen.GetResourceTypeSchemaRequestObject{
			NamespaceName: ns,
			RtName:        "mysql",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetResourceTypeSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.GetResourceTypeSchema(ctx, gen.GetResourceTypeSchemaRequestObject{
			NamespaceName: ns,
			RtName:        "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResourceTypeSchema404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceTypeService(t, []client.Object{rt}, &denyAllPDP{})
		h := newHandlerWithResourceTypeService(svc)

		resp, err := h.GetResourceTypeSchema(ctx, gen.GetResourceTypeSchemaRequestObject{
			NamespaceName: ns,
			RtName:        "mysql",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResourceTypeSchema403JSONResponse{}, resp)
	})
}
