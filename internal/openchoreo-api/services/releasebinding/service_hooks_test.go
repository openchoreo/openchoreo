// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func gatedRB() *openchoreov1alpha1.ReleaseBinding {
	rb := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
	rb.Status.Gate = &openchoreov1alpha1.DeploymentGateStatus{
		Key: "key-1",
		PreDeploy: []openchoreov1alpha1.DeploymentHookStatus{
			{Name: "image-scan", Phase: openchoreov1alpha1.HookPhaseFailed},
		},
		PostDeploy: []openchoreov1alpha1.DeploymentHookStatus{
			{Name: "smoke", Phase: openchoreov1alpha1.HookPhaseFailed},
		},
	}
	return rb
}

func TestListHooks(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the gate with both phases", func(t *testing.T) {
		svc := newService(t, gatedRB())
		gate, err := svc.ListHooks(ctx, testNamespace, testRBName)
		require.NoError(t, err)
		assert.Equal(t, "key-1", gate.Key)
		assert.Len(t, gate.PreDeploy, 1)
		assert.Len(t, gate.PostDeploy, 1)
	})

	// A binding without hooks must answer with empty lists rather than nil, so API clients
	// can treat "no gate" and "gate with nothing bound" identically.
	t.Run("binding without a gate returns empty lists", func(t *testing.T) {
		svc := newService(t, testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName))
		gate, err := svc.ListHooks(ctx, testNamespace, testRBName)
		require.NoError(t, err)
		assert.NotNil(t, gate.PreDeploy)
		assert.NotNil(t, gate.PostDeploy)
		assert.Empty(t, gate.PreDeploy)
		assert.Empty(t, gate.PostDeploy)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := newService(t).ListHooks(ctx, testNamespace, testRBName)
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})
}

func TestRetryHook(t *testing.T) {
	ctx := context.Background()

	// The controller keys on "<phase>/<name>"; anything else would be ignored silently.
	t.Run("sets the retry annotation to phase/name", func(t *testing.T) {
		svc := newService(t, gatedRB())
		result, err := svc.RetryHook(ctx, testNamespace, testRBName, HookPhasePostDeploy, "smoke")
		require.NoError(t, err)
		assert.Equal(t, "postDeploy/smoke", result.Annotations[labels.AnnotationKeyHookRetry])
		assert.Equal(t, releaseBindingTypeMeta, result.TypeMeta)

		stored, err := svc.GetReleaseBinding(ctx, testNamespace, testRBName)
		require.NoError(t, err)
		assert.Equal(t, "postDeploy/smoke", stored.Annotations[labels.AnnotationKeyHookRetry], "annotation must be persisted")
	})

	t.Run("hook not in that phase is rejected", func(t *testing.T) {
		svc := newService(t, gatedRB())
		_, err := svc.RetryHook(ctx, testNamespace, testRBName, HookPhasePreDeploy, "smoke")
		require.ErrorIs(t, err, ErrHookNotFound)
	})

	t.Run("binding without a gate has nothing to retry", func(t *testing.T) {
		svc := newService(t, testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName))
		_, err := svc.RetryHook(ctx, testNamespace, testRBName, HookPhasePreDeploy, "image-scan")
		require.ErrorIs(t, err, ErrHookNotFound)
	})

	t.Run("invalid phase is a validation error", func(t *testing.T) {
		svc := newService(t, gatedRB())
		_, err := svc.RetryHook(ctx, testNamespace, testRBName, "sideways", "image-scan")
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := newService(t).RetryHook(ctx, testNamespace, testRBName, HookPhasePreDeploy, "image-scan")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})
}

func TestAcknowledgeGate(t *testing.T) {
	ctx := context.Background()

	t.Run("sets the acknowledged annotation to the current key", func(t *testing.T) {
		svc := newService(t, gatedRB())
		result, err := svc.AcknowledgeGate(ctx, testNamespace, testRBName, "key-1")
		require.NoError(t, err)
		assert.Equal(t, "key-1", result.Annotations[labels.AnnotationKeyGateAcknowledged])
	})

	// Acknowledging a stale key must fail: otherwise an old acknowledgement could silence a
	// failure that happened for a later release.
	t.Run("stale key is rejected", func(t *testing.T) {
		svc := newService(t, gatedRB())
		_, err := svc.AcknowledgeGate(ctx, testNamespace, testRBName, "key-0")
		require.ErrorIs(t, err, ErrGateKeyMismatch)
	})

	t.Run("binding without a gate is rejected", func(t *testing.T) {
		svc := newService(t, testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName))
		_, err := svc.AcknowledgeGate(ctx, testNamespace, testRBName, "key-1")
		require.ErrorIs(t, err, ErrGateKeyMismatch)
	})

	t.Run("empty key is a validation error", func(t *testing.T) {
		svc := newService(t, gatedRB())
		_, err := svc.AcknowledgeGate(ctx, testNamespace, testRBName, "")
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
	})
}

// newInterceptedService wraps the fake client so tests can inject API-server failures that
// the in-memory client never produces on its own.
func newInterceptedService(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) Service {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(testutil.NewScheme()).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
	return NewService(c, testutil.TestLogger())
}

func TestRetryHookValidationAndErrors(t *testing.T) {
	ctx := context.Background()

	// The phase is echoed back so the caller can see what they sent; the exact message is part
	// of the API's 400 body.
	t.Run("invalid phase message names the allowed phases", func(t *testing.T) {
		_, err := newService(t, gatedRB()).RetryHook(ctx, testNamespace, testRBName, "sideways", "image-scan")
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Equal(t, `invalid hook phase "sideways": must be preDeploy or postDeploy`, vErr.Msg)
		assert.Equal(t, http.StatusBadRequest, vErr.StatusCode)
	})

	// An empty hook name would write "preDeploy/" which the controller can never match.
	t.Run("empty hook name is a validation error", func(t *testing.T) {
		_, err := newService(t, gatedRB()).RetryHook(ctx, testNamespace, testRBName, HookPhasePreDeploy, "")
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Equal(t, "hook name is required", vErr.Msg)
		assert.Equal(t, http.StatusBadRequest, vErr.StatusCode)
	})

	// A pre-existing annotation map must be kept intact: the retry patch adds one key only.
	t.Run("keeps existing annotations", func(t *testing.T) {
		rb := gatedRB()
		rb.Annotations = map[string]string{"keep": "me"}
		result, err := newService(t, rb).RetryHook(ctx, testNamespace, testRBName, HookPhasePreDeploy, "image-scan")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"keep": "me", labels.AnnotationKeyHookRetry: "preDeploy/image-scan"}, result.Annotations)
	})

	// A Get failure other than NotFound must surface as an internal error, not as a 404.
	t.Run("get failure is wrapped", func(t *testing.T) {
		svc := newInterceptedService(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("etcd down")
			},
		}, gatedRB())
		_, err := svc.RetryHook(ctx, testNamespace, testRBName, HookPhasePreDeploy, "image-scan")
		require.EqualError(t, err, "failed to get release binding: etcd down")
		assert.NotErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	// The binding may be deleted between the read and the patch; that race must still be a 404.
	t.Run("patch not found maps to binding not found", func(t *testing.T) {
		svc := newInterceptedService(t, interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return apierrors.NewNotFound(schema.GroupResource{Group: "openchoreo.dev", Resource: "releasebindings"}, testRBName)
			},
		}, gatedRB())
		_, err := svc.RetryHook(ctx, testNamespace, testRBName, HookPhasePreDeploy, "image-scan")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	// Any other patch failure must be wrapped (so the handler returns 500) and the annotation
	// must not be reported as set.
	t.Run("patch failure is wrapped", func(t *testing.T) {
		svc := newInterceptedService(t, interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errors.New("conflict storm")
			},
		}, gatedRB())
		result, err := svc.RetryHook(ctx, testNamespace, testRBName, HookPhasePreDeploy, "image-scan")
		require.EqualError(t, err, "failed to annotate release binding: conflict storm")
		assert.Nil(t, result)
	})
}

func TestAcknowledgeGateErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("empty key message", func(t *testing.T) {
		_, err := newService(t, gatedRB()).AcknowledgeGate(ctx, testNamespace, testRBName, "")
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Equal(t, "gate key is required", vErr.Msg)
		assert.Equal(t, http.StatusBadRequest, vErr.StatusCode)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := newService(t).AcknowledgeGate(ctx, testNamespace, testRBName, "key-1")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	// The stored object must carry the acknowledgement, not just the returned copy, or the
	// controller would never see it.
	t.Run("acknowledgement is persisted", func(t *testing.T) {
		svc := newService(t, gatedRB())
		_, err := svc.AcknowledgeGate(ctx, testNamespace, testRBName, "key-1")
		require.NoError(t, err)
		stored, err := svc.GetReleaseBinding(ctx, testNamespace, testRBName)
		require.NoError(t, err)
		assert.Equal(t, "key-1", stored.Annotations[labels.AnnotationKeyGateAcknowledged])
	})

	t.Run("patch failure is wrapped", func(t *testing.T) {
		svc := newInterceptedService(t, interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errors.New("boom")
			},
		}, gatedRB())
		_, err := svc.AcknowledgeGate(ctx, testNamespace, testRBName, "key-1")
		require.EqualError(t, err, "failed to annotate release binding: boom")
	})
}

func TestListHooksGetError(t *testing.T) {
	// A transient read failure must not be mistaken for "no gate" (empty lists).
	svc := newInterceptedService(t, interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return errors.New("etcd down")
		},
	}, gatedRB())
	gate, err := svc.ListHooks(context.Background(), testNamespace, testRBName)
	require.EqualError(t, err, "failed to get release binding: etcd down")
	assert.Nil(t, gate)
}
