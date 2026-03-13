// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

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

const testNamespace = "test-ns"

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateDeploymentPipeline(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		dp := testutil.NewDeploymentPipeline(testNamespace, "default")

		result, err := svc.CreateDeploymentPipeline(ctx, testNamespace, dp)
		require.NoError(t, err)
		assert.Equal(t, deploymentPipelineTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.DeploymentPipelineStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateDeploymentPipeline(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewDeploymentPipeline(testNamespace, "default")
		svc := newService(t, existing)
		dp := &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec:       existing.Spec,
		}

		_, err := svc.CreateDeploymentPipeline(ctx, testNamespace, dp)
		require.ErrorIs(t, err, ErrDeploymentPipelineAlreadyExists)
	})

	t.Run("same name in other namespace succeeds", func(t *testing.T) {
		existing := testutil.NewDeploymentPipeline("other-ns", "default")
		svc := newService(t, existing)
		dp := testutil.NewDeploymentPipeline(testNamespace, "default")

		result, err := svc.CreateDeploymentPipeline(ctx, testNamespace, dp)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})
}

func TestUpdateDeploymentPipeline(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewDeploymentPipeline(testNamespace, "default")
		svc := newService(t, existing)

		update := &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "default",
				Labels: map[string]string{"env": "prod"},
			},
			Spec: existing.Spec,
		}

		result, err := svc.UpdateDeploymentPipeline(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, deploymentPipelineTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateDeploymentPipeline(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		dp := &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateDeploymentPipeline(ctx, testNamespace, dp)
		require.ErrorIs(t, err, ErrDeploymentPipelineNotFound)
	})
}

func TestListDeploymentPipelines(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		dp1 := testutil.NewDeploymentPipeline(testNamespace, "dp-1")
		dp2 := testutil.NewDeploymentPipeline(testNamespace, "dp-2")
		svc := newService(t, dp1, dp2)

		result, err := svc.ListDeploymentPipelines(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, deploymentPipelineTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListDeploymentPipelines(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListDeploymentPipelines(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		dpInNs := testutil.NewDeploymentPipeline(testNamespace, "dp-in")
		dpOtherNs := testutil.NewDeploymentPipeline("other-ns", "dp-out")
		svc := newService(t, dpInNs, dpOtherNs)

		result, err := svc.ListDeploymentPipelines(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "dp-in", result.Items[0].Name)
	})
}

func TestGetDeploymentPipeline(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		dp := testutil.NewDeploymentPipeline(testNamespace, "default")
		svc := newService(t, dp)

		result, err := svc.GetDeploymentPipeline(ctx, testNamespace, "default")
		require.NoError(t, err)
		assert.Equal(t, deploymentPipelineTypeMeta, result.TypeMeta)
		assert.Equal(t, "default", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetDeploymentPipeline(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrDeploymentPipelineNotFound)
	})
}

func TestDeleteDeploymentPipeline(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		dp := testutil.NewDeploymentPipeline(testNamespace, "default")
		svc := newService(t, dp)

		err := svc.DeleteDeploymentPipeline(ctx, testNamespace, "default")
		require.NoError(t, err)

		_, err = svc.GetDeploymentPipeline(ctx, testNamespace, "default")
		require.ErrorIs(t, err, ErrDeploymentPipelineNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteDeploymentPipeline(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrDeploymentPipelineNotFound)
	})
}
