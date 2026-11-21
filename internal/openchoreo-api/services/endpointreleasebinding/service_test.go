// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

const newReleaseName = "new-release"

// TestUpdateEndpointReleaseBindingResourceVersion covers the optimistic concurrency of the update:
// a resourceVersion read earlier makes it conditional (#5370).
func TestUpdateEndpointReleaseBindingResourceVersion(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T) (client.Client, Service, *openchoreov1alpha1.EndpointReleaseBinding) {
		t.Helper()
		fc := testutil.NewFakeClient(testutil.NewEndpointReleaseBinding(testNamespace, testProject, testEndpointName, testEnvironment, testBindingName))
		svc := NewService(fc, testutil.TestLogger())
		read, err := svc.GetEndpointReleaseBinding(ctx, testNamespace, testBindingName)
		require.NoError(t, err)
		require.NotEmpty(t, read.ResourceVersion)
		return fc, svc, read
	}

	t.Run("stale resourceVersion is rejected and keeps the newer change", func(t *testing.T) {
		fc, svc, read := setup(t)

		// Someone else changes the binding after it was read.
		other := read.DeepCopy()
		other.Annotations = map[string]string{"example/marker": "x"}
		require.NoError(t, fc.Update(ctx, other))

		stale := read.DeepCopy()
		stale.Spec.EndpointRelease = newReleaseName
		_, err := svc.UpdateEndpointReleaseBinding(ctx, testNamespace, stale)
		require.ErrorIs(t, err, services.ErrConflict)

		stored := &openchoreov1alpha1.EndpointReleaseBinding{}
		require.NoError(t, fc.Get(ctx, client.ObjectKeyFromObject(read), stored))
		assert.Equal(t, "x", stored.Annotations["example/marker"])
		assert.Equal(t, read.Spec.EndpointRelease, stored.Spec.EndpointRelease)
	})

	t.Run("current resourceVersion succeeds", func(t *testing.T) {
		_, svc, read := setup(t)

		update := read.DeepCopy()
		update.Spec.EndpointRelease = newReleaseName
		result, err := svc.UpdateEndpointReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, newReleaseName, result.Spec.EndpointRelease)
		assert.NotEqual(t, read.ResourceVersion, result.ResourceVersion)
	})

	t.Run("no resourceVersion overwrites unconditionally", func(t *testing.T) {
		fc, svc, read := setup(t)

		other := read.DeepCopy()
		other.Annotations = map[string]string{"example/marker": "x"}
		require.NoError(t, fc.Update(ctx, other))

		update := read.DeepCopy()
		update.ResourceVersion = ""
		update.Spec.EndpointRelease = newReleaseName
		result, err := svc.UpdateEndpointReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, newReleaseName, result.Spec.EndpointRelease)
	})

	for _, rv := range []string{"abc", "0", "-1", "1.5", " 12"} {
		t.Run("malformed resourceVersion "+rv+" is a validation error", func(t *testing.T) {
			fc, svc, read := setup(t)

			update := read.DeepCopy()
			update.ResourceVersion = rv
			update.Spec.EndpointRelease = newReleaseName
			_, err := svc.UpdateEndpointReleaseBinding(ctx, testNamespace, update)
			validationErr, ok := errors.AsType[*services.ValidationError](err)
			require.True(t, ok, "expected a ValidationError, got %v", err)
			assert.Contains(t, validationErr.Msg, "metadata.resourceVersion")

			stored := &openchoreov1alpha1.EndpointReleaseBinding{}
			require.NoError(t, fc.Get(ctx, client.ObjectKeyFromObject(read), stored))
			assert.Equal(t, read.Spec.EndpointRelease, stored.Spec.EndpointRelease)
		})
	}
}
