// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace    = "test-ns"
	testProjectName  = "test-project"
	testResourceName = "test-r"
	testReleaseName  = "test-r-abc123"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestListResourceReleases(t *testing.T) {
	ctx := context.Background()

	t.Run("all without resource filter", func(t *testing.T) {
		rr1 := testutil.NewResourceRelease(testNamespace, testProjectName, "r-a", "rel-1")
		rr2 := testutil.NewResourceRelease(testNamespace, testProjectName, "r-b", "rel-2")
		svc := newService(t, rr1, rr2)

		result, err := svc.ListResourceReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, resourceReleaseTypeMeta, item.TypeMeta)
		}
	})

	t.Run("with resource filter", func(t *testing.T) {
		rr1 := testutil.NewResourceRelease(testNamespace, testProjectName, testResourceName, "rel-1")
		rr2 := testutil.NewResourceRelease(testNamespace, testProjectName, "other-r", "rel-2")
		svc := newService(t, rr1, rr2)

		result, err := svc.ListResourceReleases(ctx, testNamespace, testResourceName, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, testResourceName, result.Items[0].Spec.Owner.ResourceName)
	})

	t.Run("filtered path with limit", func(t *testing.T) {
		objs := make([]client.Object, 0, 5)
		for i := range 5 {
			rr := testutil.NewResourceRelease(testNamespace, testProjectName, testResourceName, fmt.Sprintf("rel-%d", i))
			objs = append(objs, rr)
		}
		svc := newService(t, objs...)

		result, err := svc.ListResourceReleases(ctx, testNamespace, testResourceName, services.ListOptions{Limit: 2})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(result.Items), 2)
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListResourceReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListResourceReleases(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		rrInNs := testutil.NewResourceRelease(testNamespace, testProjectName, testResourceName, "rel-in")
		rrOtherNs := testutil.NewResourceRelease("other-ns", testProjectName, testResourceName, "rel-out")
		svc := newService(t, rrInNs, rrOtherNs)

		result, err := svc.ListResourceReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "rel-in", result.Items[0].Name)
	})
}

func TestGetResourceRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		rr := testutil.NewResourceRelease(testNamespace, testProjectName, testResourceName, testReleaseName)
		svc := newService(t, rr)

		result, err := svc.GetResourceRelease(ctx, testNamespace, testReleaseName)
		require.NoError(t, err)
		assert.Equal(t, resourceReleaseTypeMeta, result.TypeMeta)
		assert.Equal(t, testReleaseName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetResourceRelease(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrResourceReleaseNotFound)
	})
}

func TestCreateResourceRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		// Use the same fixture the controller-cut release would produce — fully populated
		// ResourceType.Spec — to keep the asserted shape consistent with runtime behavior.
		rr := testutil.NewResourceRelease(testNamespace, testProjectName, testResourceName, testReleaseName)
		// Clear the Namespace so we can assert the service applies it.
		rr.Namespace = ""

		result, err := svc.CreateResourceRelease(ctx, testNamespace, rr)
		require.NoError(t, err)
		assert.Equal(t, resourceReleaseTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ResourceReleaseStatus{}, result.Status)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateResourceRelease(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrResourceReleaseNil)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewResourceRelease(testNamespace, testProjectName, testResourceName, testReleaseName)
		svc := newService(t, existing)
		dup := testutil.NewResourceRelease(testNamespace, testProjectName, testResourceName, testReleaseName)

		_, err := svc.CreateResourceRelease(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrResourceReleaseAlreadyExists)
	})
}

func TestDeleteResourceRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rr := testutil.NewResourceRelease(testNamespace, testProjectName, testResourceName, testReleaseName)
		svc := newService(t, rr)

		err := svc.DeleteResourceRelease(ctx, testNamespace, testReleaseName)
		require.NoError(t, err)

		_, err = svc.GetResourceRelease(ctx, testNamespace, testReleaseName)
		require.ErrorIs(t, err, ErrResourceReleaseNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteResourceRelease(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrResourceReleaseNotFound)
	})
}
