// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
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
	clusterhooksvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterhook"
	clusterhookmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterhook/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

func newClusterHookService(t *testing.T, objects []client.Object, pdp authzcore.PDP) clusterhooksvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return clusterhooksvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithClusterHookService(svc clusterhooksvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{ClusterHookService: svc},
		logger:   slog.Default(),
	}
}

func testClusterHookObj() *openchoreov1alpha1.ClusterHook {
	return &openchoreov1alpha1.ClusterHook{
		ObjectMeta: metav1.ObjectMeta{Name: "scan"},
		Spec: openchoreov1alpha1.HookSpec{
			WorkflowRef: &openchoreov1alpha1.WorkflowRef{Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow, Name: "trivy"},
		},
	}
}

func TestListClusterHooksHandler(t *testing.T) {
	ctx := testContext()

	t.Run("returns items when authorized", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, []client.Object{testClusterHookObj()}, &allowAllPDP{}))
		resp, err := h.ListClusterHooks(ctx, gen.ListClusterHooksRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "scan", typed.Items[0].Metadata.Name)
	})

	t.Run("filters unauthorized items", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, []client.Object{testClusterHookObj()}, &denyAllPDP{}))
		resp, err := h.ListClusterHooks(ctx, gen.ListClusterHooksRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})
}

func TestGetClusterHookHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, []client.Object{testClusterHookObj()}, &allowAllPDP{}))
		resp, err := h.GetClusterHook(ctx, gen.GetClusterHookRequestObject{ClusterHookName: "scan"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterHook200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "scan", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, "trivy", typed.Spec.WorkflowRef.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, nil, &allowAllPDP{}))
		resp, err := h.GetClusterHook(ctx, gen.GetClusterHookRequestObject{ClusterHookName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterHook404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, []client.Object{testClusterHookObj()}, &denyAllPDP{}))
		resp, err := h.GetClusterHook(ctx, gen.GetClusterHookRequestObject{ClusterHookName: "scan"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterHook403JSONResponse{}, resp)
	})
}

func TestCreateClusterHookHandler(t *testing.T) {
	ctx := testContext()
	body := func() *gen.ClusterHook {
		return &gen.ClusterHook{Metadata: gen.ObjectMeta{Name: "scan"}}
	}

	t.Run("success returns 201", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, nil, &allowAllPDP{}))
		resp, err := h.CreateClusterHook(ctx, gen.CreateClusterHookRequestObject{Body: body()})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateClusterHook201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "scan", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, nil, &allowAllPDP{}))
		resp, err := h.CreateClusterHook(ctx, gen.CreateClusterHookRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterHook400JSONResponse{}, resp)
	})

	t.Run("maps service errors", func(t *testing.T) {
		tests := []struct {
			name    string
			svcErr  error
			wantTyp any
		}{
			{"forbidden -> 403", svcpkg.ErrForbidden, gen.CreateClusterHook403JSONResponse{}},
			{"already exists -> 409", clusterhooksvc.ErrClusterHookAlreadyExists, gen.CreateClusterHook409JSONResponse{}},
			{"internal -> 500", assert.AnError, gen.CreateClusterHook500JSONResponse{}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc := clusterhookmocks.NewMockService(t)
				svc.EXPECT().CreateClusterHook(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
				h := newHandlerWithClusterHookService(svc)
				resp, err := h.CreateClusterHook(ctx, gen.CreateClusterHookRequestObject{Body: body()})
				require.NoError(t, err)
				assert.IsType(t, tt.wantTyp, resp)
			})
		}
	})
}

func TestUpdateClusterHookHandler(t *testing.T) {
	ctx := testContext()

	// The path is authoritative for the name: a body naming a different hook must not
	// let a caller update an object they did not address.
	t.Run("uses the path name over the body name", func(t *testing.T) {
		svc := clusterhookmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterHook(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ch *openchoreov1alpha1.ClusterHook) (*openchoreov1alpha1.ClusterHook, error) {
			assert.Equal(t, "from-path", ch.Name)
			return ch, nil
		})
		h := newHandlerWithClusterHookService(svc)
		resp, err := h.UpdateClusterHook(ctx, gen.UpdateClusterHookRequestObject{
			ClusterHookName: "from-path",
			Body:            &gen.ClusterHook{Metadata: gen.ObjectMeta{Name: "from-body"}},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateClusterHook200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "from-path", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, nil, &allowAllPDP{}))
		resp, err := h.UpdateClusterHook(ctx, gen.UpdateClusterHookRequestObject{
			ClusterHookName: "missing", Body: &gen.ClusterHook{Metadata: gen.ObjectMeta{Name: "missing"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterHook404JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, nil, &allowAllPDP{}))
		resp, err := h.UpdateClusterHook(ctx, gen.UpdateClusterHookRequestObject{ClusterHookName: "x"})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterHook400JSONResponse{}, resp)
	})
}

func TestDeleteClusterHookHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success returns 204", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, []client.Object{testClusterHookObj()}, &allowAllPDP{}))
		resp, err := h.DeleteClusterHook(ctx, gen.DeleteClusterHookRequestObject{ClusterHookName: "scan"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterHook204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, nil, &allowAllPDP{}))
		resp, err := h.DeleteClusterHook(ctx, gen.DeleteClusterHookRequestObject{ClusterHookName: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterHook404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, []client.Object{testClusterHookObj()}, &denyAllPDP{}))
		resp, err := h.DeleteClusterHook(ctx, gen.DeleteClusterHookRequestObject{ClusterHookName: "scan"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterHook403JSONResponse{}, resp)
	})
}

func TestListClusterHooksHandler_ErrorMapping(t *testing.T) {
	ctx := testContext()

	// Each service failure must map to its own status so the portal can tell a permission
	// problem or a bad selector apart from a server fault.
	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := clusterhookmocks.NewMockService(t)
		svc.EXPECT().ListClusterHooks(mock.Anything, mock.Anything).Return(nil, svcpkg.ErrForbidden)
		resp, err := newHandlerWithClusterHookService(svc).ListClusterHooks(ctx, gen.ListClusterHooksRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterHooks403JSONResponse)
		require.True(t, ok, "expected 403 response, got %T", resp)
		assert.Equal(t, gen.FORBIDDEN, typed.Code)
	})

	t.Run("validation error returns 400 with the service message", func(t *testing.T) {
		h := newHandlerWithClusterHookService(newClusterHookService(t, nil, &allowAllPDP{}))
		resp, err := h.ListClusterHooks(ctx, gen.ListClusterHooksRequestObject{
			Params: gen.ListClusterHooksParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterHooks400JSONResponse)
		require.True(t, ok, "expected 400 response, got %T", resp)
		assert.Equal(t, gen.BADREQUEST, typed.Code)
		assert.Contains(t, typed.Error, "invalid label selector")
	})

	// Internal error details must not leak to the client; only the generic message is sent.
	t.Run("unexpected error returns 500", func(t *testing.T) {
		svc := clusterhookmocks.NewMockService(t)
		svc.EXPECT().ListClusterHooks(mock.Anything, mock.Anything).Return(nil, assert.AnError)
		resp, err := newHandlerWithClusterHookService(svc).ListClusterHooks(ctx, gen.ListClusterHooksRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterHooks500JSONResponse)
		require.True(t, ok, "expected 500 response, got %T", resp)
		assert.Equal(t, gen.INTERNALERROR, typed.Code)
		assert.NotContains(t, typed.Error, assert.AnError.Error())
	})
}

// The handler must forward the caller's paging params to the service and hand the next
// cursor back, otherwise the portal cannot page past the first result set.
func TestListClusterHooksHandler_Pagination(t *testing.T) {
	svc := clusterhookmocks.NewMockService(t)
	svc.EXPECT().ListClusterHooks(mock.Anything, svcpkg.ListOptions{Limit: 2, Cursor: "c1", LabelSelector: "team=a"}).
		Return(&svcpkg.ListResult[openchoreov1alpha1.ClusterHook]{
			Items:      []openchoreov1alpha1.ClusterHook{*testClusterHookObj()},
			NextCursor: "c2",
		}, nil)

	resp, err := newHandlerWithClusterHookService(svc).ListClusterHooks(testContext(), gen.ListClusterHooksRequestObject{
		Params: gen.ListClusterHooksParams{Limit: ptr.To(2), Cursor: ptr.To("c1"), LabelSelector: ptr.To("team=a")},
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.ListClusterHooks200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	require.Len(t, typed.Items, 1)
	require.NotNil(t, typed.Pagination.NextCursor)
	assert.Equal(t, "c2", *typed.Pagination.NextCursor)
}

func TestCreateClusterHookHandler_ValidationMapping(t *testing.T) {
	ctx := testContext()
	body := &gen.ClusterHook{Metadata: gen.ObjectMeta{Name: "scan"}}

	// API-server schema rejections (422) and plain validation failures (400) are distinct
	// responses; both must carry the service's message so the user can fix the input.
	tests := []struct {
		name     string
		svcErr   *svcpkg.ValidationError
		wantCode gen.ErrorResponseCode
		check    func(t *testing.T, resp gen.CreateClusterHookResponseObject) gen.ErrorResponse
	}{
		{
			name:     "422 status maps to 422",
			svcErr:   &svcpkg.ValidationError{Msg: "workflowRef.kind must be ClusterWorkflow", StatusCode: http.StatusUnprocessableEntity},
			wantCode: gen.UNPROCESSABLECONTENT,
			check: func(t *testing.T, resp gen.CreateClusterHookResponseObject) gen.ErrorResponse {
				typed, ok := resp.(gen.CreateClusterHook422JSONResponse)
				require.True(t, ok, "expected 422 response, got %T", resp)
				return gen.ErrorResponse(typed.UnprocessableContentJSONResponse)
			},
		},
		{
			name:     "other status maps to 400",
			svcErr:   &svcpkg.ValidationError{Msg: "workflowRef.kind must be ClusterWorkflow"},
			wantCode: gen.BADREQUEST,
			check: func(t *testing.T, resp gen.CreateClusterHookResponseObject) gen.ErrorResponse {
				typed, ok := resp.(gen.CreateClusterHook400JSONResponse)
				require.True(t, ok, "expected 400 response, got %T", resp)
				return gen.ErrorResponse(typed.BadRequestJSONResponse)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := clusterhookmocks.NewMockService(t)
			svc.EXPECT().CreateClusterHook(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			resp, err := newHandlerWithClusterHookService(svc).CreateClusterHook(ctx, gen.CreateClusterHookRequestObject{Body: body})
			require.NoError(t, err)
			errBody := tt.check(t, resp)
			assert.Equal(t, tt.wantCode, errBody.Code)
			assert.Equal(t, tt.svcErr.Msg, errBody.Error)
		})
	}
}

func TestUpdateClusterHookHandler_ErrorMapping(t *testing.T) {
	ctx := testContext()
	req := gen.UpdateClusterHookRequestObject{
		ClusterHookName: "scan",
		Body:            &gen.ClusterHook{Metadata: gen.ObjectMeta{Name: "scan"}},
	}

	// Every update failure class must map to its documented status; a regression here
	// would e.g. turn a permission denial into a 500 in the portal.
	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateClusterHook403JSONResponse{}},
		{"not found -> 404", clusterhooksvc.ErrClusterHookNotFound, gen.UpdateClusterHook404JSONResponse{}},
		{"422 validation -> 422", &svcpkg.ValidationError{Msg: "bad", StatusCode: http.StatusUnprocessableEntity}, gen.UpdateClusterHook422JSONResponse{}},
		{"other validation -> 400", &svcpkg.ValidationError{Msg: "bad"}, gen.UpdateClusterHook400JSONResponse{}},
		{"internal -> 500", assert.AnError, gen.UpdateClusterHook500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := clusterhookmocks.NewMockService(t)
			svc.EXPECT().UpdateClusterHook(mock.Anything, mock.Anything).Return(nil, tt.svcErr)
			resp, err := newHandlerWithClusterHookService(svc).UpdateClusterHook(ctx, req)
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}

	t.Run("validation message is passed through", func(t *testing.T) {
		svc := clusterhookmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterHook(mock.Anything, mock.Anything).
			Return(nil, &svcpkg.ValidationError{Msg: "spec.workflowRef is required", StatusCode: http.StatusUnprocessableEntity})
		resp, err := newHandlerWithClusterHookService(svc).UpdateClusterHook(ctx, req)
		require.NoError(t, err)
		typed, ok := resp.(gen.UpdateClusterHook422JSONResponse)
		require.True(t, ok, "expected 422 response, got %T", resp)
		assert.Equal(t, gen.UNPROCESSABLECONTENT, typed.Code)
		assert.Equal(t, "spec.workflowRef is required", typed.Error)
	})
}

func TestGetAndDeleteClusterHookHandler_InternalError(t *testing.T) {
	ctx := testContext()

	// Unexpected service failures must be a 500, never a 404 that would tell the user
	// the hook is gone when the API was merely unavailable.
	t.Run("get unexpected error returns 500", func(t *testing.T) {
		svc := clusterhookmocks.NewMockService(t)
		svc.EXPECT().GetClusterHook(mock.Anything, "scan").Return(nil, assert.AnError)
		resp, err := newHandlerWithClusterHookService(svc).GetClusterHook(ctx, gen.GetClusterHookRequestObject{ClusterHookName: "scan"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterHook500JSONResponse{}, resp)
	})

	t.Run("delete unexpected error returns 500", func(t *testing.T) {
		svc := clusterhookmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterHook(mock.Anything, "scan").Return(assert.AnError)
		resp, err := newHandlerWithClusterHookService(svc).DeleteClusterHook(ctx, gen.DeleteClusterHookRequestObject{ClusterHookName: "scan"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterHook500JSONResponse{}, resp)
	})
}
