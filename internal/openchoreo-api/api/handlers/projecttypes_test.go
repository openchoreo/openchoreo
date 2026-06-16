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
	projecttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projecttype"
)

func newProjectTypeService(t *testing.T, objects []client.Object, pdp authzcore.PDP) projecttypesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return projecttypesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithProjectTypeService(svc projecttypesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ProjectTypeService: svc},
		logger:   slog.Default(),
	}
}

func testProjectTypeObj(name string) *openchoreov1alpha1.ProjectType {
	return &openchoreov1alpha1.ProjectType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ProjectTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTemplate{{
				ID:       "namespace",
				Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"x"}}`)},
			}},
		},
	}
}

// --- ListProjectTypes Handler ---

func TestListProjectTypesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.ListProjectTypes(ctx, gen.ListProjectTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "pt-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.ListProjectTypes(ctx, gen.ListProjectTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.ListProjectTypes(ctx, gen.ListProjectTypesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListProjectTypesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListProjectTypes400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &denyAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.ListProjectTypes(ctx, gen.ListProjectTypesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetProjectType Handler ---

func TestGetProjectTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.GetProjectType(ctx, gen.GetProjectTypeRequestObject{NamespaceName: ns, PtName: "pt-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetProjectType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "pt-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.GetProjectType(ctx, gen.GetProjectTypeRequestObject{NamespaceName: ns, PtName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProjectType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &denyAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.GetProjectType(ctx, gen.GetProjectTypeRequestObject{NamespaceName: ns, PtName: "pt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProjectType403JSONResponse{}, resp)
	})
}

// --- CreateProjectType Handler ---

func TestCreateProjectTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.CreateProjectType(ctx, gen.CreateProjectTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ProjectType{Metadata: gen.ObjectMeta{Name: "new-pt"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateProjectType201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-pt", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.CreateProjectType(ctx, gen.CreateProjectTypeRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectType400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testProjectTypeObj("new-pt")
		svc := newProjectTypeService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.CreateProjectType(ctx, gen.CreateProjectTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ProjectType{Metadata: gen.ObjectMeta{Name: "new-pt"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectType409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &denyAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.CreateProjectType(ctx, gen.CreateProjectTypeRequestObject{
			NamespaceName: ns,
			Body:          &gen.ProjectType{Metadata: gen.ObjectMeta{Name: "new-pt"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectType403JSONResponse{}, resp)
	})
}

// --- UpdateProjectType Handler ---

func TestUpdateProjectTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.UpdateProjectType(ctx, gen.UpdateProjectTypeRequestObject{
			NamespaceName: ns,
			PtName:        "pt-1",
			Body:          &gen.ProjectType{Metadata: gen.ObjectMeta{Name: "pt-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateProjectType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "pt-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.UpdateProjectType(ctx, gen.UpdateProjectTypeRequestObject{
			NamespaceName: ns,
			PtName:        "pt-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateProjectType400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.UpdateProjectType(ctx, gen.UpdateProjectTypeRequestObject{
			NamespaceName: ns,
			PtName:        "nonexistent",
			Body:          &gen.ProjectType{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateProjectType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &denyAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.UpdateProjectType(ctx, gen.UpdateProjectTypeRequestObject{
			NamespaceName: ns,
			PtName:        "pt-1",
			Body:          &gen.ProjectType{Metadata: gen.ObjectMeta{Name: "pt-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateProjectType403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.UpdateProjectType(ctx, gen.UpdateProjectTypeRequestObject{
			NamespaceName: ns,
			PtName:        "pt-1",
			Body:          &gen.ProjectType{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateProjectType200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "pt-1", typed.Metadata.Name)
	})
}

// --- DeleteProjectType Handler ---

func TestDeleteProjectTypeHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.DeleteProjectType(ctx, gen.DeleteProjectTypeRequestObject{NamespaceName: ns, PtName: "pt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectType204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.DeleteProjectType(ctx, gen.DeleteProjectTypeRequestObject{NamespaceName: ns, PtName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{testProjectTypeObj("pt-1")}, &denyAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.DeleteProjectType(ctx, gen.DeleteProjectTypeRequestObject{NamespaceName: ns, PtName: "pt-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectType403JSONResponse{}, resp)
	})
}

// --- GetProjectTypeSchema Handler ---

func TestGetProjectTypeSchemaHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	paramsRaw, _ := json.Marshal(map[string]any{"tier": "string"})
	pt := testProjectTypeObj("standard-project")
	pt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
	}

	t.Run("success", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{pt}, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.GetProjectTypeSchema(ctx, gen.GetProjectTypeSchemaRequestObject{
			NamespaceName: ns,
			PtName:        "standard-project",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetProjectTypeSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.GetProjectTypeSchema(ctx, gen.GetProjectTypeSchemaRequestObject{
			NamespaceName: ns,
			PtName:        "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProjectTypeSchema404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectTypeService(t, []client.Object{pt}, &denyAllPDP{})
		h := newHandlerWithProjectTypeService(svc)

		resp, err := h.GetProjectTypeSchema(ctx, gen.GetProjectTypeSchemaRequestObject{
			NamespaceName: ns,
			PtName:        "standard-project",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProjectTypeSchema403JSONResponse{}, resp)
	})
}
