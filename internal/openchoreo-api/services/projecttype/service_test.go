// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

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

const testNamespace = "test-ns"

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestCreateProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		pt := testutil.NewProjectType(testNamespace, "test-pt")

		result, err := svc.CreateProjectType(ctx, testNamespace, pt)
		require.NoError(t, err)
		assert.Equal(t, projectTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-pt", result.Name)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ProjectTypeStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateProjectType(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewProjectType(testNamespace, "dup-pt")
		svc := newService(t, existing)
		dup := testutil.NewProjectType(testNamespace, "dup-pt")

		_, err := svc.CreateProjectType(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrProjectTypeAlreadyExists)
	})
}

func TestUpdateProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewProjectType(testNamespace, "test-pt")
		svc := newService(t, existing)

		update := testutil.NewProjectType(testNamespace, "test-pt")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateProjectType(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, projectTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateProjectType(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		pt := testutil.NewProjectType(testNamespace, "nonexistent")

		_, err := svc.UpdateProjectType(ctx, testNamespace, pt)
		require.ErrorIs(t, err, ErrProjectTypeNotFound)
	})
}

func TestListProjectTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		pt1 := testutil.NewProjectType(testNamespace, "pt-1")
		pt2 := testutil.NewProjectType(testNamespace, "pt-2")
		svc := newService(t, pt1, pt2)

		result, err := svc.ListProjectTypes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, projectTypeTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListProjectTypes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		ptIn := testutil.NewProjectType(testNamespace, "pt-in")
		ptOut := testutil.NewProjectType("other-ns", "pt-out")
		svc := newService(t, ptIn, ptOut)

		result, err := svc.ListProjectTypes(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "pt-in", result.Items[0].Name)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListProjectTypes(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		pt := testutil.NewProjectType(testNamespace, "test-pt")
		svc := newService(t, pt)

		result, err := svc.GetProjectType(ctx, testNamespace, "test-pt")
		require.NoError(t, err)
		assert.Equal(t, projectTypeTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-pt", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetProjectType(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectTypeNotFound)
	})
}

func TestDeleteProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		pt := testutil.NewProjectType(testNamespace, "test-pt")
		svc := newService(t, pt)

		err := svc.DeleteProjectType(ctx, testNamespace, "test-pt")
		require.NoError(t, err)

		_, err = svc.GetProjectType(ctx, testNamespace, "test-pt")
		require.ErrorIs(t, err, ErrProjectTypeNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteProjectType(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectTypeNotFound)
	})
}

func TestGetProjectTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("success with nil params", func(t *testing.T) {
		pt := testutil.NewProjectType(testNamespace, "no-params")
		svc := newService(t, pt)

		result, err := svc.GetProjectTypeSchema(ctx, testNamespace, "no-params")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("success with OpenAPIV3 schema", func(t *testing.T) {
		pt := testutil.NewProjectType(testNamespace, "with-schema")
		pt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":"object","properties":{"tier":{"type":"string"}}}`)},
		}
		svc := newService(t, pt)

		result, err := svc.GetProjectTypeSchema(ctx, testNamespace, "with-schema")
		require.NoError(t, err)
		assert.Equal(t, "object", result["type"])
		props, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "tier")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetProjectTypeSchema(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrProjectTypeNotFound)
	})

	t.Run("invalid schema data", func(t *testing.T) {
		pt := testutil.NewProjectType(testNamespace, "bad-schema")
		pt.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid}`)},
		}
		svc := newService(t, pt)

		_, err := svc.GetProjectTypeSchema(ctx, testNamespace, "bad-schema")
		require.Error(t, err)
	})
}
