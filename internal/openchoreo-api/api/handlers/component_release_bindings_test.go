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
	componentcomponentreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentreleasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newComponentReleaseBindingService(t *testing.T, objects []client.Object, pdp authzcore.PDP) componentcomponentreleasebindingsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return componentcomponentreleasebindingsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithComponentReleaseBindingService(svc componentcomponentreleasebindingsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ComponentReleaseBindingService: svc},
		logger:   slog.Default(),
	}
}

func testComponentReleaseBindingObj(name string) *openchoreov1alpha1.ComponentReleaseBinding {
	return &openchoreov1alpha1.ComponentReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.ComponentReleaseBindingSpec{
			Owner: openchoreov1alpha1.ComponentReleaseBindingOwner{
				ProjectName:   "test-proj",
				ComponentName: "test-comp",
			},
			Environment: "dev",
		},
	}
}

func testComponentForCRB() *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-comp",
			Namespace: "test-ns",
		},
	}
}

func validComponentReleaseBindingBody(name string) *gen.ComponentReleaseBinding {
	return &gen.ComponentReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ComponentReleaseBindingSpec{
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ProjectName:   "test-proj",
				ComponentName: "test-comp",
			},
			Environment: "dev",
		},
	}
}

// --- ListComponentComponentReleaseBindings Handler ---

func TestListComponentComponentReleaseBindingsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.ListComponentReleaseBindings(ctx, gen.ListComponentReleaseBindingsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListComponentReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.ListComponentReleaseBindings(ctx, gen.ListComponentReleaseBindingsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListComponentReleaseBindingsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListComponentReleaseBindings400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.ListComponentReleaseBindings(ctx, gen.ListComponentReleaseBindingsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListComponentReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetComponentReleaseBinding Handler ---

func TestGetComponentReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.GetComponentReleaseBinding(ctx, gen.GetComponentReleaseBindingRequestObject{
			NamespaceName: ns, ComponentReleaseBindingName: "rb-1",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetComponentReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.GetComponentReleaseBinding(ctx, gen.GetComponentReleaseBindingRequestObject{
			NamespaceName: ns, ComponentReleaseBindingName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetComponentReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.GetComponentReleaseBinding(ctx, gen.GetComponentReleaseBindingRequestObject{
			NamespaceName: ns, ComponentReleaseBindingName: "rb-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetComponentReleaseBinding403JSONResponse{}, resp)
	})
}

// --- CreateComponentReleaseBinding Handler ---

func TestCreateComponentReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentForCRB()}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.CreateComponentReleaseBinding(ctx, gen.CreateComponentReleaseBindingRequestObject{
			NamespaceName: ns,
			Body:          validComponentReleaseBindingBody("new-rb"),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentReleaseBinding201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.CreateComponentReleaseBinding(ctx, gen.CreateComponentReleaseBindingRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		objs := []client.Object{testComponentForCRB(), testComponentReleaseBindingObj("new-rb")}
		svc := newComponentReleaseBindingService(t, objs, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.CreateComponentReleaseBinding(ctx, gen.CreateComponentReleaseBindingRequestObject{
			NamespaceName: ns,
			Body:          validComponentReleaseBindingBody("new-rb"),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentReleaseBinding409JSONResponse{}, resp)
	})

	t.Run("namespace mismatch returns 400", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		body := &gen.ComponentReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1", Namespace: ptr.To("other-ns")}}
		resp, err := h.CreateComponentReleaseBinding(ctx, gen.CreateComponentReleaseBindingRequestObject{
			NamespaceName: ns, Body: body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentForCRB()}, &denyAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.CreateComponentReleaseBinding(ctx, gen.CreateComponentReleaseBindingRequestObject{
			NamespaceName: ns,
			Body:          validComponentReleaseBindingBody("new-rb"),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentReleaseBinding403JSONResponse{}, resp)
	})
}

// --- UpdateComponentReleaseBinding Handler ---

func TestUpdateComponentReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.UpdateComponentReleaseBinding(ctx, gen.UpdateComponentReleaseBindingRequestObject{
			NamespaceName:               ns,
			ComponentReleaseBindingName: "rb-1",
			Body:                        &gen.ComponentReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
		})
		require.NoError(t, err)
		_, ok := resp.(gen.UpdateComponentReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.UpdateComponentReleaseBinding(ctx, gen.UpdateComponentReleaseBindingRequestObject{
			NamespaceName:               ns,
			ComponentReleaseBindingName: "rb-1",
			Body:                        nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.UpdateComponentReleaseBinding(ctx, gen.UpdateComponentReleaseBindingRequestObject{
			NamespaceName:               ns,
			ComponentReleaseBindingName: "nonexistent",
			Body:                        &gen.ComponentReleaseBinding{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("namespace mismatch returns 400", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		body := &gen.ComponentReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1", Namespace: ptr.To("other-ns")}}
		resp, err := h.UpdateComponentReleaseBinding(ctx, gen.UpdateComponentReleaseBindingRequestObject{
			NamespaceName:               ns,
			ComponentReleaseBindingName: "rb-1",
			Body:                        body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.UpdateComponentReleaseBinding(ctx, gen.UpdateComponentReleaseBindingRequestObject{
			NamespaceName:               ns,
			ComponentReleaseBindingName: "rb-1",
			Body:                        &gen.ComponentReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentReleaseBinding403JSONResponse{}, resp)
	})
}

// --- DeleteComponentReleaseBinding Handler ---

func TestDeleteComponentReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.DeleteComponentReleaseBinding(ctx, gen.DeleteComponentReleaseBindingRequestObject{
			NamespaceName: ns, ComponentReleaseBindingName: "rb-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentReleaseBinding204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.DeleteComponentReleaseBinding(ctx, gen.DeleteComponentReleaseBindingRequestObject{
			NamespaceName: ns, ComponentReleaseBindingName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newComponentReleaseBindingService(t, []client.Object{testComponentReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithComponentReleaseBindingService(svc)

		resp, err := h.DeleteComponentReleaseBinding(ctx, gen.DeleteComponentReleaseBindingRequestObject{
			NamespaceName: ns, ComponentReleaseBindingName: "rb-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteComponentReleaseBinding403JSONResponse{}, resp)
	})
}
