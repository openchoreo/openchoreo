// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

import (
	"context"
	"errors"
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

func newServiceWithInterceptor(t *testing.T, funcs interceptor.Funcs) Service {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(testutil.NewScheme()).WithInterceptorFuncs(funcs).Build()
	return NewService(c, testutil.TestLogger())
}

var invalidErr = apierrors.NewInvalid(
	schema.GroupKind{Group: "openchoreo.dev", Kind: "ProjectRelease"},
	"bad", field.ErrorList{field.Invalid(field.NewPath("spec"), "x", "boom")},
)

func TestNewServiceWithAuthz_Constructor(t *testing.T) {
	require.NotNil(t, NewServiceWithAuthz(testutil.NewFakeClient(), nil, testutil.TestLogger()))
}

func TestProjectReleaseService_ClientErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("create wraps Invalid as ValidationError", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return invalidErr
			},
		})
		_, err := svc.CreateProjectRelease(ctx, testNamespace, testutil.NewProjectRelease(testNamespace, testProjectName, "pr"))
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
	})

	t.Run("create existence-check error", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.CreateProjectRelease(ctx, testNamespace, testutil.NewProjectRelease(testNamespace, testProjectName, "pr"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check project release existence")
	})

	t.Run("list error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.ListProjectReleases(ctx, testNamespace, "", services.ListOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list project releases")
	})

	t.Run("get error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.GetProjectRelease(ctx, testNamespace, "pr")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get project release")
	})

	t.Run("delete error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return errors.New("boom")
			},
		})
		err := svc.DeleteProjectRelease(ctx, testNamespace, "pr")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete project release")
	})

	t.Run("create AlreadyExists on apply", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return apierrors.NewAlreadyExists(schema.GroupResource{Group: "openchoreo.dev", Resource: "projectreleases"}, "pr")
			},
		})
		_, err := svc.CreateProjectRelease(ctx, testNamespace, testutil.NewProjectRelease(testNamespace, testProjectName, "pr"))
		require.ErrorIs(t, err, ErrProjectReleaseAlreadyExists)
	})

	t.Run("create generic error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.CreateProjectRelease(ctx, testNamespace, testutil.NewProjectRelease(testNamespace, testProjectName, "pr"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create project release")
	})

	t.Run("list surfaces remaining item count", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, obj client.ObjectList, _ ...client.ListOption) error {
				l := obj.(*openchoreov1alpha1.ProjectReleaseList)
				remaining := int64(7)
				l.RemainingItemCount = &remaining
				return nil
			},
		})
		result, err := svc.ListProjectReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		require.NotNil(t, result.RemainingCount)
		assert.Equal(t, int64(7), *result.RemainingCount)
	})
}
