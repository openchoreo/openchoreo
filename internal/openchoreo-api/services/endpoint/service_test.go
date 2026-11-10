// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace = "test-ns"
	testProject   = "test-project"
)

// newService creates a Endpoint service plus the project fixture it depends on
// for the project-existence check on Create / List.
func newService(t *testing.T, extra ...client.Object) Service {
	t.Helper()
	objs := make([]client.Object, 0, 1+len(extra))
	objs = append(objs, testutil.NewProject(testNamespace, testProject))
	objs = append(objs, extra...)
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")

		result, err := svc.CreateEndpoint(ctx, testNamespace, r)
		require.NoError(t, err)
		assert.Equal(t, endpointTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-r", result.Name)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, testProject, result.Labels[labels.LabelKeyProjectName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateEndpoint(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("project not found", func(t *testing.T) {
		svc := NewService(testutil.NewFakeClient(), testutil.TestLogger())
		r := testutil.NewEndpoint(testNamespace, "missing-project", "test-r")

		_, err := svc.CreateEndpoint(ctx, testNamespace, r)
		require.ErrorIs(t, err, projectsvc.ErrProjectNotFound)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewEndpoint(testNamespace, testProject, "dup")
		svc := newService(t, existing)
		dup := testutil.NewEndpoint(testNamespace, testProject, "dup")

		_, err := svc.CreateEndpoint(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrEndpointAlreadyExists)
	})

	t.Run("namespace overrides body namespace", func(t *testing.T) {
		svc := newService(t)
		r := testutil.NewEndpoint("ignored", testProject, "test-r")

		result, err := svc.CreateEndpoint(ctx, testNamespace, r)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})
}

func TestUpdateEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		svc := newService(t, existing)

		update := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateEndpoint(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, endpointTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, testProject, result.Labels[labels.LabelKeyProjectName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateEndpoint(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		r := testutil.NewEndpoint(testNamespace, testProject, "nonexistent")

		_, err := svc.UpdateEndpoint(ctx, testNamespace, r)
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})

	t.Run("project reassignment rejected", func(t *testing.T) {
		existing := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		svc := newService(t, existing)

		update := testutil.NewEndpoint(testNamespace, "different-project", "test-r")

		_, err := svc.UpdateEndpoint(ctx, testNamespace, update)
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
		assert.Contains(t, validationErr.Msg, "immutable")
	})
}

func TestListEndpoints(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		r1 := testutil.NewEndpoint(testNamespace, testProject, "r-1")
		r2 := testutil.NewEndpoint(testNamespace, testProject, "r-2")
		svc := newService(t, r1, r2)

		result, err := svc.ListEndpoints(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, endpointTypeMeta, item.TypeMeta)
		}
	})

	t.Run("filters by project", func(t *testing.T) {
		// Two projects in the same namespace
		other := testutil.NewProject(testNamespace, "other-project")
		r1 := testutil.NewEndpoint(testNamespace, testProject, "r-mine")
		r2 := testutil.NewEndpoint(testNamespace, "other-project", "r-other")
		svc := newService(t, other, r1, r2)

		result, err := svc.ListEndpoints(ctx, testNamespace, testProject, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "r-mine", result.Items[0].Name)
	})

	t.Run("project filter rejects missing project", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListEndpoints(ctx, testNamespace, "missing", services.ListOptions{})
		require.ErrorIs(t, err, projectsvc.ErrProjectNotFound)
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListEndpoints(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListEndpoints(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		svc := newService(t, r)

		result, err := svc.GetEndpoint(ctx, testNamespace, "test-r")
		require.NoError(t, err)
		assert.Equal(t, endpointTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-r", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetEndpoint(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})
}

func TestDeleteEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		svc := newService(t, r)

		err := svc.DeleteEndpoint(ctx, testNamespace, "test-r")
		require.NoError(t, err)

		_, err = svc.GetEndpoint(ctx, testNamespace, "test-r")
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteEndpoint(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})
}
