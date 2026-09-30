// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"net/http"
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
	hooksvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/hook"
	hookmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/hook/mocks"
)

func newHookService(t *testing.T, objects []client.Object, pdp authzcore.PDP) hooksvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return hooksvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithHookService(svc hooksvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{HookService: svc},
		logger:   slog.Default(),
	}
}

func testHookObj(name string) *openchoreov1alpha1.Hook {
	return &openchoreov1alpha1.Hook{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
	}
}

// --- ListHooks Handler ---

func TestListHooksHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success - returns items", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.ListHooks(ctx, gen.ListHooksRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "hook-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newHookService(t, nil, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.ListHooks(ctx, gen.ListHooksRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newHookService(t, nil, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.ListHooks(ctx, gen.ListHooksRequestObject{
			NamespaceName: ns,
			Params:        gen.ListHooksParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListHooks400JSONResponse{}, resp)
	})

	t.Run("unauthorized items filtered out", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &denyAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.ListHooks(ctx, gen.ListHooksRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

// --- GetHook Handler ---

func TestGetHookHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.GetHook(ctx, gen.GetHookRequestObject{NamespaceName: ns, HookName: "hook-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetHook200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "hook-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newHookService(t, nil, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.GetHook(ctx, gen.GetHookRequestObject{NamespaceName: ns, HookName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetHook404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &denyAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.GetHook(ctx, gen.GetHookRequestObject{NamespaceName: ns, HookName: "hook-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetHook403JSONResponse{}, resp)
	})
}

// --- CreateHook Handler ---

func TestCreateHookHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newHookService(t, nil, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.CreateHook(ctx, gen.CreateHookRequestObject{
			NamespaceName: ns,
			Body:          &gen.Hook{Metadata: gen.ObjectMeta{Name: "new-hook"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateHook201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-hook", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newHookService(t, nil, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.CreateHook(ctx, gen.CreateHookRequestObject{
			NamespaceName: ns,
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateHook400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		existing := testHookObj("new-hook")
		svc := newHookService(t, []client.Object{existing}, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.CreateHook(ctx, gen.CreateHookRequestObject{
			NamespaceName: ns,
			Body:          &gen.Hook{Metadata: gen.ObjectMeta{Name: "new-hook"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateHook409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newHookService(t, nil, &denyAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.CreateHook(ctx, gen.CreateHookRequestObject{
			NamespaceName: ns,
			Body:          &gen.Hook{Metadata: gen.ObjectMeta{Name: "new-hook"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateHook403JSONResponse{}, resp)
	})
}

// --- UpdateHook Handler ---

func TestUpdateHookHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.UpdateHook(ctx, gen.UpdateHookRequestObject{
			NamespaceName: ns,
			HookName:      "hook-1",
			Body:          &gen.Hook{Metadata: gen.ObjectMeta{Name: "hook-1"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateHook200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "hook-1", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newHookService(t, nil, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.UpdateHook(ctx, gen.UpdateHookRequestObject{
			NamespaceName: ns,
			HookName:      "hook-1",
			Body:          nil,
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateHook400JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newHookService(t, nil, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.UpdateHook(ctx, gen.UpdateHookRequestObject{
			NamespaceName: ns,
			HookName:      "nonexistent",
			Body:          &gen.Hook{Metadata: gen.ObjectMeta{Name: "nonexistent"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateHook404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &denyAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.UpdateHook(ctx, gen.UpdateHookRequestObject{
			NamespaceName: ns,
			HookName:      "hook-1",
			Body:          &gen.Hook{Metadata: gen.ObjectMeta{Name: "hook-1"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateHook403JSONResponse{}, resp)
	})

	t.Run("URL path name overrides body name", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.UpdateHook(ctx, gen.UpdateHookRequestObject{
			NamespaceName: ns,
			HookName:      "hook-1",
			Body:          &gen.Hook{Metadata: gen.ObjectMeta{Name: "different-name"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateHook200JSONResponse)
		require.True(t, ok, "expected 200 response (URL path name used), got %T", resp)
		assert.Equal(t, "hook-1", typed.Metadata.Name)
	})
}

// --- DeleteHook Handler ---

func TestDeleteHookHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.DeleteHook(ctx, gen.DeleteHookRequestObject{NamespaceName: ns, HookName: "hook-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteHook204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newHookService(t, nil, &allowAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.DeleteHook(ctx, gen.DeleteHookRequestObject{NamespaceName: ns, HookName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteHook404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newHookService(t, []client.Object{testHookObj("hook-1")}, &denyAllPDP{})
		h := newHandlerWithHookService(svc)

		resp, err := h.DeleteHook(ctx, gen.DeleteHookRequestObject{NamespaceName: ns, HookName: "hook-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteHook403JSONResponse{}, resp)
	})
}

// --- Error mapping and pagination (mocked service) ---

func TestListHooksHandler_InternalErrorAndPagination(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	// Internal error details must not leak to the client; only the generic 500 body is sent.
	t.Run("unexpected error returns 500", func(t *testing.T) {
		svc := hookmocks.NewMockService(t)
		svc.EXPECT().ListHooks(mock.Anything, ns, mock.Anything).Return(nil, assert.AnError)
		resp, err := newHandlerWithHookService(svc).ListHooks(ctx, gen.ListHooksRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListHooks500JSONResponse)
		require.True(t, ok, "expected 500 response, got %T", resp)
		assert.Equal(t, gen.INTERNALERROR, typed.Code)
		assert.NotContains(t, typed.Error, assert.AnError.Error())
	})

	// Paging params must reach the service unchanged (an over-large limit is clamped to
	// the max page size) and the next cursor must be returned so the portal can page on.
	t.Run("forwards paging params and returns the next cursor", func(t *testing.T) {
		svc := hookmocks.NewMockService(t)
		svc.EXPECT().ListHooks(mock.Anything, ns, svcpkg.ListOptions{Limit: maxPageLimit, Cursor: "c1", LabelSelector: "team=a"}).
			Return(&svcpkg.ListResult[openchoreov1alpha1.Hook]{
				Items:      []openchoreov1alpha1.Hook{*testHookObj("hook-1")},
				NextCursor: "c2",
			}, nil)
		resp, err := newHandlerWithHookService(svc).ListHooks(ctx, gen.ListHooksRequestObject{
			NamespaceName: ns,
			Params:        gen.ListHooksParams{Limit: ptr.To(maxPageLimit + 50), Cursor: ptr.To("c1"), LabelSelector: ptr.To("team=a")},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "hook-1", typed.Items[0].Metadata.Name)
		require.NotNil(t, typed.Pagination.NextCursor)
		assert.Equal(t, "c2", *typed.Pagination.NextCursor)
	})
}

func TestCreateHookHandler_ErrorMapping(t *testing.T) {
	ctx := testContext()
	req := gen.CreateHookRequestObject{NamespaceName: "test-ns", Body: &gen.Hook{Metadata: gen.ObjectMeta{Name: "new-hook"}}}

	// Schema rejections from the API server (422) and plain validation failures (400) are
	// distinct responses, and both must carry the message so the user can fix the input.
	t.Run("422 validation maps to 422 with message", func(t *testing.T) {
		svc := hookmocks.NewMockService(t)
		svc.EXPECT().CreateHook(mock.Anything, "test-ns", mock.Anything).
			Return(nil, &svcpkg.ValidationError{Msg: "spec.workflowRef is required", StatusCode: http.StatusUnprocessableEntity})
		resp, err := newHandlerWithHookService(svc).CreateHook(ctx, req)
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateHook422JSONResponse)
		require.True(t, ok, "expected 422 response, got %T", resp)
		assert.Equal(t, gen.UNPROCESSABLECONTENT, typed.Code)
		assert.Equal(t, "spec.workflowRef is required", typed.Error)
	})

	t.Run("other validation maps to 400 with message", func(t *testing.T) {
		svc := hookmocks.NewMockService(t)
		svc.EXPECT().CreateHook(mock.Anything, "test-ns", mock.Anything).
			Return(nil, &svcpkg.ValidationError{Msg: "bad parameter"})
		resp, err := newHandlerWithHookService(svc).CreateHook(ctx, req)
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateHook400JSONResponse)
		require.True(t, ok, "expected 400 response, got %T", resp)
		assert.Equal(t, gen.BADREQUEST, typed.Code)
		assert.Equal(t, "bad parameter", typed.Error)
	})

	t.Run("unexpected error returns 500", func(t *testing.T) {
		svc := hookmocks.NewMockService(t)
		svc.EXPECT().CreateHook(mock.Anything, "test-ns", mock.Anything).Return(nil, assert.AnError)
		resp, err := newHandlerWithHookService(svc).CreateHook(ctx, req)
		require.NoError(t, err)
		assert.IsType(t, gen.CreateHook500JSONResponse{}, resp)
	})
}

func TestUpdateHookHandler_ErrorMapping(t *testing.T) {
	ctx := testContext()
	req := gen.UpdateHookRequestObject{NamespaceName: "test-ns", HookName: "hook-1", Body: &gen.Hook{Metadata: gen.ObjectMeta{Name: "hook-1"}}}

	// Validation and unexpected failures must stay distinguishable: a 422/400 tells the
	// user to fix input, a 500 tells them the server failed.
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"422 validation -> 422", &svcpkg.ValidationError{Msg: "bad", StatusCode: http.StatusUnprocessableEntity}, gen.UpdateHook422JSONResponse{}},
		{"other validation -> 400", &svcpkg.ValidationError{Msg: "bad"}, gen.UpdateHook400JSONResponse{}},
		{"internal -> 500", assert.AnError, gen.UpdateHook500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := hookmocks.NewMockService(t)
			svc.EXPECT().UpdateHook(mock.Anything, "test-ns", mock.Anything).Return(nil, tt.svcErr)
			resp, err := newHandlerWithHookService(svc).UpdateHook(ctx, req)
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestGetAndDeleteHookHandler_InternalError(t *testing.T) {
	ctx := testContext()

	// Unexpected failures must be a 500, never a 404 that would tell the user the hook is
	// gone when the API was merely unavailable.
	t.Run("get unexpected error returns 500", func(t *testing.T) {
		svc := hookmocks.NewMockService(t)
		svc.EXPECT().GetHook(mock.Anything, "test-ns", "hook-1").Return(nil, assert.AnError)
		resp, err := newHandlerWithHookService(svc).GetHook(ctx, gen.GetHookRequestObject{NamespaceName: "test-ns", HookName: "hook-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetHook500JSONResponse{}, resp)
	})

	t.Run("delete unexpected error returns 500", func(t *testing.T) {
		svc := hookmocks.NewMockService(t)
		svc.EXPECT().DeleteHook(mock.Anything, "test-ns", "hook-1").Return(assert.AnError)
		resp, err := newHandlerWithHookService(svc).DeleteHook(ctx, gen.DeleteHookRequestObject{NamespaceName: "test-ns", HookName: "hook-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteHook500JSONResponse{}, resp)
	})
}
