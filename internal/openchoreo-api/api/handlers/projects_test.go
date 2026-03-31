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
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
)

func newProjectService(t *testing.T, objects []client.Object, pdp authzcore.PDP) projectsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return projectsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithProjectService(svc projectsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ProjectService: svc},
		logger:   slog.Default(),
	}
}

func testProjectObj(name string) *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
	}
}

// --- ListProjects Handler ---

func TestListProjectsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.ListProjects(ctx, gen.ListProjectsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjects200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "proj-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newProjectService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.ListProjects(ctx, gen.ListProjectsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjects200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newProjectService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.ListProjects(ctx, gen.ListProjectsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListProjectsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListProjects400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &denyAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.ListProjects(ctx, gen.ListProjectsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjects200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetProject Handler ---

func TestGetProjectHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.GetProject(ctx, gen.GetProjectRequestObject{NamespaceName: ns, ProjectName: "proj-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetProject200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "proj-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.GetProject(ctx, gen.GetProjectRequestObject{NamespaceName: ns, ProjectName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProject404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &denyAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.GetProject(ctx, gen.GetProjectRequestObject{NamespaceName: ns, ProjectName: "proj-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProject403JSONResponse{}, resp)
	})
}

// --- CreateProject Handler ---

func TestCreateProjectHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	validBody := &gen.Project{
		Metadata: gen.ObjectMeta{Name: "new-proj"},
	}

	t.Run("success", func(t *testing.T) {
		svc := newProjectService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.CreateProject(ctx, gen.CreateProjectRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateProject201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-proj", typed.Metadata.Name)
		require.NotNil(t, typed.Metadata.Namespace)
		assert.Equal(t, ns, *typed.Metadata.Namespace)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newProjectService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.CreateProject(ctx, gen.CreateProjectRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProject400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testProjectObj("new-proj")
		svc := newProjectService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.CreateProject(ctx, gen.CreateProjectRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProject409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectService(t, nil, &denyAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.CreateProject(ctx, gen.CreateProjectRequestObject{
			NamespaceName: ns,
			Body:          validBody,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProject403JSONResponse{}, resp)
	})
}

// --- UpdateProject Handler ---

func TestUpdateProjectHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.UpdateProject(ctx, gen.UpdateProjectRequestObject{
			NamespaceName: ns,
			ProjectName:   "proj-1",
			Body:          &gen.Project{Metadata: gen.ObjectMeta{Name: "proj-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateProject200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "proj-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newProjectService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.UpdateProject(ctx, gen.UpdateProjectRequestObject{
			NamespaceName: ns,
			ProjectName:   "proj-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateProject400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.UpdateProject(ctx, gen.UpdateProjectRequestObject{
			NamespaceName: ns,
			ProjectName:   "nonexistent",
			Body:          &gen.Project{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateProject404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &denyAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.UpdateProject(ctx, gen.UpdateProjectRequestObject{
			NamespaceName: ns,
			ProjectName:   "proj-1",
			Body:          &gen.Project{Metadata: gen.ObjectMeta{Name: "proj-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateProject403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		// Body has different name than URL path
		resp, err := h.UpdateProject(ctx, gen.UpdateProjectRequestObject{
			NamespaceName: ns,
			ProjectName:   "proj-1",
			Body:          &gen.Project{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		// Should succeed using the URL path name "proj-1", not the body name
		typed, ok := resp.(gen.UpdateProject200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "proj-1", typed.Metadata.Name)
	})
}

// --- DeleteProject Handler ---

func TestDeleteProjectHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.DeleteProject(ctx, gen.DeleteProjectRequestObject{NamespaceName: ns, ProjectName: "proj-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProject204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.DeleteProject(ctx, gen.DeleteProjectRequestObject{NamespaceName: ns, ProjectName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProject404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectService(t, []client.Object{testProjectObj("proj-1")}, &denyAllPDP{})
		h := newHandlerWithProjectService(svc)

		resp, err := h.DeleteProject(ctx, gen.DeleteProjectRequestObject{NamespaceName: ns, ProjectName: "proj-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProject403JSONResponse{}, resp)
	})
}
