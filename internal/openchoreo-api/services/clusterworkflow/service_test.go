// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateClusterWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		cwf := testutil.NewClusterWorkflow("test-cwf")

		result, err := svc.CreateClusterWorkflow(ctx, cwf)
		require.NoError(t, err)
		assert.Equal(t, clusterWorkflowTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cwf", result.Name)
		assert.Equal(t, openchoreov1alpha1.ClusterWorkflowStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateClusterWorkflow(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewClusterWorkflow("dup-cwf")
		svc := newService(t, existing)
		dup := testutil.NewClusterWorkflow("dup-cwf")

		_, err := svc.CreateClusterWorkflow(ctx, dup)
		require.ErrorIs(t, err, ErrClusterWorkflowAlreadyExists)
	})
}

func TestUpdateClusterWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewClusterWorkflow("test-cwf")
		svc := newService(t, existing)

		update := testutil.NewClusterWorkflow("test-cwf")
		update.Labels = map[string]string{"env": "prod"}
		update.Annotations = map[string]string{"note": "updated"}

		result, err := svc.UpdateClusterWorkflow(ctx, update)
		require.NoError(t, err)
		assert.Equal(t, clusterWorkflowTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, "updated", result.Annotations["note"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateClusterWorkflow(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		cwf := testutil.NewClusterWorkflow("nonexistent")

		_, err := svc.UpdateClusterWorkflow(ctx, cwf)
		require.ErrorIs(t, err, ErrClusterWorkflowNotFound)
	})
}

func TestListClusterWorkflows(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		cwf1 := testutil.NewClusterWorkflow("cwf-1")
		cwf2 := testutil.NewClusterWorkflow("cwf-2")
		svc := newService(t, cwf1, cwf2)

		result, err := svc.ListClusterWorkflows(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, clusterWorkflowTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListClusterWorkflows(ctx, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListClusterWorkflows(ctx, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetClusterWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		cwf := testutil.NewClusterWorkflow("test-cwf")
		svc := newService(t, cwf)

		result, err := svc.GetClusterWorkflow(ctx, "test-cwf")
		require.NoError(t, err)
		assert.Equal(t, clusterWorkflowTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-cwf", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterWorkflow(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterWorkflowNotFound)
	})
}

func TestDeleteClusterWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cwf := testutil.NewClusterWorkflow("test-cwf")
		svc := newService(t, cwf)

		err := svc.DeleteClusterWorkflow(ctx, "test-cwf")
		require.NoError(t, err)

		_, err = svc.GetClusterWorkflow(ctx, "test-cwf")
		require.ErrorIs(t, err, ErrClusterWorkflowNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteClusterWorkflow(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterWorkflowNotFound)
	})
}

func TestGetClusterWorkflowSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		cwf := testutil.NewClusterWorkflow("no-params")
		cwf.Spec.Parameters = nil
		svc := newService(t, cwf)

		result, err := svc.GetClusterWorkflowSchema(ctx, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OpenAPIV3 schema", func(t *testing.T) {
		cwf := testutil.NewClusterWorkflow("with-schema")
		cwf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer"}}}`)},
		}
		svc := newService(t, cwf)

		result, err := svc.GetClusterWorkflowSchema(ctx, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "replicas")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetClusterWorkflowSchema(ctx, "nonexistent")
		require.ErrorIs(t, err, ErrClusterWorkflowNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		cwf := testutil.NewClusterWorkflow("bad-schema")
		cwf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, cwf)

		_, err := svc.GetClusterWorkflowSchema(ctx, "bad-schema")
		require.Error(t, err)
	})
}
