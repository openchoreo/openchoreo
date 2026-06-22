// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

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

// newErrSvc builds a Service over a fake client with the given interceptor funcs
// and seed objects, so apiserver error paths can be simulated.
func newErrSvc(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) Service {
	t.Helper()
	c := fake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithObjects(objs...).
		WithInterceptorFuncs(funcs).
		Build()
	return NewService(c, testutil.TestLogger())
}

var invalidErr = apierrors.NewInvalid(
	schema.GroupKind{Group: "openchoreo.dev", Kind: "ProjectReleaseBinding"},
	"bad", field.ErrorList{field.Invalid(field.NewPath("spec"), "x", "boom")},
)

func TestNewServiceWithAuthz_Constructor(t *testing.T) {
	require.NotNil(t, NewServiceWithAuthz(testutil.NewFakeClient(), nil, testutil.TestLogger()))
}

func TestProjectReleaseBindingService_ClientErrors(t *testing.T) {
	ctx := context.Background()
	newBinding := func() *openchoreov1alpha1.ProjectReleaseBinding {
		return testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "my-app-dev")
	}

	t.Run("create wraps project-validation error", func(t *testing.T) {
		svc := newErrSvc(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, newBinding())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to validate project")
	})

	t.Run("create wraps binding-existence-check error", func(t *testing.T) {
		project := testutil.NewProject(testNamespace, testProjectName)
		svc := newErrSvc(t, interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				// Let the Project lookup succeed; fail only the binding lookup.
				if _, ok := obj.(*openchoreov1alpha1.ProjectReleaseBinding); ok {
					return errors.New("boom")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}, project)
		_, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, newBinding())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check binding existence")
	})

	t.Run("create wraps Invalid as ValidationError", func(t *testing.T) {
		project := testutil.NewProject(testNamespace, testProjectName)
		svc := newErrSvc(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return invalidErr
			},
		}, project)
		_, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, newBinding())
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
	})

	t.Run("create AlreadyExists on apply", func(t *testing.T) {
		project := testutil.NewProject(testNamespace, testProjectName)
		svc := newErrSvc(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return apierrors.NewAlreadyExists(
					schema.GroupResource{Group: "openchoreo.dev", Resource: "projectreleasebindings"}, "my-app-dev")
			},
		}, project)
		_, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, newBinding())
		require.ErrorIs(t, err, ErrProjectReleaseBindingAlreadyExists)
	})

	t.Run("create generic error wrapped", func(t *testing.T) {
		project := testutil.NewProject(testNamespace, testProjectName)
		svc := newErrSvc(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return errors.New("boom")
			},
		}, project)
		_, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, newBinding())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create project release binding")
	})

	t.Run("update get error wrapped", func(t *testing.T) {
		svc := newErrSvc(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.UpdateProjectReleaseBinding(ctx, testNamespace, newBinding())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get project release binding")
	})

	t.Run("update wraps Invalid as ValidationError", func(t *testing.T) {
		existing := newBinding()
		svc := newErrSvc(t, interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return invalidErr
			},
		}, existing)
		_, err := svc.UpdateProjectReleaseBinding(ctx, testNamespace, newBinding())
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
	})

	t.Run("update generic error wrapped", func(t *testing.T) {
		existing := newBinding()
		svc := newErrSvc(t, interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return errors.New("boom")
			},
		}, existing)
		_, err := svc.UpdateProjectReleaseBinding(ctx, testNamespace, newBinding())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update project release binding")
	})

	t.Run("list error wrapped", func(t *testing.T) {
		svc := newErrSvc(t, interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.ListProjectReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list project release bindings")
	})

	t.Run("list surfaces remaining item count", func(t *testing.T) {
		svc := newErrSvc(t, interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, obj client.ObjectList, _ ...client.ListOption) error {
				l := obj.(*openchoreov1alpha1.ProjectReleaseBindingList)
				remaining := int64(7)
				l.RemainingItemCount = &remaining
				return nil
			},
		})
		result, err := svc.ListProjectReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		require.NotNil(t, result.RemainingCount)
		assert.Equal(t, int64(7), *result.RemainingCount)
	})

	t.Run("get error wrapped", func(t *testing.T) {
		svc := newErrSvc(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.GetProjectReleaseBinding(ctx, testNamespace, "my-app-dev")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get project release binding")
	})

	t.Run("delete error wrapped", func(t *testing.T) {
		svc := newErrSvc(t, interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return errors.New("boom")
			},
		})
		err := svc.DeleteProjectReleaseBinding(ctx, testNamespace, "my-app-dev")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete project release binding")
	})
}
