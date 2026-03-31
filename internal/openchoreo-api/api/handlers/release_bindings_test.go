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
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
)

func newReleaseBindingService(t *testing.T, objects []client.Object, pdp authzcore.PDP) releasebindingsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return releasebindingsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithReleaseBindingService(svc releasebindingsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ReleaseBindingService: svc},
		logger:   slog.Default(),
	}
}

func testReleaseBindingObj(name string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   "test-proj",
				ComponentName: "test-comp",
			},
			Environment: "dev",
		},
	}
}

func testComponentForRB() *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-comp",
			Namespace: "test-ns",
		},
	}
}

func validReleaseBindingBody(name string) *gen.ReleaseBinding {
	return &gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ReleaseBindingSpec{
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

// --- ListReleaseBindings Handler ---

func TestListReleaseBindingsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.ListReleaseBindings(ctx, gen.ListReleaseBindingsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.ListReleaseBindings(ctx, gen.ListReleaseBindingsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListReleaseBindingsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListReleaseBindings400JSONResponse{}, resp)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.ListReleaseBindings(ctx, gen.ListReleaseBindingsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListReleaseBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetReleaseBinding Handler ---

func TestGetReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.GetReleaseBinding(ctx, gen.GetReleaseBindingRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1",
		})
		require.NoError(t, err)
		_, ok := resp.(gen.GetReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.GetReleaseBinding(ctx, gen.GetReleaseBindingRequestObject{
			NamespaceName: ns, ReleaseBindingName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.GetReleaseBinding(ctx, gen.GetReleaseBindingRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.GetReleaseBinding403JSONResponse{}, resp)
	})
}

// --- CreateReleaseBinding Handler ---

func TestCreateReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testComponentForRB()}, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.CreateReleaseBinding(ctx, gen.CreateReleaseBindingRequestObject{
			NamespaceName: ns,
			Body:          validReleaseBindingBody("new-rb"),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateReleaseBinding201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.CreateReleaseBinding(ctx, gen.CreateReleaseBindingRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		objs := []client.Object{testComponentForRB(), testReleaseBindingObj("new-rb")}
		svc := newReleaseBindingService(t, objs, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.CreateReleaseBinding(ctx, gen.CreateReleaseBindingRequestObject{
			NamespaceName: ns,
			Body:          validReleaseBindingBody("new-rb"),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateReleaseBinding409JSONResponse{}, resp)
	})

	t.Run("namespace mismatch returns 400", func(t *testing.T) {
		svc := newReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		body := &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1", Namespace: ptr.To("other-ns")}}
		resp, err := h.CreateReleaseBinding(ctx, gen.CreateReleaseBindingRequestObject{
			NamespaceName: ns, Body: body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testComponentForRB()}, &denyAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.CreateReleaseBinding(ctx, gen.CreateReleaseBindingRequestObject{
			NamespaceName: ns,
			Body:          validReleaseBindingBody("new-rb"),
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateReleaseBinding403JSONResponse{}, resp)
	})
}

// --- UpdateReleaseBinding Handler ---

func TestUpdateReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.UpdateReleaseBinding(ctx, gen.UpdateReleaseBindingRequestObject{
			NamespaceName:      ns,
			ReleaseBindingName: "rb-1",
			Body:               &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
		})
		require.NoError(t, err)
		_, ok := resp.(gen.UpdateReleaseBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.UpdateReleaseBinding(ctx, gen.UpdateReleaseBindingRequestObject{
			NamespaceName:      ns,
			ReleaseBindingName: "rb-1",
			Body:               nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.UpdateReleaseBinding(ctx, gen.UpdateReleaseBindingRequestObject{
			NamespaceName:      ns,
			ReleaseBindingName: "nonexistent",
			Body:               &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("namespace mismatch returns 400", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		body := &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1", Namespace: ptr.To("other-ns")}}
		resp, err := h.UpdateReleaseBinding(ctx, gen.UpdateReleaseBindingRequestObject{
			NamespaceName:      ns,
			ReleaseBindingName: "rb-1",
			Body:               body,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateReleaseBinding400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.UpdateReleaseBinding(ctx, gen.UpdateReleaseBindingRequestObject{
			NamespaceName:      ns,
			ReleaseBindingName: "rb-1",
			Body:               &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateReleaseBinding403JSONResponse{}, resp)
	})
}

// --- DeleteReleaseBinding Handler ---

func TestDeleteReleaseBindingHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.DeleteReleaseBinding(ctx, gen.DeleteReleaseBindingRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteReleaseBinding204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newReleaseBindingService(t, nil, &allowAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.DeleteReleaseBinding(ctx, gen.DeleteReleaseBindingRequestObject{
			NamespaceName: ns, ReleaseBindingName: "nonexistent",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteReleaseBinding404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &denyAllPDP{})
		h := newHandlerWithReleaseBindingService(svc)

		resp, err := h.DeleteReleaseBinding(ctx, gen.DeleteReleaseBindingRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1",
		})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteReleaseBinding403JSONResponse{}, resp)
	})
}
