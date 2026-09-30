// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newServiceWithInterceptor(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) Service {
	t.Helper()
	c := fake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithObjects(objs...).
		WithInterceptorFuncs(funcs).
		Build()
	return NewService(c, testutil.TestLogger())
}

var (
	errBoom    = errors.New("boom")
	invalidErr = apierrors.NewInvalid(
		schema.GroupKind{Group: "openchoreo.dev", Kind: "Hook"},
		testHookName, field.ErrorList{field.Invalid(field.NewPath("spec", "workflowRef"), "x", "workflow ref is required")},
	)
)

func TestHookService_ClientErrors(t *testing.T) {
	ctx := context.Background()

	// CRD/webhook rejections must reach the handler as a ValidationError carrying the
	// API server's 422, so the caller sees the field message instead of a 500.
	t.Run("create maps Invalid to a 422 ValidationError", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return invalidErr
			},
		})
		_, err := svc.CreateHook(ctx, testNamespace, newHook(testNamespace, testHookName))
		vErr, ok := errors.AsType[*services.ValidationError](err)
		require.True(t, ok, "expected ValidationError, got %v", err)
		assert.Equal(t, http.StatusUnprocessableEntity, vErr.StatusCode)
		assert.Contains(t, vErr.Msg, "workflow ref is required")
	})

	// Unexpected client failures must not be mistaken for a conflict or validation
	// problem; they are wrapped so the handler maps them to 500.
	t.Run("create wraps unexpected errors", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return errBoom
			},
		})
		_, err := svc.CreateHook(ctx, testNamespace, newHook(testNamespace, testHookName))
		require.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, ErrHookAlreadyExists)
		_, isValidation := errors.AsType[*services.ValidationError](err)
		assert.False(t, isValidation)
		assert.Contains(t, err.Error(), "failed to create hook")
	})

	// A read failure while loading the existing hook must not be reported as not-found,
	// otherwise a transient API error would tell the user the hook does not exist.
	t.Run("update wraps get errors other than not-found", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errBoom
			},
		})
		_, err := svc.UpdateHook(ctx, testNamespace, newHook(testNamespace, testHookName))
		require.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, ErrHookNotFound)
		assert.Contains(t, err.Error(), "failed to get hook")
	})

	t.Run("update maps Invalid to a 422 ValidationError", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return invalidErr
			},
		}, newHook(testNamespace, testHookName))
		_, err := svc.UpdateHook(ctx, testNamespace, newHook(testNamespace, testHookName))
		vErr, ok := errors.AsType[*services.ValidationError](err)
		require.True(t, ok, "expected ValidationError, got %v", err)
		assert.Equal(t, http.StatusUnprocessableEntity, vErr.StatusCode)
	})

	t.Run("update wraps unexpected update errors", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return errBoom
			},
		}, newHook(testNamespace, testHookName))
		_, err := svc.UpdateHook(ctx, testNamespace, newHook(testNamespace, testHookName))
		require.ErrorIs(t, err, errBoom)
		assert.Contains(t, err.Error(), "failed to update hook")
	})

	t.Run("list wraps client errors", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errBoom
			},
		})
		_, err := svc.ListHooks(ctx, testNamespace, services.ListOptions{})
		require.ErrorIs(t, err, errBoom)
		assert.Contains(t, err.Error(), "failed to list hooks")
	})

	t.Run("get wraps errors other than not-found", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errBoom
			},
		})
		_, err := svc.GetHook(ctx, testNamespace, testHookName)
		require.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, ErrHookNotFound)
	})

	t.Run("delete wraps errors other than not-found", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return errBoom
			},
		})
		err := svc.DeleteHook(ctx, testNamespace, testHookName)
		require.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, ErrHookNotFound)
		assert.Contains(t, err.Error(), "failed to delete hook")
	})
}

// Pagination is how the portal pages through hooks: the limit and cursor must reach the
// API server, and the continue token and remaining count must come back to the caller.
func TestListHooks_Pagination(t *testing.T) {
	var gotOpts client.ListOptions
	svc := newServiceWithInterceptor(t, interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			gotOpts.ApplyOptions(opts)
			if err := c.List(ctx, list, opts...); err != nil {
				return err
			}
			hl := list.(*openchoreov1alpha1.HookList)
			hl.Continue = "next-page"
			remaining := int64(7)
			hl.RemainingItemCount = &remaining
			return nil
		},
	}, newHook(testNamespace, testHookName))

	result, err := svc.ListHooks(context.Background(), testNamespace, services.ListOptions{Limit: 5, Cursor: "page-2"})
	require.NoError(t, err)

	assert.Equal(t, testNamespace, gotOpts.Namespace)
	assert.Equal(t, int64(5), gotOpts.Limit)
	assert.Equal(t, "page-2", gotOpts.Continue)
	assert.Equal(t, "next-page", result.NextCursor)
	require.NotNil(t, result.RemainingCount)
	assert.Equal(t, int64(7), *result.RemainingCount)
	require.Len(t, result.Items, 1)
	assert.Equal(t, hookTypeMeta, result.Items[0].TypeMeta)
}
