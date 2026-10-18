// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterendpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterendpointtype"
	cetsvcmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterendpointtype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newClusterEndpointTypeService(t *testing.T, objects []client.Object, pdp authzcore.PDP) clusterendpointtypesvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clusterendpointtypesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithClusterEndpointTypeService(cetSvc clusterendpointtypesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterEndpointTypeService: cetSvc},
		logger:   slog.Default(),
	}
}

func newCETFixture() *openchoreov1alpha1.ClusterEndpointType {
	return &openchoreov1alpha1.ClusterEndpointType{
		ObjectMeta: metav1.ObjectMeta{Name: "http-gateway"},
		Spec: openchoreov1alpha1.ClusterEndpointTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTemplate{{
				ID:       "route",
				Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"x"}}`)},
			}},
		},
	}
}

func TestListClusterEndpointTypesHandler(t *testing.T) {
	ctx := testContext()
	crt := newCETFixture()

	t.Run("returns items when authorized", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, []client.Object{crt}, &allowAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)

		resp, err := h.ListClusterEndpointTypes(ctx, gen.ListClusterEndpointTypesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterEndpointTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Len(t, typed.Items, 1)
	})

	t.Run("filters unauthorized items", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, []client.Object{crt}, &denyAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)

		resp, err := h.ListClusterEndpointTypes(ctx, gen.ListClusterEndpointTypesRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterEndpointTypes200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

func TestListClusterEndpointTypesHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.ListClusterEndpointTypes400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.ListClusterEndpointTypes500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := cetsvcmocks.NewMockService(t)
			svc.EXPECT().ListClusterEndpointTypes(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterEndpointTypeService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.ListClusterEndpointTypes(ctx, gen.ListClusterEndpointTypesRequestObject{})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestGetClusterEndpointTypeHandler(t *testing.T) {
	ctx := testContext()
	crt := newCETFixture()

	t.Run("success", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, []client.Object{crt}, &allowAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.GetClusterEndpointType(ctx, gen.GetClusterEndpointTypeRequestObject{CetName: "http-gateway"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterEndpointType200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "http-gateway", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.GetClusterEndpointType(ctx, gen.GetClusterEndpointTypeRequestObject{CetName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterEndpointType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, []client.Object{crt}, &denyAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.GetClusterEndpointType(ctx, gen.GetClusterEndpointTypeRequestObject{CetName: "http-gateway"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterEndpointType403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := cetsvcmocks.NewMockService(t)
		svc.EXPECT().GetClusterEndpointType(mock.Anything, "http-gateway").Return(nil, errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{ClusterEndpointTypeService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.GetClusterEndpointType(ctx, gen.GetClusterEndpointTypeRequestObject{CetName: "http-gateway"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterEndpointType500JSONResponse{}, resp)
	})
}

func TestGetClusterEndpointTypeSchemaHandler(t *testing.T) {
	ctx := testContext()
	paramsRaw, _ := json.Marshal(map[string]any{"version": "string"})
	crt := newCETFixture()
	crt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
	}

	t.Run("returns schema when authorized", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, []client.Object{crt}, &allowAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.GetClusterEndpointTypeSchema(ctx, gen.GetClusterEndpointTypeSchemaRequestObject{CetName: "http-gateway"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterEndpointTypeSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.NotEmpty(t, typed)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.GetClusterEndpointTypeSchema(ctx, gen.GetClusterEndpointTypeSchemaRequestObject{CetName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterEndpointTypeSchema404JSONResponse{}, resp)
	})

	t.Run("returns 403 when forbidden", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, []client.Object{crt}, &denyAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.GetClusterEndpointTypeSchema(ctx, gen.GetClusterEndpointTypeSchemaRequestObject{CetName: "http-gateway"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterEndpointTypeSchema403JSONResponse{}, resp)
	})
}

func TestCreateClusterEndpointTypeHandler_NilBody(t *testing.T) {
	ctx := testContext()
	h := &Handler{
		services: &handlerservices.Services{ClusterEndpointTypeService: cetsvcmocks.NewMockService(t)},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.CreateClusterEndpointType(ctx, gen.CreateClusterEndpointTypeRequestObject{Body: nil})
	require.NoError(t, err)
	assert.IsType(t, gen.CreateClusterEndpointType400JSONResponse{}, resp)
}

func TestCreateClusterEndpointTypeHandler_Success(t *testing.T) {
	ctx := testContext()
	svc := cetsvcmocks.NewMockService(t)
	svc.EXPECT().CreateClusterEndpointType(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, crt *openchoreov1alpha1.ClusterEndpointType) (*openchoreov1alpha1.ClusterEndpointType, error) {
		return crt, nil
	})
	h := &Handler{
		services: &handlerservices.Services{ClusterEndpointTypeService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := gen.ClusterEndpointType{Metadata: gen.ObjectMeta{Name: "crt"}}
	resp, err := h.CreateClusterEndpointType(ctx, gen.CreateClusterEndpointTypeRequestObject{Body: &body})
	require.NoError(t, err)
	typed, ok := resp.(gen.CreateClusterEndpointType201JSONResponse)
	require.True(t, ok, "expected 201 response, got %T", resp)
	assert.Equal(t, "crt", typed.Metadata.Name)
}

func TestCreateClusterEndpointTypeHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.CreateClusterEndpointType403JSONResponse{}},
		{"already exists -> 409", clusterendpointtypesvc.ErrClusterEndpointTypeAlreadyExists, gen.CreateClusterEndpointType409JSONResponse{}},
		{"validation error -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.CreateClusterEndpointType400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.CreateClusterEndpointType500JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := cetsvcmocks.NewMockService(t)
			svc.EXPECT().CreateClusterEndpointType(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterEndpointTypeService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			body := gen.ClusterEndpointType{Metadata: gen.ObjectMeta{Name: "crt"}}
			resp, err := h.CreateClusterEndpointType(ctx, gen.CreateClusterEndpointTypeRequestObject{Body: &body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestUpdateClusterEndpointTypeHandler_UsesPathName(t *testing.T) {
	ctx := testContext()
	svc := cetsvcmocks.NewMockService(t)
	svc.EXPECT().UpdateClusterEndpointType(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, crt *openchoreov1alpha1.ClusterEndpointType) (*openchoreov1alpha1.ClusterEndpointType, error) {
		assert.Equal(t, "from-path", crt.Name)
		return crt, nil
	})
	h := &Handler{
		services: &handlerservices.Services{ClusterEndpointTypeService: svc},
		logger:   slog.Default(),
	}

	body := gen.ClusterEndpointType{Metadata: gen.ObjectMeta{Name: "from-body"}}
	resp, err := h.UpdateClusterEndpointType(ctx, gen.UpdateClusterEndpointTypeRequestObject{
		CetName: "from-path",
		Body:    &body,
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.UpdateClusterEndpointType200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	assert.Equal(t, "from-path", typed.Metadata.Name)
}

func TestUpdateClusterEndpointTypeHandler_NilBody(t *testing.T) {
	ctx := testContext()
	h := &Handler{
		services: &handlerservices.Services{ClusterEndpointTypeService: cetsvcmocks.NewMockService(t)},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.UpdateClusterEndpointType(ctx, gen.UpdateClusterEndpointTypeRequestObject{CetName: "crt", Body: nil})
	require.NoError(t, err)
	assert.IsType(t, gen.UpdateClusterEndpointType400JSONResponse{}, resp)
}

func TestUpdateClusterEndpointTypeHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateClusterEndpointType403JSONResponse{}},
		{"not found -> 404", clusterendpointtypesvc.ErrClusterEndpointTypeNotFound, gen.UpdateClusterEndpointType404JSONResponse{}},
		{"validation error -> 400", &svcpkg.ValidationError{Msg: "invalid spec"}, gen.UpdateClusterEndpointType400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.UpdateClusterEndpointType500JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := cetsvcmocks.NewMockService(t)
			svc.EXPECT().UpdateClusterEndpointType(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{ClusterEndpointTypeService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			body := gen.ClusterEndpointType{Metadata: gen.ObjectMeta{Name: "crt"}}
			resp, err := h.UpdateClusterEndpointType(ctx, gen.UpdateClusterEndpointTypeRequestObject{CetName: "crt", Body: &body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestDeleteClusterEndpointTypeHandler(t *testing.T) {
	ctx := testContext()
	crt := newCETFixture()

	t.Run("success", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, []client.Object{crt}, &allowAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.DeleteClusterEndpointType(ctx, gen.DeleteClusterEndpointTypeRequestObject{CetName: "http-gateway"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterEndpointType204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, nil, &allowAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.DeleteClusterEndpointType(ctx, gen.DeleteClusterEndpointTypeRequestObject{CetName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterEndpointType404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newClusterEndpointTypeService(t, []client.Object{crt}, &denyAllPDP{})
		h := newHandlerWithClusterEndpointTypeService(svc)
		resp, err := h.DeleteClusterEndpointType(ctx, gen.DeleteClusterEndpointTypeRequestObject{CetName: "http-gateway"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterEndpointType403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := cetsvcmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterEndpointType(mock.Anything, "http-gateway").Return(errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{ClusterEndpointTypeService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.DeleteClusterEndpointType(ctx, gen.DeleteClusterEndpointTypeRequestObject{CetName: "http-gateway"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterEndpointType500JSONResponse{}, resp)
	})
}
