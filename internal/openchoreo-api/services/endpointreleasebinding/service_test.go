// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace    = "test-ns"
	testProject      = "test-project"
	testEndpointName = "test-r"
	testEnvironment  = "dev"
	testBindingName  = "test-r-dev"
)

// newService creates a service plus the Endpoint fixture validateEndpointExists relies on.
func newService(t *testing.T, extra ...client.Object) Service {
	t.Helper()
	objs := make([]client.Object, 0, 1+len(extra))
	objs = append(objs, testutil.NewEndpoint(testNamespace, testProject, testEndpointName))
	objs = append(objs, extra...)
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateEndpointReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, testBindingName)

		result, err := svc.CreateEndpointReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, endpointReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testBindingName, result.Name)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, testProject, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testEndpointName, result.Labels[labels.LabelKeyEndpointCRName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateEndpointReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("endpoint not found", func(t *testing.T) {
		svc := NewService(testutil.NewFakeClient(), testutil.TestLogger())
		rb := testutil.NewEndpointReleaseBinding(testNamespace, testProject, "missing-endpoint", testEnvironment, testBindingName)

		_, err := svc.CreateEndpointReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, "dup")
		svc := newService(t, existing)
		dup := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, "dup")

		_, err := svc.CreateEndpointReleaseBinding(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrEndpointReleaseBindingAlreadyExists)
	})

	t.Run("namespace overrides body namespace", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewEndpointReleaseBinding("ignored", testProject, testEndpointName, testEnvironment, testBindingName)

		result, err := svc.CreateEndpointReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})
}

func TestUpdateEndpointReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, testBindingName)
		svc := newService(t, existing)

		update := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, testBindingName)
		update.Spec.EndpointRelease = "test-r-abc123"

		result, err := svc.UpdateEndpointReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, endpointReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-r-abc123", result.Spec.EndpointRelease)
		assert.Equal(t, testProject, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testEndpointName, result.Labels[labels.LabelKeyEndpointCRName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateEndpointReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, "nonexistent")

		_, err := svc.UpdateEndpointReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrEndpointReleaseBindingNotFound)
	})
}

func TestListEndpointReleaseBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("all without endpoint filter", func(t *testing.T) {
		rb1 := testutil.NewEndpointReleaseBinding(testNamespace, testProject, "r-a", "dev", "rb-1")
		rb2 := testutil.NewEndpointReleaseBinding(testNamespace, testProject, "r-b", "dev", "rb-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListEndpointReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, endpointReleaseBindingTypeMeta, item.TypeMeta)
		}
	})

	t.Run("with endpoint filter", func(t *testing.T) {
		rb1 := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, "dev", "rb-1")
		rb2 := testutil.NewEndpointReleaseBinding(testNamespace, testProject, "other-r", "dev", "rb-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListEndpointReleaseBindings(ctx, testNamespace, testEndpointName, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, testEndpointName, result.Items[0].Spec.Owner.EndpointName)
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListEndpointReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListEndpointReleaseBindings(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetEndpointReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		rb := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, testBindingName)
		svc := newService(t, rb)

		result, err := svc.GetEndpointReleaseBinding(ctx, testNamespace, testBindingName)
		require.NoError(t, err)
		assert.Equal(t, endpointReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testBindingName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetEndpointReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEndpointReleaseBindingNotFound)
	})
}

func TestDeleteEndpointReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rb := testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, testBindingName)
		svc := newService(t, rb)

		err := svc.DeleteEndpointReleaseBinding(ctx, testNamespace, testBindingName)
		require.NoError(t, err)

		_, err = svc.GetEndpointReleaseBinding(ctx, testNamespace, testBindingName)
		require.ErrorIs(t, err, ErrEndpointReleaseBindingNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteEndpointReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEndpointReleaseBindingNotFound)
	})
}
