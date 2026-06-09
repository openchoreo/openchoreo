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
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
	workloadmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload/mocks"
)

func newWorkloadService(t *testing.T, objects []client.Object, pdp authzcore.PDP) workloadsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return workloadsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithWorkloadService(svc workloadsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{WorkloadService: svc},
		logger:   slog.Default(),
	}
}

func testWorkloadObj(name string) *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
	}
}

// --- ListWorkloads Handler ---

func TestListWorkloadsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.ListWorkloads(ctx, gen.ListWorkloadsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListWorkloads200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "wl-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newWorkloadService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.ListWorkloads(ctx, gen.ListWorkloadsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListWorkloads200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newWorkloadService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.ListWorkloads(ctx, gen.ListWorkloadsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListWorkloadsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListWorkloads400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &denyAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.ListWorkloads(ctx, gen.ListWorkloadsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListWorkloads200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetWorkload Handler ---

func TestGetWorkloadHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.GetWorkload(ctx, gen.GetWorkloadRequestObject{NamespaceName: ns, WorkloadName: "wl-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetWorkload200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "wl-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkloadService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.GetWorkload(ctx, gen.GetWorkloadRequestObject{NamespaceName: ns, WorkloadName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkload404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &denyAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.GetWorkload(ctx, gen.GetWorkloadRequestObject{NamespaceName: ns, WorkloadName: "wl-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkload403JSONResponse{}, resp)
	})
}

// --- CreateWorkload Handler ---

func testComponentForWorkload() *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: "my-proj"},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
		},
	}
}

func newCreateWorkloadBody() *gen.Workload {
	return &gen.Workload{
		Metadata: gen.ObjectMeta{Name: "new-wl"},
		Spec: &gen.WorkloadSpec{
			Owner: &struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ProjectName:   "my-proj",
				ComponentName: "my-comp",
			},
		},
	}
}

func TestCreateWorkloadHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	comp := testComponentForWorkload()

	t.Run("success", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{comp}, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.CreateWorkload(ctx, gen.CreateWorkloadRequestObject{
			NamespaceName: ns,
			Body:          newCreateWorkloadBody(),
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateWorkload201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-wl", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newWorkloadService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.CreateWorkload(ctx, gen.CreateWorkloadRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkload400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testWorkloadObj("new-wl")
		existing.Spec.Owner = openchoreov1alpha1.WorkloadOwner{
			ProjectName:   "my-proj",
			ComponentName: "my-comp",
		}
		svc := newWorkloadService(t, []client.Object{comp, existing}, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.CreateWorkload(ctx, gen.CreateWorkloadRequestObject{
			NamespaceName: ns,
			Body:          newCreateWorkloadBody(),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkload409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{comp}, &denyAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.CreateWorkload(ctx, gen.CreateWorkloadRequestObject{
			NamespaceName: ns,
			Body:          newCreateWorkloadBody(),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkload403JSONResponse{}, resp)
	})
}

// --- UpdateWorkload Handler ---

func TestUpdateWorkloadHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.UpdateWorkload(ctx, gen.UpdateWorkloadRequestObject{
			NamespaceName: ns,
			WorkloadName:  "wl-1",
			Body:          &gen.Workload{Metadata: gen.ObjectMeta{Name: "wl-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateWorkload200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "wl-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newWorkloadService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.UpdateWorkload(ctx, gen.UpdateWorkloadRequestObject{
			NamespaceName: ns,
			WorkloadName:  "wl-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateWorkload400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkloadService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.UpdateWorkload(ctx, gen.UpdateWorkloadRequestObject{
			NamespaceName: ns,
			WorkloadName:  "nonexistent",
			Body:          &gen.Workload{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateWorkload404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &denyAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.UpdateWorkload(ctx, gen.UpdateWorkloadRequestObject{
			NamespaceName: ns,
			WorkloadName:  "wl-1",
			Body:          &gen.Workload{Metadata: gen.ObjectMeta{Name: "wl-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateWorkload403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.UpdateWorkload(ctx, gen.UpdateWorkloadRequestObject{
			NamespaceName: ns,
			WorkloadName:  "wl-1",
			Body:          &gen.Workload{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateWorkload200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "wl-1", typed.Metadata.Name)
	})
}

// --- DeleteWorkload Handler ---

func TestDeleteWorkloadHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.DeleteWorkload(ctx, gen.DeleteWorkloadRequestObject{NamespaceName: ns, WorkloadName: "wl-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkload204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkloadService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.DeleteWorkload(ctx, gen.DeleteWorkloadRequestObject{NamespaceName: ns, WorkloadName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkload404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkloadService(t, []client.Object{testWorkloadObj("wl-1")}, &denyAllPDP{})
		h := newHandlerWithWorkloadService(svc)

		resp, err := h.DeleteWorkload(ctx, gen.DeleteWorkloadRequestObject{NamespaceName: ns, WorkloadName: "wl-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkload403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := workloadmocks.NewMockService(t)
		svc.EXPECT().DeleteWorkload(mock.Anything, ns, "wl-1").Return(errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{WorkloadService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.DeleteWorkload(ctx, gen.DeleteWorkloadRequestObject{NamespaceName: ns, WorkloadName: "wl-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkload500JSONResponse{}, resp)
	})
}

// --- Additional error mapping tests using mocks ---

func TestListWorkloadsHandler_ErrorMapping(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"
	t.Run("internal error returns 500", func(t *testing.T) {
		svc := workloadmocks.NewMockService(t)
		svc.EXPECT().ListWorkloads(mock.Anything, ns, "", mock.Anything).Return(nil, errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{WorkloadService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.ListWorkloads(ctx, gen.ListWorkloadsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		assert.IsType(t, gen.ListWorkloads500JSONResponse{}, resp)
	})

	t.Run("with component filter forwards componentName", func(t *testing.T) {
		svc := workloadmocks.NewMockService(t)
		svc.EXPECT().ListWorkloads(mock.Anything, ns, "comp-a", mock.Anything).Return(&svcpkg.ListResult[openchoreov1alpha1.Workload]{Items: nil}, nil)
		h := &Handler{
			services: &handlerservices.Services{WorkloadService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		comp := "comp-a"
		resp, err := h.ListWorkloads(ctx, gen.ListWorkloadsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListWorkloadsParams{Component: &comp},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListWorkloads200JSONResponse{}, resp)
	})
}

func TestGetWorkloadHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := workloadmocks.NewMockService(t)
	svc.EXPECT().GetWorkload(mock.Anything, "test-ns", "wl-1").Return(nil, errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{WorkloadService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.GetWorkload(ctx, gen.GetWorkloadRequestObject{NamespaceName: "test-ns", WorkloadName: "wl-1"})
	require.NoError(t, err)
	assert.IsType(t, gen.GetWorkload500JSONResponse{}, resp)
}

func TestCreateWorkloadHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"
	body := newCreateWorkloadBody()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"component not found -> 400", workloadsvc.ErrComponentNotFound, gen.CreateWorkload400JSONResponse{}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.CreateWorkload400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.CreateWorkload500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workloadmocks.NewMockService(t)
			svc.EXPECT().CreateWorkload(mock.Anything, ns, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkloadService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.CreateWorkload(ctx, gen.CreateWorkloadRequestObject{NamespaceName: ns, Body: body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateWorkloadHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"
	body := &gen.Workload{Metadata: gen.ObjectMeta{Name: "wl-1"}}

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.UpdateWorkload400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.UpdateWorkload500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workloadmocks.NewMockService(t)
			svc.EXPECT().UpdateWorkload(mock.Anything, ns, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkloadService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.UpdateWorkload(ctx, gen.UpdateWorkloadRequestObject{NamespaceName: ns, WorkloadName: "wl-1", Body: body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}
