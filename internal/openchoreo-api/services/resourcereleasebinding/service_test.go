// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

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
	testResourceName = "test-r"
	testEnvironment  = "dev"
	testBindingName  = "test-r-dev"
)

// newService creates a service plus the Resource fixture validateResourceExists relies on.
func newService(t *testing.T, extra ...client.Object) Service {
	t.Helper()
	objs := make([]client.Object, 0, 1+len(extra))
	objs = append(objs, testutil.NewResource(testNamespace, testProject, testResourceName))
	objs = append(objs, extra...)
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateResourceReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, testBindingName)

		result, err := svc.CreateResourceReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, resourceReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testBindingName, result.Name)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, testProject, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testResourceName, result.Labels[labels.LabelKeyResourceName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateResourceReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("resource not found", func(t *testing.T) {
		svc := NewService(testutil.NewFakeClient(), testutil.TestLogger())
		rb := testutil.NewResourceReleaseBinding(testNamespace, testProject, "missing-resource", testEnvironment, testBindingName)

		_, err := svc.CreateResourceReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, "dup")
		svc := newService(t, existing)
		dup := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, "dup")

		_, err := svc.CreateResourceReleaseBinding(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrResourceReleaseBindingAlreadyExists)
	})

	t.Run("namespace overrides body namespace", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewResourceReleaseBinding("ignored", testProject, testResourceName, testEnvironment, testBindingName)

		result, err := svc.CreateResourceReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})
}

func TestUpdateResourceReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, testBindingName)
		svc := newService(t, existing)

		update := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, testBindingName)
		update.Spec.ResourceRelease = "test-r-abc123"

		result, err := svc.UpdateResourceReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, resourceReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-r-abc123", result.Spec.ResourceRelease)
		assert.Equal(t, testProject, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testResourceName, result.Labels[labels.LabelKeyResourceName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateResourceReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, "nonexistent")

		_, err := svc.UpdateResourceReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrResourceReleaseBindingNotFound)
	})
}

func TestListResourceReleaseBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("all without resource filter", func(t *testing.T) {
		rb1 := testutil.NewResourceReleaseBinding(testNamespace, testProject, "r-a", "dev", "rb-1")
		rb2 := testutil.NewResourceReleaseBinding(testNamespace, testProject, "r-b", "dev", "rb-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListResourceReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, resourceReleaseBindingTypeMeta, item.TypeMeta)
		}
	})

	t.Run("with resource filter", func(t *testing.T) {
		rb1 := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, "dev", "rb-1")
		rb2 := testutil.NewResourceReleaseBinding(testNamespace, testProject, "other-r", "dev", "rb-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListResourceReleaseBindings(ctx, testNamespace, testResourceName, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, testResourceName, result.Items[0].Spec.Owner.ResourceName)
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListResourceReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListResourceReleaseBindings(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetResourceReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		rb := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, testBindingName)
		svc := newService(t, rb)

		result, err := svc.GetResourceReleaseBinding(ctx, testNamespace, testBindingName)
		require.NoError(t, err)
		assert.Equal(t, resourceReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testBindingName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetResourceReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrResourceReleaseBindingNotFound)
	})
}

func TestDeleteResourceReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rb := testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, testBindingName)
		svc := newService(t, rb)

		err := svc.DeleteResourceReleaseBinding(ctx, testNamespace, testBindingName)
		require.NoError(t, err)

		_, err = svc.GetResourceReleaseBinding(ctx, testNamespace, testBindingName)
		require.ErrorIs(t, err, ErrResourceReleaseBindingNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteResourceReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrResourceReleaseBindingNotFound)
	})
}

const newReleaseName = "new-release"

// TestUpdateResourceReleaseBindingResourceVersion covers the optimistic concurrency of the update:
// a resourceVersion read earlier makes it conditional (#5370).
func TestUpdateResourceReleaseBindingResourceVersion(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T) (client.Client, Service, *openchoreov1alpha1.ResourceReleaseBinding) {
		t.Helper()
		fc := testutil.NewFakeClient(testutil.NewResourceReleaseBinding(testNamespace, testProject, testResourceName, testEnvironment, testBindingName))
		svc := NewService(fc, testutil.TestLogger())
		read, err := svc.GetResourceReleaseBinding(ctx, testNamespace, testBindingName)
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
		stale.Spec.ResourceRelease = newReleaseName
		_, err := svc.UpdateResourceReleaseBinding(ctx, testNamespace, stale)
		require.ErrorIs(t, err, services.ErrConflict)

		stored := &openchoreov1alpha1.ResourceReleaseBinding{}
		require.NoError(t, fc.Get(ctx, client.ObjectKeyFromObject(read), stored))
		assert.Equal(t, "x", stored.Annotations["example/marker"])
		assert.Equal(t, read.Spec.ResourceRelease, stored.Spec.ResourceRelease)
	})

	t.Run("current resourceVersion succeeds", func(t *testing.T) {
		_, svc, read := setup(t)

		update := read.DeepCopy()
		update.Spec.ResourceRelease = newReleaseName
		result, err := svc.UpdateResourceReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, newReleaseName, result.Spec.ResourceRelease)
		assert.NotEqual(t, read.ResourceVersion, result.ResourceVersion)
	})

	t.Run("no resourceVersion overwrites unconditionally", func(t *testing.T) {
		fc, svc, read := setup(t)

		other := read.DeepCopy()
		other.Annotations = map[string]string{"example/marker": "x"}
		require.NoError(t, fc.Update(ctx, other))

		update := read.DeepCopy()
		update.ResourceVersion = ""
		update.Spec.ResourceRelease = newReleaseName
		result, err := svc.UpdateResourceReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, newReleaseName, result.Spec.ResourceRelease)
	})

	for _, rv := range []string{"abc", "0", "-1", "1.5", " 12"} {
		t.Run("malformed resourceVersion "+rv+" is a validation error", func(t *testing.T) {
			fc, svc, read := setup(t)

			update := read.DeepCopy()
			update.ResourceVersion = rv
			update.Spec.ResourceRelease = newReleaseName
			_, err := svc.UpdateResourceReleaseBinding(ctx, testNamespace, update)
			validationErr, ok := errors.AsType[*services.ValidationError](err)
			require.True(t, ok, "expected a ValidationError, got %v", err)
			assert.Contains(t, validationErr.Msg, "metadata.resourceVersion")

			stored := &openchoreov1alpha1.ResourceReleaseBinding{}
			require.NoError(t, fc.Get(ctx, client.ObjectKeyFromObject(read), stored))
			assert.Equal(t, read.Spec.ResourceRelease, stored.Spec.ResourceRelease)
		})
	}
}
