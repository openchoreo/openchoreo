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
	endpointsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

const (
	testEndpointNs      = "test-ns"
	testEndpointProject = "test-project"
)

func newEndpointService(t *testing.T, objects []client.Object, pdp authzcore.PDP) endpointsvc.Service {
	t.Helper()
	// Always include the test project so create/list project-existence checks succeed.
	bootstrap := make([]client.Object, 0, 1+len(objects))
	bootstrap = append(bootstrap,
		&openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: testEndpointProject, Namespace: testEndpointNs},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
			},
		},
	)
	bootstrap = append(bootstrap, objects...)

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(bootstrap...).
		Build()
	return endpointsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithEndpointService(svc endpointsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{EndpointService: svc},
		logger:   slog.Default(),
	}
}

func testEndpointObj(name string) *openchoreov1alpha1.Endpoint {
	return &openchoreov1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testEndpointNs},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner: openchoreov1alpha1.EndpointOwner{ProjectName: testEndpointProject},
			Type: openchoreov1alpha1.EndpointTypeRef{
				Kind: openchoreov1alpha1.EndpointTypeRefKindEndpointType,
				Name: "http-gateway",
			},
		},
	}
}

// --- ListEndpoints Handler ---

func TestListEndpointsHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.ListEndpoints(ctx, gen.ListEndpointsRequestObject{NamespaceName: testEndpointNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpoints200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "r-1", typed.Items[0].Metadata.Name)
	})

	t.Run("filtered by project", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.ListEndpoints(ctx, gen.ListEndpointsRequestObject{
			NamespaceName: testEndpointNs,
			Params:        gen.ListEndpointsParams{Project: ptr.To(testEndpointProject)},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpoints200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("project not found returns 404", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.ListEndpoints(ctx, gen.ListEndpointsRequestObject{
			NamespaceName: testEndpointNs,
			Params:        gen.ListEndpointsParams{Project: ptr.To("missing-project")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListEndpoints404JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.ListEndpoints(ctx, gen.ListEndpointsRequestObject{NamespaceName: testEndpointNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpoints200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.ListEndpoints(ctx, gen.ListEndpointsRequestObject{
			NamespaceName: testEndpointNs,
			Params:        gen.ListEndpointsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListEndpoints400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.ListEndpoints(ctx, gen.ListEndpointsRequestObject{NamespaceName: testEndpointNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListEndpoints200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetEndpoint Handler ---

func TestGetEndpointHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.GetEndpoint(ctx, gen.GetEndpointRequestObject{NamespaceName: testEndpointNs, EndpointName: "r-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetEndpoint200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "r-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.GetEndpoint(ctx, gen.GetEndpointRequestObject{NamespaceName: testEndpointNs, EndpointName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpoint404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.GetEndpoint(ctx, gen.GetEndpointRequestObject{NamespaceName: testEndpointNs, EndpointName: "r-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetEndpoint403JSONResponse{}, resp)
	})
}

// --- CreateEndpoint Handler ---

func TestCreateEndpointHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		body := gen.Endpoint{
			Metadata: gen.ObjectMeta{Name: "new-r"},
			Spec: &gen.EndpointSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testEndpointProject},
				Type: gen.EndpointTypeRef{Name: "http-gateway"},
			},
		}
		resp, err := h.CreateEndpoint(ctx, gen.CreateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateEndpoint201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-r", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.CreateEndpoint(ctx, gen.CreateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpoint400JSONResponse{}, resp)
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		body := gen.Endpoint{
			Metadata: gen.ObjectMeta{Name: "new-r"},
			Spec: &gen.EndpointSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: "missing-project"},
				Type: gen.EndpointTypeRef{Name: "http-gateway"},
			},
		}
		resp, err := h.CreateEndpoint(ctx, gen.CreateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpoint400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testEndpointObj("new-r")
		svc := newEndpointService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		body := gen.Endpoint{
			Metadata: gen.ObjectMeta{Name: "new-r"},
			Spec: &gen.EndpointSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testEndpointProject},
				Type: gen.EndpointTypeRef{Name: "http-gateway"},
			},
		}
		resp, err := h.CreateEndpoint(ctx, gen.CreateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpoint409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointService(t, nil, &denyAllPDP{})
		h := newHandlerWithEndpointService(svc)

		body := gen.Endpoint{
			Metadata: gen.ObjectMeta{Name: "new-r"},
			Spec: &gen.EndpointSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testEndpointProject},
				Type: gen.EndpointTypeRef{Name: "http-gateway"},
			},
		}
		resp, err := h.CreateEndpoint(ctx, gen.CreateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateEndpoint403JSONResponse{}, resp)
	})
}

// --- UpdateEndpoint Handler ---

func TestUpdateEndpointHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		body := gen.Endpoint{
			Metadata: gen.ObjectMeta{Name: "r-1"},
			Spec: &gen.EndpointSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testEndpointProject},
				Type: gen.EndpointTypeRef{Name: "http-gateway"},
			},
		}
		resp, err := h.UpdateEndpoint(ctx, gen.UpdateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			EndpointName:  "r-1",
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateEndpoint200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "r-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.UpdateEndpoint(ctx, gen.UpdateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			EndpointName:  "r-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEndpoint400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		body := gen.Endpoint{
			Metadata: gen.ObjectMeta{Name: "nonexistent"},
			Spec: &gen.EndpointSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testEndpointProject},
				Type: gen.EndpointTypeRef{Name: "http-gateway"},
			},
		}
		resp, err := h.UpdateEndpoint(ctx, gen.UpdateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			EndpointName:  "nonexistent",
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateEndpoint404JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		body := gen.Endpoint{
			Metadata: gen.ObjectMeta{Name: "different-name"},
			Spec: &gen.EndpointSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testEndpointProject},
				Type: gen.EndpointTypeRef{Name: "http-gateway"},
			},
		}
		resp, err := h.UpdateEndpoint(ctx, gen.UpdateEndpointRequestObject{
			NamespaceName: testEndpointNs,
			EndpointName:  "r-1",
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateEndpoint200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "r-1", typed.Metadata.Name)
	})
}

// --- DeleteEndpoint Handler ---

func TestDeleteEndpointHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.DeleteEndpoint(ctx, gen.DeleteEndpointRequestObject{NamespaceName: testEndpointNs, EndpointName: "r-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpoint204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newEndpointService(t, nil, &allowAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.DeleteEndpoint(ctx, gen.DeleteEndpointRequestObject{NamespaceName: testEndpointNs, EndpointName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpoint404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newEndpointService(t, []client.Object{testEndpointObj("r-1")}, &denyAllPDP{})
		h := newHandlerWithEndpointService(svc)

		resp, err := h.DeleteEndpoint(ctx, gen.DeleteEndpointRequestObject{NamespaceName: testEndpointNs, EndpointName: "r-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteEndpoint403JSONResponse{}, resp)
	})
}
