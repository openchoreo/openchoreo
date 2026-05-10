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
	resourcesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resource"
)

const (
	testResourceNs      = "test-ns"
	testResourceProject = "test-project"
)

func newResourceService(t *testing.T, objects []client.Object, pdp authzcore.PDP) resourcesvc.Service {
	t.Helper()
	// Always include the test project so create/list project-existence checks succeed.
	bootstrap := make([]client.Object, 0, 1+len(objects))
	bootstrap = append(bootstrap,
		&openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: testResourceProject, Namespace: testResourceNs},
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
	return resourcesvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithResourceService(svc resourcesvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ResourceService: svc},
		logger:   slog.Default(),
	}
}

func testResourceObj(name string) *openchoreov1alpha1.Resource {
	return &openchoreov1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testResourceNs},
		Spec: openchoreov1alpha1.ResourceSpec{
			Owner: openchoreov1alpha1.ResourceOwner{ProjectName: testResourceProject},
			Type: openchoreov1alpha1.ResourceTypeRef{
				Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
				Name: "mysql",
			},
		},
	}
}

// --- ListResources Handler ---

func TestListResourcesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.ListResources(ctx, gen.ListResourcesRequestObject{NamespaceName: testResourceNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResources200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "r-1", typed.Items[0].Metadata.Name)
	})

	t.Run("filtered by project", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.ListResources(ctx, gen.ListResourcesRequestObject{
			NamespaceName: testResourceNs,
			Params:        gen.ListResourcesParams{Project: ptr.To(testResourceProject)},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResources200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("project not found returns 404", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.ListResources(ctx, gen.ListResourcesRequestObject{
			NamespaceName: testResourceNs,
			Params:        gen.ListResourcesParams{Project: ptr.To("missing-project")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListResources404JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.ListResources(ctx, gen.ListResourcesRequestObject{NamespaceName: testResourceNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResources200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.ListResources(ctx, gen.ListResourcesRequestObject{
			NamespaceName: testResourceNs,
			Params:        gen.ListResourcesParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListResources400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &denyAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.ListResources(ctx, gen.ListResourcesRequestObject{NamespaceName: testResourceNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResources200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetResource Handler ---

func TestGetResourceHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.GetResource(ctx, gen.GetResourceRequestObject{NamespaceName: testResourceNs, ResourceName: "r-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetResource200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "r-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.GetResource(ctx, gen.GetResourceRequestObject{NamespaceName: testResourceNs, ResourceName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResource404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &denyAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.GetResource(ctx, gen.GetResourceRequestObject{NamespaceName: testResourceNs, ResourceName: "r-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResource403JSONResponse{}, resp)
	})
}

// --- CreateResource Handler ---

func TestCreateResourceHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		body := gen.ResourceInstance{
			Metadata: gen.ObjectMeta{Name: "new-r"},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testResourceProject},
				Type: gen.ResourceTypeRef{Name: "mysql"},
			},
		}
		resp, err := h.CreateResource(ctx, gen.CreateResourceRequestObject{
			NamespaceName: testResourceNs,
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateResource201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-r", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.CreateResource(ctx, gen.CreateResourceRequestObject{
			NamespaceName: testResourceNs,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResource400JSONResponse{}, resp)
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		body := gen.ResourceInstance{
			Metadata: gen.ObjectMeta{Name: "new-r"},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: "missing-project"},
				Type: gen.ResourceTypeRef{Name: "mysql"},
			},
		}
		resp, err := h.CreateResource(ctx, gen.CreateResourceRequestObject{
			NamespaceName: testResourceNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResource400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testResourceObj("new-r")
		svc := newResourceService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		body := gen.ResourceInstance{
			Metadata: gen.ObjectMeta{Name: "new-r"},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testResourceProject},
				Type: gen.ResourceTypeRef{Name: "mysql"},
			},
		}
		resp, err := h.CreateResource(ctx, gen.CreateResourceRequestObject{
			NamespaceName: testResourceNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResource409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceService(t, nil, &denyAllPDP{})
		h := newHandlerWithResourceService(svc)

		body := gen.ResourceInstance{
			Metadata: gen.ObjectMeta{Name: "new-r"},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testResourceProject},
				Type: gen.ResourceTypeRef{Name: "mysql"},
			},
		}
		resp, err := h.CreateResource(ctx, gen.CreateResourceRequestObject{
			NamespaceName: testResourceNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResource403JSONResponse{}, resp)
	})
}

// --- UpdateResource Handler ---

func TestUpdateResourceHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		body := gen.ResourceInstance{
			Metadata: gen.ObjectMeta{Name: "r-1"},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testResourceProject},
				Type: gen.ResourceTypeRef{Name: "mysql"},
			},
		}
		resp, err := h.UpdateResource(ctx, gen.UpdateResourceRequestObject{
			NamespaceName: testResourceNs,
			ResourceName:  "r-1",
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateResource200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "r-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.UpdateResource(ctx, gen.UpdateResourceRequestObject{
			NamespaceName: testResourceNs,
			ResourceName:  "r-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateResource400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		body := gen.ResourceInstance{
			Metadata: gen.ObjectMeta{Name: "nonexistent"},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testResourceProject},
				Type: gen.ResourceTypeRef{Name: "mysql"},
			},
		}
		resp, err := h.UpdateResource(ctx, gen.UpdateResourceRequestObject{
			NamespaceName: testResourceNs,
			ResourceName:  "nonexistent",
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateResource404JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		body := gen.ResourceInstance{
			Metadata: gen.ObjectMeta{Name: "different-name"},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testResourceProject},
				Type: gen.ResourceTypeRef{Name: "mysql"},
			},
		}
		resp, err := h.UpdateResource(ctx, gen.UpdateResourceRequestObject{
			NamespaceName: testResourceNs,
			ResourceName:  "r-1",
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateResource200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "r-1", typed.Metadata.Name)
	})
}

// --- DeleteResource Handler ---

func TestDeleteResourceHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.DeleteResource(ctx, gen.DeleteResourceRequestObject{NamespaceName: testResourceNs, ResourceName: "r-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResource204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.DeleteResource(ctx, gen.DeleteResourceRequestObject{NamespaceName: testResourceNs, ResourceName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResource404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceService(t, []client.Object{testResourceObj("r-1")}, &denyAllPDP{})
		h := newHandlerWithResourceService(svc)

		resp, err := h.DeleteResource(ctx, gen.DeleteResourceRequestObject{NamespaceName: testResourceNs, ResourceName: "r-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResource403JSONResponse{}, resp)
	})
}
