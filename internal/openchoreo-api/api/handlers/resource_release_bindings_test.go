// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

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
	resourcereleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
)

const (
	testRRBNs       = "test-ns"
	testRRBProject  = "test-project"
	testRRBResource = "test-r"
)

func newResourceReleaseBindingService(t *testing.T, objects []client.Object, pdp authzcore.PDP) resourcereleasebindingsvc.Service {
	t.Helper()
	// Bootstrap with a Resource so create-time validation passes.
	bootstrap := make([]client.Object, 0, 1+len(objects))
	bootstrap = append(bootstrap, &openchoreov1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{Name: testRRBResource, Namespace: testRRBNs},
		Spec: openchoreov1alpha1.ResourceSpec{
			Owner: openchoreov1alpha1.ResourceOwner{ProjectName: testRRBProject},
			Type: openchoreov1alpha1.ResourceTypeRef{
				Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
				Name: "mysql",
			},
		},
	})
	bootstrap = append(bootstrap, objects...)

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(bootstrap...).
		Build()
	return resourcereleasebindingsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithResourceReleaseBindingService(svc resourcereleasebindingsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ResourceReleaseBindingService: svc},
		logger:   slog.Default(),
	}
}

func testResourceReleaseBindingObj(name string) *openchoreov1alpha1.ResourceReleaseBinding {
	return &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testRRBNs},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
				ProjectName:  testRRBProject,
				ResourceName: testRRBResource,
			},
			Environment: "dev",
		},
	}
}

// --- ListResourceReleaseBindings Handler ---

func TestListResourceReleaseBindingsHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success - returns items", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.ListResourceReleaseBindings(ctx, gen.ListResourceReleaseBindingsRequestObject{NamespaceName: testRRBNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("filtered by resource", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.ListResourceReleaseBindings(ctx, gen.ListResourceReleaseBindingsRequestObject{
			NamespaceName: testRRBNs,
			Params:        gen.ListResourceReleaseBindingsParams{Resource: ptr.To(testRRBResource)},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.ListResourceReleaseBindings(ctx, gen.ListResourceReleaseBindingsRequestObject{
			NamespaceName: testRRBNs,
			Params:        gen.ListResourceReleaseBindingsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListResourceReleaseBindings400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.ListResourceReleaseBindings(ctx, gen.ListResourceReleaseBindingsRequestObject{NamespaceName: testRRBNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.ListResourceReleaseBindings(ctx, gen.ListResourceReleaseBindingsRequestObject{NamespaceName: testRRBNs})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListResourceReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetResourceReleaseBinding Handler ---

func TestGetResourceReleaseBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.GetResourceReleaseBinding(ctx, gen.GetResourceReleaseBindingRequestObject{NamespaceName: testRRBNs, ResourceReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetResourceReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rb-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.GetResourceReleaseBinding(ctx, gen.GetResourceReleaseBindingRequestObject{NamespaceName: testRRBNs, ResourceReleaseBindingName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResourceReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.GetResourceReleaseBinding(ctx, gen.GetResourceReleaseBindingRequestObject{NamespaceName: testRRBNs, ResourceReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetResourceReleaseBinding403JSONResponse{}, resp)
	})
}

// --- CreateResourceReleaseBinding Handler ---

func newRRBCreateBody(name string) gen.ResourceReleaseBinding {
	return gen.ResourceReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ResourceReleaseBindingSpec{
			Owner: struct {
				ProjectName  string `json:"projectName"`
				ResourceName string `json:"resourceName"`
			}{ProjectName: testRRBProject, ResourceName: testRRBResource},
			Environment: "dev",
		},
	}
}

func TestCreateResourceReleaseBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		body := newRRBCreateBody("rb-new")
		resp, err := h.CreateResourceReleaseBinding(ctx, gen.CreateResourceReleaseBindingRequestObject{
			NamespaceName: testRRBNs,
			Body:          &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateResourceReleaseBinding201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "rb-new", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.CreateResourceReleaseBinding(ctx, gen.CreateResourceReleaseBindingRequestObject{NamespaceName: testRRBNs, Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("missing resource returns 400", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		body := newRRBCreateBody("rb-new")
		body.Spec.Owner.ResourceName = "missing-r"
		resp, err := h.CreateResourceReleaseBinding(ctx, gen.CreateResourceReleaseBindingRequestObject{
			NamespaceName: testRRBNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testResourceReleaseBindingObj("rb-new")
		svc := newResourceReleaseBindingService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		body := newRRBCreateBody("rb-new")
		resp, err := h.CreateResourceReleaseBinding(ctx, gen.CreateResourceReleaseBindingRequestObject{
			NamespaceName: testRRBNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceReleaseBinding409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &denyAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		body := newRRBCreateBody("rb-new")
		resp, err := h.CreateResourceReleaseBinding(ctx, gen.CreateResourceReleaseBindingRequestObject{
			NamespaceName: testRRBNs,
			Body:          &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateResourceReleaseBinding403JSONResponse{}, resp)
	})
}

// --- UpdateResourceReleaseBinding Handler ---

func TestUpdateResourceReleaseBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		body := newRRBCreateBody("rb-1")
		resp, err := h.UpdateResourceReleaseBinding(ctx, gen.UpdateResourceReleaseBindingRequestObject{
			NamespaceName:              testRRBNs,
			ResourceReleaseBindingName: "rb-1",
			Body:                       &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateResourceReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rb-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.UpdateResourceReleaseBinding(ctx, gen.UpdateResourceReleaseBindingRequestObject{
			NamespaceName:              testRRBNs,
			ResourceReleaseBindingName: "rb-1",
			Body:                       nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateResourceReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		body := newRRBCreateBody("nonexistent")
		resp, err := h.UpdateResourceReleaseBinding(ctx, gen.UpdateResourceReleaseBindingRequestObject{
			NamespaceName:              testRRBNs,
			ResourceReleaseBindingName: "nonexistent",
			Body:                       &body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateResourceReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		body := newRRBCreateBody("different-name")
		resp, err := h.UpdateResourceReleaseBinding(ctx, gen.UpdateResourceReleaseBindingRequestObject{
			NamespaceName:              testRRBNs,
			ResourceReleaseBindingName: "rb-1",
			Body:                       &body,
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateResourceReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "rb-1", typed.Metadata.Name)
	})
}

// A client that reads a binding and writes it back (occ resource promote, the portal,
// the MCP tools) must not drop the requeue intervals set with kubectl or GitOps.
func TestResourceReleaseBindingHTTPGetThenUpdateKeepsIntervals(t *testing.T) {
	existing := testResourceReleaseBindingObj("rb-1")
	existing.Spec.Interval = &metav1.Duration{Duration: 2 * time.Minute}
	existing.Spec.ProgressingInterval = &metav1.Duration{Duration: 30 * time.Second}
	svc := newResourceReleaseBindingService(t, []client.Object{existing}, &allowAllPDP{})
	handler := newTestHTTPHandler(t, &handlerservices.Services{ResourceReleaseBindingService: svc})
	path := "/api/v1/namespaces/" + testRRBNs + "/resourcereleasebindings/rb-1"

	req, rec := doRequest(t, handler, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, rec.Body.Bytes())

	var fetched gen.ResourceReleaseBinding
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fetched))
	require.NotNil(t, fetched.Spec)
	fetched.Spec.ResourceRelease = ptr.To("test-r-abc123")
	body, err := json.Marshal(fetched)
	require.NoError(t, err)

	req, rec = doRequest(t, handler, http.MethodPut, path, body)
	require.Equal(t, http.StatusOK, rec.Code)
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, rec.Body.Bytes())

	got, err := svc.GetResourceReleaseBinding(testContext(), testRRBNs, "rb-1")
	require.NoError(t, err)
	assert.Equal(t, "test-r-abc123", got.Spec.ResourceRelease)
	assert.Equal(t, &metav1.Duration{Duration: 2 * time.Minute}, got.Spec.Interval)
	assert.Equal(t, &metav1.Duration{Duration: 30 * time.Second}, got.Spec.ProgressingInterval)
}

// --- DeleteResourceReleaseBinding Handler ---

func TestDeleteResourceReleaseBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.DeleteResourceReleaseBinding(ctx, gen.DeleteResourceReleaseBindingRequestObject{NamespaceName: testRRBNs, ResourceReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceReleaseBinding204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.DeleteResourceReleaseBinding(ctx, gen.DeleteResourceReleaseBindingRequestObject{NamespaceName: testRRBNs, ResourceReleaseBindingName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newResourceReleaseBindingService(t, []client.Object{testResourceReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithResourceReleaseBindingService(svc)

		resp, err := h.DeleteResourceReleaseBinding(ctx, gen.DeleteResourceReleaseBindingRequestObject{NamespaceName: testRRBNs, ResourceReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceReleaseBinding403JSONResponse{}, resp)
	})
}
