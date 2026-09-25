// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

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
	testHookName  = "image-scan"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func newHook(namespace, name string) *openchoreov1alpha1.Hook {
	return &openchoreov1alpha1.Hook{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: openchoreov1alpha1.HookSpec{
			Type:        openchoreov1alpha1.HookTypeWorkflow,
			WorkflowRef: &openchoreov1alpha1.WorkflowRef{Kind: openchoreov1alpha1.WorkflowRefKindWorkflow, Name: "trivy"},
			Parameters: []openchoreov1alpha1.HookParameter{
				{Name: "image", From: "${deployment.workload.containers.main.image}"},
			},
		},
	}
}

func TestCreateHook(t *testing.T) {
	ctx := context.Background()

	t.Run("success sets namespace, type meta and clears status", func(t *testing.T) {
		svc := newService(t)
		h := newHook("", testHookName)
		h.Status.ObservedGeneration = 7 // must not survive a create

		result, err := svc.CreateHook(ctx, testNamespace, h)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, hookTypeMeta, result.TypeMeta)
		assert.Equal(t, openchoreov1alpha1.HookStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.CreateHook(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		svc := newService(t, newHook(testNamespace, testHookName))
		_, err := svc.CreateHook(ctx, testNamespace, newHook(testNamespace, testHookName))
		require.ErrorIs(t, err, ErrHookAlreadyExists)
	})
}

func TestUpdateHook(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces spec and labels but keeps server-managed fields", func(t *testing.T) {
		existing := newHook(testNamespace, testHookName)
		existing.Status.ObservedGeneration = 3
		svc := newService(t, existing)

		update := newHook("", testHookName)
		update.Labels = map[string]string{"team": "platform"}
		update.Spec.Parameters = append(update.Spec.Parameters, openchoreov1alpha1.HookParameter{Name: "ticket", Required: true})

		result, err := svc.UpdateHook(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, hookTypeMeta, result.TypeMeta)
		assert.Equal(t, "platform", result.Labels["team"])
		assert.Len(t, result.Spec.Parameters, 2)
		assert.Equal(t, int64(3), result.Status.ObservedGeneration, "update must not touch status")
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.UpdateHook(ctx, testNamespace, nil)
		require.Error(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.UpdateHook(ctx, testNamespace, newHook("", "missing"))
		require.ErrorIs(t, err, ErrHookNotFound)
	})
}

func TestListHooks(t *testing.T) {
	ctx := context.Background()

	t.Run("lists only the namespace's hooks", func(t *testing.T) {
		svc := newService(t, newHook(testNamespace, "a"), newHook(testNamespace, "b"), newHook("other", "c"))
		result, err := svc.ListHooks(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, hookTypeMeta, item.TypeMeta)
		}
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.ListHooks(ctx, testNamespace, services.ListOptions{LabelSelector: "===bad"})
		require.Error(t, err)
	})
}

func TestGetHook(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t, newHook(testNamespace, testHookName))
		result, err := svc.GetHook(ctx, testNamespace, testHookName)
		require.NoError(t, err)
		assert.Equal(t, testHookName, result.Name)
		assert.Equal(t, hookTypeMeta, result.TypeMeta)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetHook(ctx, testNamespace, "missing")
		require.ErrorIs(t, err, ErrHookNotFound)
	})
}

func TestDeleteHook(t *testing.T) {
	ctx := context.Background()

	t.Run("success removes the object", func(t *testing.T) {
		svc := newService(t, newHook(testNamespace, testHookName))
		require.NoError(t, svc.DeleteHook(ctx, testNamespace, testHookName))
		_, err := svc.GetHook(ctx, testNamespace, testHookName)
		require.ErrorIs(t, err, ErrHookNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		require.ErrorIs(t, svc.DeleteHook(ctx, testNamespace, "missing"), ErrHookNotFound)
	})
}
