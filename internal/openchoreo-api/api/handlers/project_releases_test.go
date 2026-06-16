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
	projectreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectrelease"
)

func newProjectReleaseService(t *testing.T, objects []client.Object, pdp authzcore.PDP) projectreleasesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return projectreleasesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithProjectReleaseService(svc projectreleasesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ProjectReleaseService: svc},
		logger:   slog.Default(),
	}
}

func testProjectReleaseObj(name string) *openchoreov1alpha1.ProjectRelease {
	return &openchoreov1alpha1.ProjectRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.ProjectReleaseSpec{
			Owner: openchoreov1alpha1.ProjectReleaseOwner{
				ProjectName: "my-app",
			},
		},
	}
}

// --- ListProjectReleases Handler ---

func TestListProjectReleasesHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newProjectReleaseService(t, []client.Object{testProjectReleaseObj("pr-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.ListProjectReleases(ctx, gen.ListProjectReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "pr-1", typed.Items[0].Metadata.Name)
	})

	t.Run("filter by project", func(t *testing.T) {
		pr1 := testProjectReleaseObj("pr-1")
		pr2 := testProjectReleaseObj("pr-2")
		pr2.Spec.Owner.ProjectName = "other-app"
		svc := newProjectReleaseService(t, []client.Object{pr1, pr2}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.ListProjectReleases(ctx, gen.ListProjectReleasesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListProjectReleasesParams{Project: ptr.To("my-app")},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "pr-1", typed.Items[0].Metadata.Name)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newProjectReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.ListProjectReleases(ctx, gen.ListProjectReleasesRequestObject{
			NamespaceName: ns,
			Params:        gen.ListProjectReleasesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListProjectReleases400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newProjectReleaseService(t, []client.Object{testProjectReleaseObj("pr-1")}, &denyAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.ListProjectReleases(ctx, gen.ListProjectReleasesRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectReleases200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetProjectRelease Handler ---

func TestGetProjectReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectReleaseService(t, []client.Object{testProjectReleaseObj("pr-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.GetProjectRelease(ctx, gen.GetProjectReleaseRequestObject{NamespaceName: ns, ProjectReleaseName: "pr-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetProjectRelease200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "pr-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.GetProjectRelease(ctx, gen.GetProjectReleaseRequestObject{NamespaceName: ns, ProjectReleaseName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProjectRelease404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectReleaseService(t, []client.Object{testProjectReleaseObj("pr-1")}, &denyAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.GetProjectRelease(ctx, gen.GetProjectReleaseRequestObject{NamespaceName: ns, ProjectReleaseName: "pr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProjectRelease403JSONResponse{}, resp)
	})
}

// --- CreateProjectRelease Handler ---

func TestCreateProjectReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.CreateProjectRelease(ctx, gen.CreateProjectReleaseRequestObject{
			NamespaceName: ns,
			Body:          &gen.ProjectRelease{Metadata: gen.ObjectMeta{Name: "new-pr"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateProjectRelease201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-pr", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newProjectReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.CreateProjectRelease(ctx, gen.CreateProjectReleaseRequestObject{NamespaceName: ns, Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectRelease400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testProjectReleaseObj("new-pr")
		svc := newProjectReleaseService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.CreateProjectRelease(ctx, gen.CreateProjectReleaseRequestObject{
			NamespaceName: ns,
			Body:          &gen.ProjectRelease{Metadata: gen.ObjectMeta{Name: "new-pr"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectRelease409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectReleaseService(t, nil, &denyAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.CreateProjectRelease(ctx, gen.CreateProjectReleaseRequestObject{
			NamespaceName: ns,
			Body:          &gen.ProjectRelease{Metadata: gen.ObjectMeta{Name: "new-pr"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectRelease403JSONResponse{}, resp)
	})
}

// --- DeleteProjectRelease Handler ---

func TestDeleteProjectReleaseHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectReleaseService(t, []client.Object{testProjectReleaseObj("pr-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.DeleteProjectRelease(ctx, gen.DeleteProjectReleaseRequestObject{NamespaceName: ns, ProjectReleaseName: "pr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectRelease204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectReleaseService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.DeleteProjectRelease(ctx, gen.DeleteProjectReleaseRequestObject{NamespaceName: ns, ProjectReleaseName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectRelease404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectReleaseService(t, []client.Object{testProjectReleaseObj("pr-1")}, &denyAllPDP{})
		h := newHandlerWithProjectReleaseService(svc)

		resp, err := h.DeleteProjectRelease(ctx, gen.DeleteProjectReleaseRequestObject{NamespaceName: ns, ProjectReleaseName: "pr-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectRelease403JSONResponse{}, resp)
	})
}
