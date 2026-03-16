// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const testNamespace = "test-ns"

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		wf := testutil.NewWorkflow(testNamespace, "test-wf")

		result, err := svc.CreateWorkflow(ctx, testNamespace, wf)
		require.NoError(t, err)
		assert.Equal(t, workflowTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, "test-wf", result.Name)
		assert.Equal(t, openchoreov1alpha1.WorkflowStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateWorkflow(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrWorkflowNil)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewWorkflow(testNamespace, "dup-wf")
		svc := newService(t, existing)
		dup := &openchoreov1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-wf"},
		}

		_, err := svc.CreateWorkflow(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrWorkflowAlreadyExists)
	})

	t.Run("same name in other namespace succeeds", func(t *testing.T) {
		existing := testutil.NewWorkflow("other-ns", "test-wf")
		svc := newService(t, existing)
		wf := testutil.NewWorkflow(testNamespace, "test-wf")

		result, err := svc.CreateWorkflow(ctx, testNamespace, wf)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})
}

func TestUpdateWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewWorkflow(testNamespace, "test-wf")
		svc := newService(t, existing)

		update := testutil.NewWorkflow(testNamespace, "test-wf")
		update.Labels = map[string]string{"env": "prod"}
		update.Annotations = map[string]string{"note": "updated"}

		result, err := svc.UpdateWorkflow(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, workflowTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, "updated", result.Annotations["note"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateWorkflow(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrWorkflowNil)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		wf := &openchoreov1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateWorkflow(ctx, testNamespace, wf)
		require.ErrorIs(t, err, ErrWorkflowNotFound)
	})
}

func TestListWorkflows(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		wf1 := testutil.NewWorkflow(testNamespace, "wf-1")
		wf2 := testutil.NewWorkflow(testNamespace, "wf-2")
		svc := newService(t, wf1, wf2)

		result, err := svc.ListWorkflows(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, workflowTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListWorkflows(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		wfInNs := testutil.NewWorkflow(testNamespace, "wf-in")
		wfOtherNs := testutil.NewWorkflow("other-ns", "wf-out")
		svc := newService(t, wfInNs, wfOtherNs)

		result, err := svc.ListWorkflows(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "wf-in", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListWorkflows(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, "test-wf")
		svc := newService(t, wf)

		result, err := svc.GetWorkflow(ctx, testNamespace, "test-wf")
		require.NoError(t, err)
		assert.Equal(t, workflowTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-wf", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetWorkflow(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkflowNotFound)
	})
}

func TestDeleteWorkflow(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, "test-wf")
		svc := newService(t, wf)

		err := svc.DeleteWorkflow(ctx, testNamespace, "test-wf")
		require.NoError(t, err)

		_, err = svc.GetWorkflow(ctx, testNamespace, "test-wf")
		require.ErrorIs(t, err, ErrWorkflowNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteWorkflow(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkflowNotFound)
	})
}

func TestGetWorkflowSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, "no-params")
		wf.Spec.Parameters = nil
		svc := newService(t, wf)

		result, err := svc.GetWorkflowSchema(ctx, testNamespace, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OpenAPIV3 schema", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, "with-schema")
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer"}}}`)},
		}
		svc := newService(t, wf)

		result, err := svc.GetWorkflowSchema(ctx, testNamespace, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "replicas")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetWorkflowSchema(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrWorkflowNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		wf := testutil.NewWorkflow(testNamespace, "bad-schema")
		wf.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, wf)

		_, err := svc.GetWorkflowSchema(ctx, testNamespace, "bad-schema")
		require.Error(t, err)
	})
}
