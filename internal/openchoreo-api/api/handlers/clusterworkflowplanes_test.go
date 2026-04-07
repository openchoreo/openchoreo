// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterworkflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflowplane"
	cwpmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflowplane/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newClusterWorkflowPlaneService(
	t *testing.T, objects []client.Object, pdp authzcore.PDP,
) clusterworkflowplanesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clusterworkflowplanesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithClusterWorkflowPlaneService(svc clusterworkflowplanesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterWorkflowPlaneService: svc},
		logger:   slog.Default(),
	}
}

func testClusterWorkflowPlaneObj(name string) *openchoreov1alpha1.ClusterWorkflowPlane {
	return &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID: name,
		},
	}
}

// --- ListClusterWorkflowPlanes Handler ---

func TestListClusterWorkflowPlanesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, []client.Object{testClusterWorkflowPlaneObj("cwp-1")}, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.ListClusterWorkflowPlanes(ctx, gen.ListClusterWorkflowPlanesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterWorkflowPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.ListClusterWorkflowPlanes(ctx, gen.ListClusterWorkflowPlanesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterWorkflowPlanes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetClusterWorkflowPlane Handler ---

func TestGetClusterWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.GetClusterWorkflowPlane(ctx, gen.GetClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetClusterWorkflowPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.GetClusterWorkflowPlane(ctx, gen.GetClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.GetClusterWorkflowPlane(ctx, gen.GetClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterWorkflowPlane403JSONResponse{}, resp)
	})
}

// --- CreateClusterWorkflowPlane Handler ---

func TestCreateClusterWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{
			Body: &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "new-cwp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflowPlane201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflowPlane400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("new-cwp")}
		svc := newClusterWorkflowPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{
			Body: &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "new-cwp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflowPlane409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{
			Body: &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "new-cwp"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterWorkflowPlane403JSONResponse{}, resp)
	})
}

// --- UpdateClusterWorkflowPlane Handler ---

func TestUpdateClusterWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
			Body:                     &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "cwp-1"}},
		})
		require.NoError(t, err)
		_, ok := resp.(gen.UpdateClusterWorkflowPlane200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
			Body:                     nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterWorkflowPlane400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "nonexistent",
			Body:                     &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
			Body:                     &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "cwp-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterWorkflowPlane403JSONResponse{}, resp)
	})
}

// --- DeleteClusterWorkflowPlane Handler ---

func TestDeleteClusterWorkflowPlaneHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.DeleteClusterWorkflowPlane(ctx, gen.DeleteClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflowPlane204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterWorkflowPlaneService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.DeleteClusterWorkflowPlane(ctx, gen.DeleteClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflowPlane404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		objs := []client.Object{testClusterWorkflowPlaneObj("cwp-1")}
		svc := newClusterWorkflowPlaneService(t, objs, &denyAllPDP{})
		h := newHandlerWithClusterWorkflowPlaneService(svc)

		resp, err := h.DeleteClusterWorkflowPlane(ctx, gen.DeleteClusterWorkflowPlaneRequestObject{
			ClusterWorkflowPlaneName: "cwp-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflowPlane403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := cwpmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterWorkflowPlane(mock.Anything, "cwp-1").Return(errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{ClusterWorkflowPlaneService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.DeleteClusterWorkflowPlane(ctx, gen.DeleteClusterWorkflowPlaneRequestObject{ClusterWorkflowPlaneName: "cwp-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterWorkflowPlane500JSONResponse{}, resp)
	})
}

// --- Additional error mapping tests using mocks ---

func TestListClusterWorkflowPlanesHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := cwpmocks.NewMockService(t)
	svc.EXPECT().ListClusterWorkflowPlanes(mock.Anything, mock.Anything).Return(nil, errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{ClusterWorkflowPlaneService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.ListClusterWorkflowPlanes(ctx, gen.ListClusterWorkflowPlanesRequestObject{})
	require.NoError(t, err)
	assert.IsType(t, gen.ListClusterWorkflowPlanes500JSONResponse{}, resp)
}

func TestGetClusterWorkflowPlaneHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := cwpmocks.NewMockService(t)
	svc.EXPECT().GetClusterWorkflowPlane(mock.Anything, "cwp-1").Return(nil, errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{ClusterWorkflowPlaneService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.GetClusterWorkflowPlane(ctx, gen.GetClusterWorkflowPlaneRequestObject{ClusterWorkflowPlaneName: "cwp-1"})
	require.NoError(t, err)
	assert.IsType(t, gen.GetClusterWorkflowPlane500JSONResponse{}, resp)
}

func TestCreateClusterWorkflowPlaneHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	body := &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "new-cwp"}}

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.CreateClusterWorkflowPlane400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.CreateClusterWorkflowPlane500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := cwpmocks.NewMockService(t)
			svc.EXPECT().CreateClusterWorkflowPlane(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterWorkflowPlaneService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.CreateClusterWorkflowPlane(ctx, gen.CreateClusterWorkflowPlaneRequestObject{Body: body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateClusterWorkflowPlaneHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	body := &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "cwp-1"}}

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.UpdateClusterWorkflowPlane400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.UpdateClusterWorkflowPlane500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := cwpmocks.NewMockService(t)
			svc.EXPECT().UpdateClusterWorkflowPlane(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterWorkflowPlaneService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
				ClusterWorkflowPlaneName: "cwp-1",
				Body:                     body,
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateClusterWorkflowPlaneHandler_UsesPathName(t *testing.T) {
	ctx := testContext()
	svc := cwpmocks.NewMockService(t)
	svc.EXPECT().UpdateClusterWorkflowPlane(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, cwp *openchoreov1alpha1.ClusterWorkflowPlane) (*openchoreov1alpha1.ClusterWorkflowPlane, error) {
		assert.Equal(t, "from-path", cwp.Name)
		return cwp, nil
	})
	h := &Handler{
		services: &handlerservices.Services{ClusterWorkflowPlaneService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "from-body"}}
	resp, err := h.UpdateClusterWorkflowPlane(ctx, gen.UpdateClusterWorkflowPlaneRequestObject{
		ClusterWorkflowPlaneName: "from-path",
		Body:                     &body,
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.UpdateClusterWorkflowPlane200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	assert.Equal(t, "from-path", typed.Metadata.Name)
}
