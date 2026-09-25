// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	releasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
)

// internalLeak is the text an unexpected service error carries; it must never reach a client.
const internalLeak = "etcd at 10.0.0.7 refused connection"

func newHandlerWithMockRBService(svc *releasebindingmocks.MockService) *Handler {
	return &Handler{services: &handlerservices.Services{ReleaseBindingService: svc}, logger: slog.Default()}
}

// TestListReleaseBindingHooksHandlerErrorBodies pins the exact error bodies: the UI keys on
// the code, and an unexpected error must be a generic 500 with no internal detail.
func TestListReleaseBindingHooksHandlerErrorBodies(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"
	req := gen.ListReleaseBindingHooksRequestObject{NamespaceName: ns, ReleaseBindingName: "rb-1"}

	tests := []struct {
		name   string
		svcErr error
		want   gen.ListReleaseBindingHooksResponseObject
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden,
			gen.ListReleaseBindingHooks403JSONResponse{ForbiddenJSONResponse: gen.ForbiddenJSONResponse{
				Code: gen.FORBIDDEN, Error: "You do not have permission to perform this operation"}}},
		{"not found -> 404", releasebindingsvc.ErrReleaseBindingNotFound,
			gen.ListReleaseBindingHooks404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse{
				Code: gen.NOTFOUND, Error: "ReleaseBinding not found"}}},
		{"unexpected -> 500", fmt.Errorf("failed to get release binding: %s", internalLeak),
			gen.ListReleaseBindingHooks500JSONResponse{InternalErrorJSONResponse: gen.InternalErrorJSONResponse{
				Code: gen.INTERNALERROR, Error: "Internal server error"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := releasebindingmocks.NewMockService(t)
			svc.EXPECT().ListHooks(mock.Anything, ns, "rb-1").Return(nil, tt.svcErr)
			resp, err := newHandlerWithMockRBService(svc).ListReleaseBindingHooks(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, tt.want, resp)
		})
	}

	// The handler, not only the service, guarantees "empty list, not absent": a service that
	// returns a gate with nil lists must still produce [] in the response.
	t.Run("nil lists from the service become empty lists", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().ListHooks(mock.Anything, ns, "rb-1").Return(&openchoreov1alpha1.DeploymentGateStatus{Key: "k"}, nil)
		resp, err := newHandlerWithMockRBService(svc).ListReleaseBindingHooks(ctx, req)
		require.NoError(t, err)
		typed, ok := resp.(gen.ListReleaseBindingHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "k", ptr.Deref(typed.Key, ""))
		require.NotNil(t, typed.PreDeploy)
		require.NotNil(t, typed.PostDeploy)
		assert.Empty(t, *typed.PreDeploy)
		assert.Empty(t, *typed.PostDeploy)
	})
}

// TestRetryReleaseBindingHookHandlerErrorBodies checks request validation (through the real
// service) and the exact error mapping of every service failure.
func TestRetryReleaseBindingHookHandlerErrorBodies(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	// An unknown or empty phase must be rejected before the binding is touched, with a message
	// that tells the caller which phases exist.
	t.Run("invalid or missing phase returns 400", func(t *testing.T) {
		for phase, wantMsg := range map[gen.HookRetryRequestPhase]string{
			"sideways": `invalid hook phase "sideways": must be preDeploy or postDeploy`,
			"":         `invalid hook phase "": must be preDeploy or postDeploy`,
		} {
			h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &allowAllPDP{}))
			resp, err := h.RetryReleaseBindingHook(ctx, gen.RetryReleaseBindingHookRequestObject{
				NamespaceName: ns, ReleaseBindingName: "rb-1", HookName: "image-scan",
				Body: &gen.HookRetryRequest{Phase: phase},
			})
			require.NoError(t, err)
			assert.Equal(t, gen.RetryReleaseBindingHook400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
				Code: gen.BADREQUEST, Error: wantMsg}}, resp)
		}
	})

	t.Run("nil body message", func(t *testing.T) {
		resp, err := newHandlerWithMockRBService(releasebindingmocks.NewMockService(t)).RetryReleaseBindingHook(ctx,
			gen.RetryReleaseBindingHookRequestObject{NamespaceName: ns, ReleaseBindingName: "rb-1", HookName: "x"})
		require.NoError(t, err)
		assert.Equal(t, gen.RetryReleaseBindingHook400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
			Code: gen.BADREQUEST, Error: "Request body is required"}}, resp)
	})

	tests := []struct {
		name   string
		svcErr error
		want   gen.RetryReleaseBindingHookResponseObject
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden,
			gen.RetryReleaseBindingHook403JSONResponse{ForbiddenJSONResponse: gen.ForbiddenJSONResponse{
				Code: gen.FORBIDDEN, Error: "You do not have permission to perform this operation"}}},
		{"binding not found -> 404", releasebindingsvc.ErrReleaseBindingNotFound,
			gen.RetryReleaseBindingHook404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse{
				Code: gen.NOTFOUND, Error: "ReleaseBinding not found"}}},
		// A missing hook is a different 404 from a missing binding, so the caller can tell them apart.
		{"hook not found -> 404 Hook", fmt.Errorf("wrapped: %w", releasebindingsvc.ErrHookNotFound),
			gen.RetryReleaseBindingHook404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse{
				Code: gen.NOTFOUND, Error: "Hook not found"}}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "hook name is required", StatusCode: http.StatusBadRequest},
			gen.RetryReleaseBindingHook400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
				Code: gen.BADREQUEST, Error: "hook name is required"}}},
		{"unexpected -> 500", fmt.Errorf("failed to annotate release binding: %s", internalLeak),
			gen.RetryReleaseBindingHook500JSONResponse{InternalErrorJSONResponse: gen.InternalErrorJSONResponse{
				Code: gen.INTERNALERROR, Error: "Internal server error"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := releasebindingmocks.NewMockService(t)
			svc.EXPECT().RetryHook(mock.Anything, ns, "rb-1", "postDeploy", "smoke").Return(nil, tt.svcErr)
			resp, err := newHandlerWithMockRBService(svc).RetryReleaseBindingHook(ctx, gen.RetryReleaseBindingHookRequestObject{
				NamespaceName: ns, ReleaseBindingName: "rb-1", HookName: "smoke",
				Body: &gen.HookRetryRequest{Phase: gen.PostDeploy},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, resp)
		})
	}
}

// TestAcknowledgeReleaseBindingGateHandlerErrorBodies checks every acknowledge failure maps to
// the documented status with an exact body.
func TestAcknowledgeReleaseBindingGateHandlerErrorBodies(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	// An empty key is rejected by the service; it must reach the caller as a 400, not a 500.
	t.Run("missing key returns 400", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &allowAllPDP{}))
		resp, err := h.AcknowledgeReleaseBindingGate(ctx, gen.AcknowledgeReleaseBindingGateRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1", Body: &gen.GateAcknowledgeRequest{},
		})
		require.NoError(t, err)
		assert.Equal(t, gen.AcknowledgeReleaseBindingGate400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
			Code: gen.BADREQUEST, Error: "gate key is required"}}, resp)
	})

	// The happy path must return the annotated binding and leave the gate status as it was.
	t.Run("success returns the binding with its gate", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &allowAllPDP{}))
		resp, err := h.AcknowledgeReleaseBindingGate(ctx, gen.AcknowledgeReleaseBindingGateRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1", Body: &gen.GateAcknowledgeRequest{Key: "key-1"},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.AcknowledgeReleaseBindingGate200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "rb-1", typed.Metadata.Name)
		require.NotNil(t, typed.Status)
		require.NotNil(t, typed.Status.Gate)
		assert.Equal(t, "key-1", ptr.Deref(typed.Status.Gate.Key, ""))
		require.NotNil(t, typed.Status.Gate.PreDeploy)
		require.Len(t, *typed.Status.Gate.PreDeploy, 1)
		assert.Equal(t, "image-scan", (*typed.Status.Gate.PreDeploy)[0].Name)
	})

	tests := []struct {
		name   string
		svcErr error
		want   gen.AcknowledgeReleaseBindingGateResponseObject
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden,
			gen.AcknowledgeReleaseBindingGate403JSONResponse{ForbiddenJSONResponse: gen.ForbiddenJSONResponse{
				Code: gen.FORBIDDEN, Error: "You do not have permission to perform this operation"}}},
		{"not found -> 404", releasebindingsvc.ErrReleaseBindingNotFound,
			gen.AcknowledgeReleaseBindingGate404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse{
				Code: gen.NOTFOUND, Error: "ReleaseBinding not found"}}},
		// A stale key is the caller's mistake (400), and the message says why.
		{"key mismatch -> 400", releasebindingsvc.ErrGateKeyMismatch,
			gen.AcknowledgeReleaseBindingGate400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
				Code: gen.BADREQUEST, Error: "gate key does not match the release binding's current gate"}}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "gate key is required", StatusCode: http.StatusBadRequest},
			gen.AcknowledgeReleaseBindingGate400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
				Code: gen.BADREQUEST, Error: "gate key is required"}}},
		{"unexpected -> 500", fmt.Errorf("failed to annotate release binding: %s", internalLeak),
			gen.AcknowledgeReleaseBindingGate500JSONResponse{InternalErrorJSONResponse: gen.InternalErrorJSONResponse{
				Code: gen.INTERNALERROR, Error: "Internal server error"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := releasebindingmocks.NewMockService(t)
			svc.EXPECT().AcknowledgeGate(mock.Anything, ns, "rb-1", "key-1").Return(nil, tt.svcErr)
			resp, err := newHandlerWithMockRBService(svc).AcknowledgeReleaseBindingGate(ctx, gen.AcknowledgeReleaseBindingGateRequestObject{
				NamespaceName: ns, ReleaseBindingName: "rb-1", Body: &gen.GateAcknowledgeRequest{Key: "key-1"},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, resp)
		})
	}
}

// TestReleaseBindingHooksMalformedJSON drives the real router: a body that is not JSON must be
// rejected with 400 before the service is called (the mock fails the test on any call).
func TestReleaseBindingHooksMalformedJSON(t *testing.T) {
	for _, path := range []string{
		"/api/v1/namespaces/ns-1/releasebindings/rb-1/hooks/scan/retry",
		"/api/v1/namespaces/ns-1/releasebindings/rb-1/gate/acknowledge",
	} {
		t.Run(path, func(t *testing.T) {
			svc := releasebindingmocks.NewMockService(t)
			h := newTestHTTPHandler(t, &handlerservices.Services{ReleaseBindingService: svc})
			_, rec := doRequest(t, h, http.MethodPost, path, []byte(`{"phase":`))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "can't decode JSON body")
		})
	}
}
