// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

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

const testHookName = "trivy-image-scan"

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func newClusterHook(name string) *openchoreov1alpha1.ClusterHook {
	return &openchoreov1alpha1.ClusterHook{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.HookSpec{
			Type:        openchoreov1alpha1.HookTypeWorkflow,
			WorkflowRef: &openchoreov1alpha1.WorkflowRef{Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow, Name: "trivy"},
		},
	}
}

func TestCreateClusterHook(t *testing.T) {
	ctx := context.Background()

	t.Run("success sets type meta and clears status", func(t *testing.T) {
		svc := newService(t)
		ch := newClusterHook(testHookName)
		ch.Status.ObservedGeneration = 2

		result, err := svc.CreateClusterHook(ctx, ch)
		require.NoError(t, err)
		assert.Equal(t, clusterHookTypeMeta, result.TypeMeta)
		assert.Equal(t, openchoreov1alpha1.HookStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		_, err := newService(t).CreateClusterHook(ctx, nil)
		require.Error(t, err)
	})

	t.Run("already exists", func(t *testing.T) {
		svc := newService(t, newClusterHook(testHookName))
		_, err := svc.CreateClusterHook(ctx, newClusterHook(testHookName))
		require.ErrorIs(t, err, ErrClusterHookAlreadyExists)
	})
}

func TestUpdateClusterHook(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces spec, keeps status", func(t *testing.T) {
		existing := newClusterHook(testHookName)
		existing.Status.ObservedGeneration = 5
		svc := newService(t, existing)

		update := newClusterHook(testHookName)
		update.Spec.Parameters = []openchoreov1alpha1.HookParameter{{Name: "severity", Default: strPtr("HIGH")}}

		result, err := svc.UpdateClusterHook(ctx, update)
		require.NoError(t, err)
		assert.Len(t, result.Spec.Parameters, 1)
		assert.Equal(t, int64(5), result.Status.ObservedGeneration)
		assert.Equal(t, clusterHookTypeMeta, result.TypeMeta)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := newService(t).UpdateClusterHook(ctx, newClusterHook("missing"))
		require.ErrorIs(t, err, ErrClusterHookNotFound)
	})
}

func TestListGetDeleteClusterHook(t *testing.T) {
	ctx := context.Background()

	t.Run("list", func(t *testing.T) {
		svc := newService(t, newClusterHook("a"), newClusterHook("b"))
		result, err := svc.ListClusterHooks(ctx, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		assert.Equal(t, clusterHookTypeMeta, result.Items[0].TypeMeta)
	})

	t.Run("get", func(t *testing.T) {
		svc := newService(t, newClusterHook(testHookName))
		result, err := svc.GetClusterHook(ctx, testHookName)
		require.NoError(t, err)
		assert.Equal(t, testHookName, result.Name)

		_, err = svc.GetClusterHook(ctx, "missing")
		require.ErrorIs(t, err, ErrClusterHookNotFound)
	})

	t.Run("delete", func(t *testing.T) {
		svc := newService(t, newClusterHook(testHookName))
		require.NoError(t, svc.DeleteClusterHook(ctx, testHookName))
		require.ErrorIs(t, svc.DeleteClusterHook(ctx, testHookName), ErrClusterHookNotFound)
	})
}

func strPtr(s string) *string { return &s }
