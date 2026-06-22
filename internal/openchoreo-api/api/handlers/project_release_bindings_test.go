// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	projectreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectreleasebinding"
	projectreleasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectreleasebinding/mocks"
)

func newProjectReleaseBindingService(t *testing.T, objects []client.Object, pdp authzcore.PDP) projectreleasebindingsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return projectreleasebindingsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithProjectReleaseBindingService(svc projectreleasebindingsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ProjectReleaseBindingService: svc},
		logger:   slog.Default(),
	}
}

func testProjectReleaseBindingObj(name string) *openchoreov1alpha1.ProjectReleaseBinding {
	return &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: "my-app"},
			Environment: "dev",
		},
	}
}

// --- ListProjectReleaseBindings Handler ---

func TestListProjectReleaseBindingsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectReleaseBindingObj("b-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.ListProjectReleaseBindings(ctx, gen.ListProjectReleaseBindingsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "b-1", typed.Items[0].Metadata.Name)
	})

	t.Run("filter by project", func(t *testing.T) {
		b1 := testProjectReleaseBindingObj("b-1")
		b2 := testProjectReleaseBindingObj("b-2")
		b2.Spec.Owner.ProjectName = "other-app"
		svc := newProjectReleaseBindingService(t, []client.Object{b1, b2}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.ListProjectReleaseBindings(ctx, gen.ListProjectReleaseBindingsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListProjectReleaseBindingsParams{Project: ptr.To("my-app")},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "b-1", typed.Items[0].Metadata.Name)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.ListProjectReleaseBindings(ctx, gen.ListProjectReleaseBindingsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListProjectReleaseBindingsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListProjectReleaseBindings400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectReleaseBindingObj("b-1")}, &denyAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.ListProjectReleaseBindings(ctx, gen.ListProjectReleaseBindingsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListProjectReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetProjectReleaseBinding Handler ---

func TestGetProjectReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectReleaseBindingObj("b-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.GetProjectReleaseBinding(ctx, gen.GetProjectReleaseBindingRequestObject{NamespaceName: ns, ProjectReleaseBindingName: "b-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetProjectReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "b-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.GetProjectReleaseBinding(ctx, gen.GetProjectReleaseBindingRequestObject{NamespaceName: ns, ProjectReleaseBindingName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProjectReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectReleaseBindingObj("b-1")}, &denyAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.GetProjectReleaseBinding(ctx, gen.GetProjectReleaseBindingRequestObject{NamespaceName: ns, ProjectReleaseBindingName: "b-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetProjectReleaseBinding403JSONResponse{}, resp)
	})
}

// --- CreateProjectReleaseBinding Handler ---

func TestCreateProjectReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	newBody := func() *gen.ProjectReleaseBinding {
		return &gen.ProjectReleaseBinding{
			Metadata: gen.ObjectMeta{Name: "b-1"},
			Spec: &gen.ProjectReleaseBindingSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: "my-app"},
				Environment: "dev",
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectObj("my-app")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.CreateProjectReleaseBinding(ctx, gen.CreateProjectReleaseBindingRequestObject{NamespaceName: ns, Body: newBody()})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateProjectReleaseBinding201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "b-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.CreateProjectReleaseBinding(ctx, gen.CreateProjectReleaseBindingRequestObject{NamespaceName: ns, Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("referenced project not found returns 400", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.CreateProjectReleaseBinding(ctx, gen.CreateProjectReleaseBindingRequestObject{NamespaceName: ns, Body: newBody()})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectObj("my-app"), testProjectReleaseBindingObj("b-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.CreateProjectReleaseBinding(ctx, gen.CreateProjectReleaseBindingRequestObject{NamespaceName: ns, Body: newBody()})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectReleaseBinding409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectObj("my-app")}, &denyAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.CreateProjectReleaseBinding(ctx, gen.CreateProjectReleaseBindingRequestObject{NamespaceName: ns, Body: newBody()})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateProjectReleaseBinding403JSONResponse{}, resp)
	})
}

// --- UpdateProjectReleaseBinding Handler ---

func TestUpdateProjectReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	newBody := func(name string) *gen.ProjectReleaseBinding {
		return &gen.ProjectReleaseBinding{
			Metadata: gen.ObjectMeta{Name: name},
			Spec: &gen.ProjectReleaseBindingSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: "my-app"},
				Environment: "dev",
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectReleaseBindingObj("b-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.UpdateProjectReleaseBinding(ctx, gen.UpdateProjectReleaseBindingRequestObject{
			NamespaceName:             ns,
			ProjectReleaseBindingName: "b-1",
			Body:                      newBody("b-1"),
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateProjectReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "b-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.UpdateProjectReleaseBinding(ctx, gen.UpdateProjectReleaseBindingRequestObject{
			NamespaceName:             ns,
			ProjectReleaseBindingName: "b-1",
			Body:                      nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateProjectReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.UpdateProjectReleaseBinding(ctx, gen.UpdateProjectReleaseBindingRequestObject{
			NamespaceName:             ns,
			ProjectReleaseBindingName: "nonexistent",
			Body:                      newBody("nonexistent"),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateProjectReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectReleaseBindingObj("b-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.UpdateProjectReleaseBinding(ctx, gen.UpdateProjectReleaseBindingRequestObject{
			NamespaceName:             ns,
			ProjectReleaseBindingName: "b-1",
			Body:                      newBody("different-name"),
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateProjectReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "b-1", typed.Metadata.Name)
	})
}

// --- DeleteProjectReleaseBinding Handler ---

func TestDeleteProjectReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectReleaseBindingObj("b-1")}, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.DeleteProjectReleaseBinding(ctx, gen.DeleteProjectReleaseBindingRequestObject{NamespaceName: ns, ProjectReleaseBindingName: "b-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectReleaseBinding204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.DeleteProjectReleaseBinding(ctx, gen.DeleteProjectReleaseBindingRequestObject{NamespaceName: ns, ProjectReleaseBindingName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newProjectReleaseBindingService(t, []client.Object{testProjectReleaseBindingObj("b-1")}, &denyAllPDP{})
		h := newHandlerWithProjectReleaseBindingService(svc)

		resp, err := h.DeleteProjectReleaseBinding(ctx, gen.DeleteProjectReleaseBindingRequestObject{NamespaceName: ns, ProjectReleaseBindingName: "b-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteProjectReleaseBinding403JSONResponse{}, resp)
	})
}

// --- Error mapping (mock service) ---

func newProjectReleaseBindingHandlerWithMock(svc *projectreleasebindingmocks.MockService) *Handler {
	return &Handler{
		services: &handlerservices.Services{ProjectReleaseBindingService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func bindingBody(name string) *gen.ProjectReleaseBinding {
	return &gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ProjectReleaseBindingSpec{
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{ProjectName: "my-app"},
			Environment: "dev",
		},
	}
}

func TestListProjectReleaseBindingsHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.ListProjectReleaseBindings400JSONResponse{}},
		{"internal -> 500", errors.New("boom"), gen.ListProjectReleaseBindings500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := projectreleasebindingmocks.NewMockService(t)
			svc.EXPECT().ListProjectReleaseBindings(mock.Anything, "test-ns", "", mock.Anything).Return(nil, tt.svcErr)
			resp, err := newProjectReleaseBindingHandlerWithMock(svc).ListProjectReleaseBindings(ctx, gen.ListProjectReleaseBindingsRequestObject{NamespaceName: "test-ns"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestGetProjectReleaseBindingHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetProjectReleaseBinding403JSONResponse{}},
		{"not found -> 404", projectreleasebindingsvc.ErrProjectReleaseBindingNotFound, gen.GetProjectReleaseBinding404JSONResponse{}},
		{"internal -> 500", errors.New("boom"), gen.GetProjectReleaseBinding500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := projectreleasebindingmocks.NewMockService(t)
			svc.EXPECT().GetProjectReleaseBinding(mock.Anything, "test-ns", "b-1").Return(nil, tt.svcErr)
			resp, err := newProjectReleaseBindingHandlerWithMock(svc).GetProjectReleaseBinding(ctx, gen.GetProjectReleaseBindingRequestObject{NamespaceName: "test-ns", ProjectReleaseBindingName: "b-1"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestCreateProjectReleaseBindingHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.CreateProjectReleaseBinding403JSONResponse{}},
		{"project not found -> 400", projectreleasebindingsvc.ErrProjectNotFound, gen.CreateProjectReleaseBinding400JSONResponse{}},
		{"already exists -> 409", projectreleasebindingsvc.ErrProjectReleaseBindingAlreadyExists, gen.CreateProjectReleaseBinding409JSONResponse{}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.CreateProjectReleaseBinding400JSONResponse{}},
		{"internal -> 500", errors.New("boom"), gen.CreateProjectReleaseBinding500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := projectreleasebindingmocks.NewMockService(t)
			svc.EXPECT().CreateProjectReleaseBinding(mock.Anything, "test-ns", mock.Anything).Return(nil, tt.svcErr)
			resp, err := newProjectReleaseBindingHandlerWithMock(svc).CreateProjectReleaseBinding(ctx, gen.CreateProjectReleaseBindingRequestObject{
				NamespaceName: "test-ns",
				Body:          bindingBody("b-1"),
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateProjectReleaseBindingHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateProjectReleaseBinding403JSONResponse{}},
		{"not found -> 404", projectreleasebindingsvc.ErrProjectReleaseBindingNotFound, gen.UpdateProjectReleaseBinding404JSONResponse{}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.UpdateProjectReleaseBinding400JSONResponse{}},
		{"internal -> 500", errors.New("boom"), gen.UpdateProjectReleaseBinding500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := projectreleasebindingmocks.NewMockService(t)
			svc.EXPECT().UpdateProjectReleaseBinding(mock.Anything, "test-ns", mock.Anything).Return(nil, tt.svcErr)
			resp, err := newProjectReleaseBindingHandlerWithMock(svc).UpdateProjectReleaseBinding(ctx, gen.UpdateProjectReleaseBindingRequestObject{
				NamespaceName:             "test-ns",
				ProjectReleaseBindingName: "b-1",
				Body:                      bindingBody("b-1"),
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestDeleteProjectReleaseBindingHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.DeleteProjectReleaseBinding403JSONResponse{}},
		{"not found -> 404", projectreleasebindingsvc.ErrProjectReleaseBindingNotFound, gen.DeleteProjectReleaseBinding404JSONResponse{}},
		{"internal -> 500", errors.New("boom"), gen.DeleteProjectReleaseBinding500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := projectreleasebindingmocks.NewMockService(t)
			svc.EXPECT().DeleteProjectReleaseBinding(mock.Anything, "test-ns", "b-1").Return(tt.svcErr)
			resp, err := newProjectReleaseBindingHandlerWithMock(svc).DeleteProjectReleaseBinding(ctx, gen.DeleteProjectReleaseBindingRequestObject{NamespaceName: "test-ns", ProjectReleaseBindingName: "b-1"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}
