// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentreleasebinding

import (
	"context"
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

func TestCreateComponentReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, comp)
		rb := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)

		result, err := svc.CreateComponentReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ComponentReleaseBindingStatus{}, result.Status)
		assert.Equal(t, componentReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})

	t.Run("nil input", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, comp)

		_, err := svc.CreateComponentReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		existing := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		svc := newService(t, comp, existing)

		rb := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		_, err := svc.CreateComponentReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrComponentReleaseBindingAlreadyExists)
	})

	t.Run("component not found", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)

		_, err := svc.CreateComponentReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrComponentNotFound)
	})

	t.Run("label normalization", func(t *testing.T) {
		comp := testutil.NewComponent(testNamespace, testProjectName, testComponentName)
		svc := newService(t, comp)

		rb := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		rb.Labels = map[string]string{
			labels.LabelKeyProjectName:   "wrong",
			labels.LabelKeyComponentName: "wrong",
		}

		result, err := svc.CreateComponentReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})
}

func TestUpdateComponentReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.ComponentReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testRBName,
				Labels: map[string]string{"custom": "value"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateComponentReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, componentReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, openchoreov1alpha1.ComponentReleaseBindingStatus{}, result.Status)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
		assert.Equal(t, "value", result.Labels["custom"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateComponentReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		rb := &openchoreov1alpha1.ComponentReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateComponentReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrComponentReleaseBindingNotFound)
	})
}

func TestListComponentComponentReleaseBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		rb1 := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, "rb-1")
		rb2 := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, "staging", "rb-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListComponentComponentReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, componentReleaseBindingTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListComponentComponentReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("with component filter", func(t *testing.T) {
		rb1 := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, "rb-1")
		rb2 := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, "other-component", testEnvironmentName, "rb-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListComponentComponentReleaseBindings(ctx, testNamespace, testComponentName, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, testComponentName, result.Items[0].Spec.Owner.ComponentName)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		rbInNs := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, "rb-in")
		rbOtherNs := testutil.NewComponentReleaseBinding("other-ns", testProjectName, testComponentName, testEnvironmentName, "rb-out")
		svc := newService(t, rbInNs, rbOtherNs)

		result, err := svc.ListComponentComponentReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "rb-in", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListComponentComponentReleaseBindings(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetComponentReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		rb := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		svc := newService(t, rb)

		result, err := svc.GetComponentReleaseBinding(ctx, testNamespace, testRBName)
		require.NoError(t, err)
		assert.Equal(t, componentReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, testRBName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetComponentReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentReleaseBindingNotFound)
	})
}

func TestDeleteComponentReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rb := testutil.NewComponentReleaseBinding(testNamespace, testProjectName, testComponentName, testEnvironmentName, testRBName)
		svc := newService(t, rb)

		err := svc.DeleteComponentReleaseBinding(ctx, testNamespace, testRBName)
		require.NoError(t, err)

		_, err = svc.GetComponentReleaseBinding(ctx, testNamespace, testRBName)
		require.ErrorIs(t, err, ErrComponentReleaseBindingNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteComponentReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentReleaseBindingNotFound)
	})
}
