// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace = "test-ns"
	testSRName    = "test-secret-ref"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateSecretReference(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		sr := &openchoreov1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{Name: testSRName},
			Spec:       testutil.NewSecretReference(testNamespace, testSRName).Spec,
		}

		result, err := svc.CreateSecretReference(ctx, testNamespace, sr)
		require.NoError(t, err)
		assert.Equal(t, secretReferenceTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.SecretReferenceStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.CreateSecretReference(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewSecretReference(testNamespace, testSRName)
		svc := newService(t, existing)
		sr := &openchoreov1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{Name: testSRName},
			Spec:       existing.Spec,
		}

		_, err := svc.CreateSecretReference(ctx, testNamespace, sr)
		require.ErrorIs(t, err, ErrSecretReferenceAlreadyExists)
	})
}

func TestUpdateSecretReference(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewSecretReference(testNamespace, testSRName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testSRName,
				Labels: map[string]string{"env": "prod"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateSecretReference(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, secretReferenceTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, existing.Spec, result.Spec)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.UpdateSecretReference(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		sr := &openchoreov1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateSecretReference(ctx, testNamespace, sr)
		require.ErrorIs(t, err, ErrSecretReferenceNotFound)
	})
}

func TestListSecretReferences(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		sr1 := testutil.NewSecretReference(testNamespace, "sr-1")
		sr2 := testutil.NewSecretReference(testNamespace, "sr-2")
		svc := newService(t, sr1, sr2)

		result, err := svc.ListSecretReferences(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, secretReferenceTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListSecretReferences(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListSecretReferences(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		srInNs := testutil.NewSecretReference(testNamespace, "sr-in")
		srOtherNs := testutil.NewSecretReference("other-ns", "sr-out")
		svc := newService(t, srInNs, srOtherNs)

		result, err := svc.ListSecretReferences(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "sr-in", result.Items[0].Name)
	})
}

func TestGetSecretReference(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		sr := testutil.NewSecretReference(testNamespace, testSRName)
		svc := newService(t, sr)

		result, err := svc.GetSecretReference(ctx, testNamespace, testSRName)
		require.NoError(t, err)
		assert.Equal(t, secretReferenceTypeMeta, result.TypeMeta)
		assert.Equal(t, testSRName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetSecretReference(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrSecretReferenceNotFound)
	})
}

func TestDeleteSecretReference(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		sr := testutil.NewSecretReference(testNamespace, testSRName)
		svc := newService(t, sr)

		err := svc.DeleteSecretReference(ctx, testNamespace, testSRName)
		require.NoError(t, err)

		_, err = svc.GetSecretReference(ctx, testNamespace, testSRName)
		require.ErrorIs(t, err, ErrSecretReferenceNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteSecretReference(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrSecretReferenceNotFound)
	})
}
