// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

func TestListProjectReleases(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		pr1 := testutil.NewProjectRelease(testNamespace, testProjectName, "rel-1")
		pr2 := testutil.NewProjectRelease(testNamespace, "other-app", "rel-2")
		svc := newService(t, pr1, pr2)

		result, err := svc.ListProjectReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, projectReleaseTypeMeta, item.TypeMeta)
		}
	})

	t.Run("filter by project", func(t *testing.T) {
		pr1 := testutil.NewProjectRelease(testNamespace, testProjectName, "rel-1")
		pr2 := testutil.NewProjectRelease(testNamespace, "other-app", "rel-2")
		svc := newService(t, pr1, pr2)

		result, err := svc.ListProjectReleases(ctx, testNamespace, testProjectName, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "rel-1", result.Items[0].Name)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		prIn := testutil.NewProjectRelease(testNamespace, testProjectName, "rel-in")
		prOut := testutil.NewProjectRelease("other-ns", testProjectName, "rel-out")
		svc := newService(t, prIn, prOut)

		result, err := svc.ListProjectReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "rel-in", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListProjectReleases(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetProjectRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		pr := testutil.NewProjectRelease(testNamespace, testProjectName, "rel-1")
		svc := newService(t, pr)

		result, err := svc.GetProjectRelease(ctx, testNamespace, "rel-1")
		require.NoError(t, err)
		assert.Equal(t, projectReleaseTypeMeta, result.TypeMeta)
		assert.Equal(t, "rel-1", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetProjectRelease(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectReleaseNotFound)
	})
}

func TestCreateProjectRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		pr := testutil.NewProjectRelease(testNamespace, testProjectName, "rel-1")

		result, err := svc.CreateProjectRelease(ctx, testNamespace, pr)
		require.NoError(t, err)
		assert.Equal(t, projectReleaseTypeMeta, result.TypeMeta)
		assert.Equal(t, "rel-1", result.Name)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateProjectRelease(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrProjectReleaseNil)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewProjectRelease(testNamespace, testProjectName, "dup")
		svc := newService(t, existing)
		dup := testutil.NewProjectRelease(testNamespace, testProjectName, "dup")

		_, err := svc.CreateProjectRelease(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrProjectReleaseAlreadyExists)
	})
}

func TestDeleteProjectRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		pr := testutil.NewProjectRelease(testNamespace, testProjectName, "rel-1")
		svc := newService(t, pr)

		err := svc.DeleteProjectRelease(ctx, testNamespace, "rel-1")
		require.NoError(t, err)

		_, err = svc.GetProjectRelease(ctx, testNamespace, "rel-1")
		require.ErrorIs(t, err, ErrProjectReleaseNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteProjectRelease(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectReleaseNotFound)
	})
}
