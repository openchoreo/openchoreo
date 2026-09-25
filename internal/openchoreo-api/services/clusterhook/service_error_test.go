// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

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
		schema.GroupKind{Group: "openchoreo.dev", Kind: "ClusterHook"},
		testHookName, field.ErrorList{field.Invalid(field.NewPath("spec", "workflowRef", "kind"), "Workflow", "a ClusterHook may only reference a ClusterWorkflow")},
	)
)

func TestClusterHookService_ClientErrors(t *testing.T) {
	ctx := context.Background()

	// The CRD rule that a ClusterHook may only reference a ClusterWorkflow is enforced by
	// the API server; that rejection must surface as a 422 ValidationError with its
	// message, not as a 500.
	t.Run("create maps Invalid to a 422 ValidationError", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return invalidErr
			},
		})
		_, err := svc.CreateClusterHook(ctx, newClusterHook(testHookName))
		vErr, ok := errors.AsType[*services.ValidationError](err)
		require.True(t, ok, "expected ValidationError, got %v", err)
		assert.Equal(t, http.StatusUnprocessableEntity, vErr.StatusCode)
		assert.Contains(t, vErr.Msg, "may only reference a ClusterWorkflow")
	})

	// Unexpected client failures are wrapped so the handler maps them to 500 rather
	// than a misleading conflict or validation response.
	t.Run("create wraps unexpected errors", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return errBoom
			},
		})
		_, err := svc.CreateClusterHook(ctx, newClusterHook(testHookName))
		require.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, ErrClusterHookAlreadyExists)
		_, isValidation := errors.AsType[*services.ValidationError](err)
		assert.False(t, isValidation)
		assert.Contains(t, err.Error(), "failed to create cluster hook")
	})

	t.Run("update nil input", func(t *testing.T) {
		_, err := newService(t).UpdateClusterHook(ctx, nil)
		require.Error(t, err)
	})

	// A transient read failure must not be reported as not-found, otherwise the portal
	// would tell the user the hook does not exist.
	t.Run("update wraps get errors other than not-found", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errBoom
			},
		})
		_, err := svc.UpdateClusterHook(ctx, newClusterHook(testHookName))
		require.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, ErrClusterHookNotFound)
		assert.Contains(t, err.Error(), "failed to get cluster hook")
	})

	t.Run("update maps Invalid to a 422 ValidationError", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return invalidErr
			},
		}, newClusterHook(testHookName))
		_, err := svc.UpdateClusterHook(ctx, newClusterHook(testHookName))
		vErr, ok := errors.AsType[*services.ValidationError](err)
		require.True(t, ok, "expected ValidationError, got %v", err)
		assert.Equal(t, http.StatusUnprocessableEntity, vErr.StatusCode)
	})

	t.Run("update wraps unexpected update errors", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return errBoom
			},
		}, newClusterHook(testHookName))
		_, err := svc.UpdateClusterHook(ctx, newClusterHook(testHookName))
		require.ErrorIs(t, err, errBoom)
		assert.Contains(t, err.Error(), "failed to update cluster hook")
	})

	// A malformed selector is caller error and must come back as a ValidationError (400),
	// not be sent to the API server.
	t.Run("list rejects an invalid label selector", func(t *testing.T) {
		_, err := newService(t).ListClusterHooks(ctx, services.ListOptions{LabelSelector: "===invalid"})
		_, ok := errors.AsType[*services.ValidationError](err)
		require.True(t, ok, "expected ValidationError, got %v", err)
	})

	t.Run("list wraps client errors", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errBoom
			},
		})
		_, err := svc.ListClusterHooks(ctx, services.ListOptions{})
		require.ErrorIs(t, err, errBoom)
		assert.Contains(t, err.Error(), "failed to list cluster hooks")
	})

	t.Run("get wraps errors other than not-found", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errBoom
			},
		})
		_, err := svc.GetClusterHook(ctx, testHookName)
		require.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, ErrClusterHookNotFound)
	})

	t.Run("delete wraps errors other than not-found", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return errBoom
			},
		})
		err := svc.DeleteClusterHook(ctx, testHookName)
		require.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, ErrClusterHookNotFound)
		assert.Contains(t, err.Error(), "failed to delete cluster hook")
	})
}

// Pagination is how the portal pages through cluster hooks: limit and cursor must reach
// the API server, and the continue token and remaining count must come back.
func TestListClusterHooks_Pagination(t *testing.T) {
	var gotOpts client.ListOptions
	svc := newServiceWithInterceptor(t, interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			gotOpts.ApplyOptions(opts)
			if err := c.List(ctx, list, opts...); err != nil {
				return err
			}
			hl := list.(*openchoreov1alpha1.ClusterHookList)
			hl.Continue = "next-page"
			remaining := int64(3)
			hl.RemainingItemCount = &remaining
			return nil
		},
	}, newClusterHook(testHookName))

	result, err := svc.ListClusterHooks(context.Background(), services.ListOptions{Limit: 5, Cursor: "page-2"})
	require.NoError(t, err)

	assert.Equal(t, int64(5), gotOpts.Limit)
	assert.Equal(t, "page-2", gotOpts.Continue)
	assert.Equal(t, "next-page", result.NextCursor)
	require.NotNil(t, result.RemainingCount)
	assert.Equal(t, int64(3), *result.RemainingCount)
	require.Len(t, result.Items, 1)
	assert.Equal(t, clusterHookTypeMeta, result.Items[0].TypeMeta)
}
