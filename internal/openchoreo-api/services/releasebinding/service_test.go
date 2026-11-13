// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace       = "test-ns"
	testProjectName     = "test-project"
	testComponentName   = "test-component"
	testEnvironmentName = "dev"
	testRBName          = "test-rb"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, comp)
		rb := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)

		result, err := svc.CreateReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ReleaseBindingStatus{}, result.Status)
		assert.Equal(t, releaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})

	t.Run("nil input", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, comp)

		_, err := svc.CreateReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		existing := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		svc := newService(t, comp, existing)

		rb := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		_, err := svc.CreateReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrReleaseBindingAlreadyExists)
	})

	t.Run("component not found", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)

		_, err := svc.CreateReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrComponentNotFound)
	})

	t.Run("label normalization", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, comp)

		rb := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		rb.Labels = map[string]string{
			labels.LabelKeyProjectName:   "wrong",
			labels.LabelKeyComponentName: "wrong",
		}

		result, err := svc.CreateReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})
}

func TestUpdateReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testRBName,
				Labels: map[string]string{"custom": "value"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, releaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, openchoreov1alpha1.ReleaseBindingStatus{}, result.Status)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
		assert.Equal(t, "value", result.Labels["custom"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		rb := &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})
}

func TestListReleaseBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		rb1 := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, "rb-1")
		rb2 := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, "staging", "rb-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, releaseBindingTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("with component filter", func(t *testing.T) {
		rb1 := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, "rb-1")
		rb2 := testutil.NewReleaseBinding(testNamespace, testProjectName, "other-component", testEnvironmentName, "rb-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListReleaseBindings(ctx, testNamespace, testComponentName, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, testComponentName, result.Items[0].Spec.Owner.ComponentName)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		rbInNs := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, "rb-in")
		rbOtherNs := testutil.NewReleaseBinding("other-ns", testProjectName, testComponentName, testEnvironmentName, "rb-out")
		svc := newService(t, rbInNs, rbOtherNs)

		result, err := svc.ListReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "rb-in", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListReleaseBindings(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		rb := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		svc := newService(t, rb)

		result, err := svc.GetReleaseBinding(ctx, testNamespace, testRBName)
		require.NoError(t, err)
		assert.Equal(t, releaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testRBName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})
}

func TestDeleteReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rb := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		svc := newService(t, rb)

		err := svc.DeleteReleaseBinding(ctx, testNamespace, testRBName)
		require.NoError(t, err)

		_, err = svc.GetReleaseBinding(ctx, testNamespace, testRBName)
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})
}

const newReleaseName = "new-release"

// TestUpdateReleaseBindingResourceVersion covers the optimistic concurrency of the update:
// a resourceVersion read earlier makes it conditional (#5370).
func TestUpdateReleaseBindingResourceVersion(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T) (client.Client, Service, *openchoreov1alpha1.ReleaseBinding) {
		t.Helper()
		// dev is the first environment of the pipeline, so any release may be set on it.
		fc := testutil.NewFakeClient(
			testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName),
			testutil.NewProject(testNamespace, testProjectName),
			testutil.NewDeploymentPipeline(testNamespace, "default"),
		)
		svc := NewService(fc, testutil.TestLogger())
		read, err := svc.GetReleaseBinding(ctx, testNamespace, testRBName)
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
		stale.Spec.ReleaseName = newReleaseName
		_, err := svc.UpdateReleaseBinding(ctx, testNamespace, stale)
		require.ErrorIs(t, err, services.ErrConflict)

		stored := &openchoreov1alpha1.ReleaseBinding{}
		require.NoError(t, fc.Get(ctx, client.ObjectKeyFromObject(read), stored))
		assert.Equal(t, "x", stored.Annotations["example/marker"])
		assert.Equal(t, read.Spec.ReleaseName, stored.Spec.ReleaseName)
	})

	t.Run("current resourceVersion succeeds", func(t *testing.T) {
		_, svc, read := setup(t)

		update := read.DeepCopy()
		update.Spec.ReleaseName = newReleaseName
		result, err := svc.UpdateReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, newReleaseName, result.Spec.ReleaseName)
		assert.NotEqual(t, read.ResourceVersion, result.ResourceVersion)
	})

	t.Run("no resourceVersion overwrites unconditionally", func(t *testing.T) {
		fc, svc, read := setup(t)

		other := read.DeepCopy()
		other.Annotations = map[string]string{"example/marker": "x"}
		require.NoError(t, fc.Update(ctx, other))

		update := read.DeepCopy()
		update.ResourceVersion = ""
		update.Spec.ReleaseName = newReleaseName
		result, err := svc.UpdateReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, newReleaseName, result.Spec.ReleaseName)
	})

	for _, rv := range []string{"abc", "0", "-1", "1.5", " 12"} {
		t.Run("malformed resourceVersion "+rv+" is a validation error", func(t *testing.T) {
			fc, svc, read := setup(t)

			update := read.DeepCopy()
			update.ResourceVersion = rv
			update.Spec.ReleaseName = newReleaseName
			_, err := svc.UpdateReleaseBinding(ctx, testNamespace, update)
			validationErr, ok := errors.AsType[*services.ValidationError](err)
			require.True(t, ok, "expected a ValidationError, got %v", err)
			assert.Contains(t, validationErr.Msg, "metadata.resourceVersion")

			stored := &openchoreov1alpha1.ReleaseBinding{}
			require.NoError(t, fc.Get(ctx, client.ObjectKeyFromObject(read), stored))
			assert.Equal(t, read.Spec.ReleaseName, stored.Spec.ReleaseName)
		})
	}
}

// unpromotedRelease is a release no binding in the source environment references.
const unpromotedRelease = "old-release"

// TestUpdateReleaseBindingStaleBeforePromotionCheck: a stale request is judged
// against a state its caller never saw, so it must get the conflict (which the
// clients retry on) rather than a promotion error about that state.
func TestUpdateReleaseBindingStaleBeforePromotionCheck(t *testing.T) {
	ctx := context.Background()
	// The test pipeline promotes dev -> prod; nothing in dev references unpromotedRelease.
	fc := testutil.NewFakeClient(
		testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, "prod", "rb-prod"),
		testutil.NewProject(testNamespace, testProjectName),
		testutil.NewDeploymentPipeline(testNamespace, "default"),
	)
	svc := NewService(fc, testutil.TestLogger())
	read, err := svc.GetReleaseBinding(ctx, testNamespace, "rb-prod")
	require.NoError(t, err)

	other := read.DeepCopy()
	other.Annotations = map[string]string{"example/marker": "x"}
	require.NoError(t, fc.Update(ctx, other))

	stale := read.DeepCopy()
	stale.Spec.ReleaseName = unpromotedRelease
	_, err = svc.UpdateReleaseBinding(ctx, testNamespace, stale)
	require.ErrorIs(t, err, services.ErrConflict)

	t.Run("a malformed version is still a validation error", func(t *testing.T) {
		bad := read.DeepCopy()
		bad.ResourceVersion = "abc"
		bad.Spec.ReleaseName = unpromotedRelease
		_, err := svc.UpdateReleaseBinding(ctx, testNamespace, bad)
		validationErr, ok := errors.AsType[*services.ValidationError](err)
		require.True(t, ok, "expected a ValidationError, got %v", err)
		assert.Contains(t, validationErr.Msg, "metadata.resourceVersion")
	})

	t.Run("a current version still runs the promotion check", func(t *testing.T) {
		current, err := svc.GetReleaseBinding(ctx, testNamespace, "rb-prod")
		require.NoError(t, err)
		current.Spec.ReleaseName = unpromotedRelease
		_, err = svc.UpdateReleaseBinding(ctx, testNamespace, current)
		validationErr, ok := errors.AsType[*services.ValidationError](err)
		require.True(t, ok, "expected a ValidationError, got %v", err)
		assert.Contains(t, validationErr.Msg, "must be referenced by a ReleaseBinding")
	})
}
