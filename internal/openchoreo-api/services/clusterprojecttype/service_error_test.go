// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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
	schema.GroupKind{Group: "openchoreo.dev", Kind: "ClusterProjectType"},
	"bad", field.ErrorList{field.Invalid(field.NewPath("spec"), "x", "boom")},
)

func TestNewServiceWithAuthz_Constructor(t *testing.T) {
	require.NotNil(t, NewServiceWithAuthz(testutil.NewFakeClient(), nil, testutil.TestLogger()))
}

func TestClusterProjectTypeService_ClientErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("create wraps Invalid as ValidationError", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return invalidErr
			},
		})
		_, err := svc.CreateClusterProjectType(ctx, testutil.NewClusterProjectType("cpt"))
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
	})

	t.Run("create generic error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.CreateClusterProjectType(ctx, testutil.NewClusterProjectType("cpt"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create cluster project type")
	})

	t.Run("update of missing returns NotFound", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{})
		_, err := svc.UpdateClusterProjectType(ctx, testutil.NewClusterProjectType("cpt"))
		require.ErrorIs(t, err, ErrClusterProjectTypeNotFound)
	})

	t.Run("update wraps Invalid as ValidationError", func(t *testing.T) {
		c := fake.NewClientBuilder().
			WithScheme(testutil.NewScheme()).
			WithObjects(testutil.NewClusterProjectType("cpt")).
			WithInterceptorFuncs(interceptor.Funcs{
				Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
					return invalidErr
				},
			}).Build()
		svc := NewService(c, testutil.TestLogger())
		_, err := svc.UpdateClusterProjectType(ctx, testutil.NewClusterProjectType("cpt"))
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
	})

	t.Run("list error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.ListClusterProjectTypes(ctx, services.ListOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list cluster project types")
	})

	t.Run("get error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.GetClusterProjectType(ctx, "cpt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get cluster project type")
	})

	t.Run("delete error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return errors.New("boom")
			},
		})
		err := svc.DeleteClusterProjectType(ctx, "cpt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete cluster project type")
	})

	t.Run("update get error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("boom")
			},
		})
		_, err := svc.UpdateClusterProjectType(ctx, testutil.NewClusterProjectType("cpt"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get cluster project type")
	})

	t.Run("update generic error wrapped", func(t *testing.T) {
		c := fake.NewClientBuilder().
			WithScheme(testutil.NewScheme()).
			WithObjects(testutil.NewClusterProjectType("cpt")).
			WithInterceptorFuncs(interceptor.Funcs{
				Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
					return errors.New("boom")
				},
			}).Build()
		svc := NewService(c, testutil.TestLogger())
		_, err := svc.UpdateClusterProjectType(ctx, testutil.NewClusterProjectType("cpt"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update cluster project type")
	})

	t.Run("schema conversion error wrapped", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
				cpt := obj.(*openchoreov1alpha1.ClusterProjectType)
				cpt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
				}
				return nil
			},
		})
		_, err := svc.GetClusterProjectTypeSchema(ctx, "cpt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to convert to JSON schema")
	})

	t.Run("list surfaces remaining item count", func(t *testing.T) {
		svc := newServiceWithInterceptor(t, interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, obj client.ObjectList, _ ...client.ListOption) error {
				l := obj.(*openchoreov1alpha1.ClusterProjectTypeList)
				remaining := int64(7)
				l.RemainingItemCount = &remaining
				return nil
			},
		})
		result, err := svc.ListClusterProjectTypes(ctx, services.ListOptions{})
		require.NoError(t, err)
		require.NotNil(t, result.RemainingCount)
		assert.Equal(t, int64(7), *result.RemainingCount)
	})
}
