// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

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
	testNamespace   = "test-ns"
	testProjectName = "test-project"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateProject(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		proj := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: testProjectName},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
					Kind: openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline,
					Name: "my-pipeline",
				},
			},
		}

		result, err := svc.CreateProject(ctx, testNamespace, proj)
		require.NoError(t, err)
		assert.Equal(t, projectTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ProjectStatus{}, result.Status)
		assert.Equal(t, "my-pipeline", result.Spec.DeploymentPipelineRef.Name)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateProject(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewProject(testNamespace, testProjectName)
		svc := newService(t, existing)
		proj := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: testProjectName},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
			},
		}

		_, err := svc.CreateProject(ctx, testNamespace, proj)
		require.ErrorIs(t, err, ErrProjectAlreadyExists)
	})

	t.Run("default pipeline ref when empty", func(t *testing.T) {
		svc := newService(t)
		proj := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: "proj-no-pipeline"},
			Spec:       openchoreov1alpha1.ProjectSpec{},
		}

		result, err := svc.CreateProject(ctx, testNamespace, proj)
		require.NoError(t, err)
		assert.Equal(t, defaultPipeline, result.Spec.DeploymentPipelineRef.Name)
		assert.Equal(t, openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline, result.Spec.DeploymentPipelineRef.Kind)
	})

	t.Run("explicit pipeline ref preserved", func(t *testing.T) {
		svc := newService(t)
		proj := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: "proj-explicit"},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
					Kind: openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline,
					Name: "custom-pipeline",
				},
			},
		}

		result, err := svc.CreateProject(ctx, testNamespace, proj)
		require.NoError(t, err)
		assert.Equal(t, "custom-pipeline", result.Spec.DeploymentPipelineRef.Name)
	})
}

func TestUpdateProject(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewProject(testNamespace, testProjectName)
		svc := newService(t, existing)

		update := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:        testProjectName,
				Labels:      map[string]string{"env": "prod"},
				Annotations: map[string]string{"note": "updated"},
			},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "new-pipeline"},
			},
		}

		result, err := svc.UpdateProject(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, projectTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, "updated", result.Annotations["note"])
		assert.Equal(t, "new-pipeline", result.Spec.DeploymentPipelineRef.Name)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateProject(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		proj := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateProject(ctx, testNamespace, proj)
		require.ErrorIs(t, err, ErrProjectNotFound)
	})
}

func TestListProjects(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		p1 := testutil.NewProject(testNamespace, "proj-1")
		p2 := testutil.NewProject(testNamespace, "proj-2")
		svc := newService(t, p1, p2)

		result, err := svc.ListProjects(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, projectTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListProjects(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListProjects(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		projInNs := testutil.NewProject(testNamespace, "proj-in")
		projOtherNs := testutil.NewProject("other-ns", "proj-out")
		svc := newService(t, projInNs, projOtherNs)

		result, err := svc.ListProjects(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "proj-in", result.Items[0].Name)
	})
}

func TestGetProject(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		proj := testutil.NewProject(testNamespace, testProjectName)
		svc := newService(t, proj)

		result, err := svc.GetProject(ctx, testNamespace, testProjectName)
		require.NoError(t, err)
		assert.Equal(t, projectTypeMeta, result.TypeMeta)
		assert.Equal(t, testProjectName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetProject(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectNotFound)
	})
}

func TestDeleteProject(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		proj := testutil.NewProject(testNamespace, testProjectName)
		svc := newService(t, proj)

		err := svc.DeleteProject(ctx, testNamespace, testProjectName)
		require.NoError(t, err)

		_, err = svc.GetProject(ctx, testNamespace, testProjectName)
		require.ErrorIs(t, err, ErrProjectNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteProject(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectNotFound)
	})
}
