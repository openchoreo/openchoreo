// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace   = "test-ns"
	testProjectName = "my-app"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateProjectReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		project := testutil.NewProject(testNamespace, testProjectName)
		svc := newService(t, project)
		rb := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "my-app-dev")

		result, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, rb)
		require.NoError(t, err)
		assert.Equal(t, projectReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, "my-app-dev", result.Name)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("referenced project not found", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewProjectReleaseBinding(testNamespace, "ghost", "dev", "ghost-dev")

		_, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrProjectNotFound)
	})

	t.Run("already exists", func(t *testing.T) {
		project := testutil.NewProject(testNamespace, testProjectName)
		existing := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "dup")
		svc := newService(t, project, existing)
		dup := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "dup")

		_, err := svc.CreateProjectReleaseBinding(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrProjectReleaseBindingAlreadyExists)
	})
}

func TestUpdateProjectReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success - advances projectRelease pin", func(t *testing.T) {
		existing := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "my-app-dev")
		svc := newService(t, existing)

		update := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "my-app-dev")
		update.Spec.ProjectRelease = "my-app-abc123"

		result, err := svc.UpdateProjectReleaseBinding(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, projectReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, "my-app-abc123", result.Spec.ProjectRelease)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateProjectReleaseBinding(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		rb := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "nonexistent")

		_, err := svc.UpdateProjectReleaseBinding(ctx, testNamespace, rb)
		require.ErrorIs(t, err, ErrProjectReleaseBindingNotFound)
	})
}

func TestListProjectReleaseBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		rb1 := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "b-1")
		rb2 := testutil.NewProjectReleaseBinding(testNamespace, "other-app", "dev", "b-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListProjectReleaseBindings(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, projectReleaseBindingTypeMeta, item.TypeMeta)
		}
	})

	t.Run("filter by project", func(t *testing.T) {
		rb1 := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "b-1")
		rb2 := testutil.NewProjectReleaseBinding(testNamespace, "other-app", "dev", "b-2")
		svc := newService(t, rb1, rb2)

		result, err := svc.ListProjectReleaseBindings(ctx, testNamespace, testProjectName, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "b-1", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListProjectReleaseBindings(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetProjectReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		rb := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "my-app-dev")
		svc := newService(t, rb)

		result, err := svc.GetProjectReleaseBinding(ctx, testNamespace, "my-app-dev")
		require.NoError(t, err)
		assert.Equal(t, projectReleaseBindingTypeMeta, result.TypeMeta)
		assert.Equal(t, "my-app-dev", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetProjectReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectReleaseBindingNotFound)
	})
}

func TestDeleteProjectReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rb := testutil.NewProjectReleaseBinding(testNamespace, testProjectName, "dev", "my-app-dev")
		svc := newService(t, rb)

		err := svc.DeleteProjectReleaseBinding(ctx, testNamespace, "my-app-dev")
		require.NoError(t, err)

		_, err = svc.GetProjectReleaseBinding(ctx, testNamespace, "my-app-dev")
		require.ErrorIs(t, err, ErrProjectReleaseBindingNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteProjectReleaseBinding(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectReleaseBindingNotFound)
	})
}
