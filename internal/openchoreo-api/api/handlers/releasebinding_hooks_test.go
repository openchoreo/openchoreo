// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	releasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
)

func testGatedReleaseBindingObj() *openchoreov1alpha1.ReleaseBinding {
	rb := testReleaseBindingObj("rb-1")
	rb.Status.Gate = &openchoreov1alpha1.DeploymentGateStatus{
		Key: "key-1",
		PreDeploy: []openchoreov1alpha1.DeploymentHookStatus{
			{Name: "image-scan", Phase: openchoreov1alpha1.HookPhaseFailed, Reason: "HookFailed", Message: "3 CRITICAL CVEs"},
		},
	}
	return rb
}

func TestListReleaseBindingHooksHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("returns the gate", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &allowAllPDP{}))
		resp, err := h.ListReleaseBindingHooks(ctx, gen.ListReleaseBindingHooksRequestObject{NamespaceName: ns, ReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListReleaseBindingHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "key-1", ptr.Deref(typed.Key, ""))
		require.NotNil(t, typed.PreDeploy)
		require.Len(t, *typed.PreDeploy, 1)
		assert.Equal(t, "3 CRITICAL CVEs", ptr.Deref((*typed.PreDeploy)[0].Message, ""))
	})

	t.Run("binding without hooks returns empty lists", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testReleaseBindingObj("rb-1")}, &allowAllPDP{}))
		resp, err := h.ListReleaseBindingHooks(ctx, gen.ListReleaseBindingHooksRequestObject{NamespaceName: ns, ReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListReleaseBindingHooks200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.NotNil(t, typed.PreDeploy)
		assert.Empty(t, *typed.PreDeploy)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, nil, &allowAllPDP{}))
		resp, err := h.ListReleaseBindingHooks(ctx, gen.ListReleaseBindingHooksRequestObject{NamespaceName: ns, ReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.ListReleaseBindingHooks404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &denyAllPDP{}))
		resp, err := h.ListReleaseBindingHooks(ctx, gen.ListReleaseBindingHooksRequestObject{NamespaceName: ns, ReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.ListReleaseBindingHooks403JSONResponse{}, resp)
	})
}

func TestRetryReleaseBindingHookHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success annotates and returns the binding", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &allowAllPDP{}))
		resp, err := h.RetryReleaseBindingHook(ctx, gen.RetryReleaseBindingHookRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1", HookName: "image-scan",
			Body: &gen.HookRetryRequest{Phase: gen.PreDeploy},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.RetryReleaseBindingHook200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.NotNil(t, typed.Metadata.Annotations)
		assert.Equal(t, "preDeploy/image-scan", (*typed.Metadata.Annotations)[labels.AnnotationKeyHookRetry])
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, nil, &allowAllPDP{}))
		resp, err := h.RetryReleaseBindingHook(ctx, gen.RetryReleaseBindingHookRequestObject{NamespaceName: ns, ReleaseBindingName: "rb-1", HookName: "x"})
		require.NoError(t, err)
		assert.IsType(t, gen.RetryReleaseBindingHook400JSONResponse{}, resp)
	})

	t.Run("hook not in gate returns 404", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &allowAllPDP{}))
		resp, err := h.RetryReleaseBindingHook(ctx, gen.RetryReleaseBindingHookRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1", HookName: "nope",
			Body: &gen.HookRetryRequest{Phase: gen.PreDeploy},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.RetryReleaseBindingHook404JSONResponse{}, resp)
	})

	t.Run("maps service errors", func(t *testing.T) {
		tests := []struct {
			name    string
			svcErr  error
			wantTyp any
		}{
			{"forbidden -> 403", svcpkg.ErrForbidden, gen.RetryReleaseBindingHook403JSONResponse{}},
			{"binding not found -> 404", releasebindingsvc.ErrReleaseBindingNotFound, gen.RetryReleaseBindingHook404JSONResponse{}},
			{"validation -> 400", &svcpkg.ValidationError{Msg: "bad", StatusCode: 400}, gen.RetryReleaseBindingHook400JSONResponse{}},
			{"internal -> 500", assert.AnError, gen.RetryReleaseBindingHook500JSONResponse{}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc := releasebindingmocks.NewMockService(t)
				svc.EXPECT().RetryHook(mock.Anything, ns, "rb-1", "postDeploy", "smoke").Return(nil, tt.svcErr)
				h := &Handler{services: &handlerservices.Services{ReleaseBindingService: svc}, logger: slog.Default()}
				resp, err := h.RetryReleaseBindingHook(ctx, gen.RetryReleaseBindingHookRequestObject{
					NamespaceName: ns, ReleaseBindingName: "rb-1", HookName: "smoke",
					Body: &gen.HookRetryRequest{Phase: gen.PostDeploy},
				})
				require.NoError(t, err)
				assert.IsType(t, tt.wantTyp, resp)
			})
		}
	})
}

func TestAcknowledgeReleaseBindingGateHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success annotates with the key", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &allowAllPDP{}))
		resp, err := h.AcknowledgeReleaseBindingGate(ctx, gen.AcknowledgeReleaseBindingGateRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1", Body: &gen.GateAcknowledgeRequest{Key: "key-1"},
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.AcknowledgeReleaseBindingGate200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "key-1", (*typed.Metadata.Annotations)[labels.AnnotationKeyGateAcknowledged])
	})

	t.Run("stale key returns 400", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &allowAllPDP{}))
		resp, err := h.AcknowledgeReleaseBindingGate(ctx, gen.AcknowledgeReleaseBindingGateRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1", Body: &gen.GateAcknowledgeRequest{Key: "old"},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.AcknowledgeReleaseBindingGate400JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, nil, &allowAllPDP{}))
		resp, err := h.AcknowledgeReleaseBindingGate(ctx, gen.AcknowledgeReleaseBindingGateRequestObject{NamespaceName: ns, ReleaseBindingName: "rb-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.AcknowledgeReleaseBindingGate400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, []client.Object{testGatedReleaseBindingObj()}, &denyAllPDP{}))
		resp, err := h.AcknowledgeReleaseBindingGate(ctx, gen.AcknowledgeReleaseBindingGateRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1", Body: &gen.GateAcknowledgeRequest{Key: "key-1"},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.AcknowledgeReleaseBindingGate403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h := newHandlerWithReleaseBindingService(newReleaseBindingService(t, nil, &allowAllPDP{}))
		resp, err := h.AcknowledgeReleaseBindingGate(ctx, gen.AcknowledgeReleaseBindingGateRequestObject{
			NamespaceName: ns, ReleaseBindingName: "rb-1", Body: &gen.GateAcknowledgeRequest{Key: "key-1"},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.AcknowledgeReleaseBindingGate404JSONResponse{}, resp)
	})
}
