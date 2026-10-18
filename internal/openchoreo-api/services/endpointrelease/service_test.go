// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace    = "test-ns"
	testProjectName  = "test-project"
	testEndpointName = "test-r"
	testReleaseName  = "test-r-abc123"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestListEndpointReleases(t *testing.T) {
	ctx := context.Background()

	t.Run("all without endpoint filter", func(t *testing.T) {
		rr1 := testutil.NewEndpointRelease(testNamespace, testProjectName, "r-a", "rel-1")
		rr2 := testutil.NewEndpointRelease(testNamespace, testProjectName, "r-b", "rel-2")
		svc := newService(t, rr1, rr2)

		result, err := svc.ListEndpointReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, endpointReleaseTypeMeta, item.TypeMeta)
		}
	})

	t.Run("with endpoint filter", func(t *testing.T) {
		rr1 := testutil.NewEndpointRelease(testNamespace, testProjectName, testEndpointName, "rel-1")
		rr2 := testutil.NewEndpointRelease(testNamespace, testProjectName, "other-r", "rel-2")
		svc := newService(t, rr1, rr2)

		result, err := svc.ListEndpointReleases(ctx, testNamespace, testEndpointName, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, testEndpointName, result.Items[0].Spec.Owner.EndpointName)
	})

	t.Run("filtered path with limit", func(t *testing.T) {
		objs := make([]client.Object, 0, 5)
		for i := range 5 {
			rr := testutil.NewEndpointRelease(testNamespace, testProjectName, testEndpointName, fmt.Sprintf("rel-%d", i))
			objs = append(objs, rr)
		}
		svc := newService(t, objs...)

		result, err := svc.ListEndpointReleases(ctx, testNamespace, testEndpointName, services.ListOptions{Limit: 2})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(result.Items), 2)
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListEndpointReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListEndpointReleases(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		rrInNs := testutil.NewEndpointRelease(testNamespace, testProjectName, testEndpointName, "rel-in")
		rrOtherNs := testutil.NewEndpointRelease("other-ns", testProjectName, testEndpointName, "rel-out")
		svc := newService(t, rrInNs, rrOtherNs)

		result, err := svc.ListEndpointReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "rel-in", result.Items[0].Name)
	})
}

func TestGetEndpointRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		rr := testutil.NewEndpointRelease(testNamespace, testProjectName, testEndpointName, testReleaseName)
		svc := newService(t, rr)

		result, err := svc.GetEndpointRelease(ctx, testNamespace, testReleaseName)
		require.NoError(t, err)
		assert.Equal(t, endpointReleaseTypeMeta, result.TypeMeta)
		assert.Equal(t, testReleaseName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetEndpointRelease(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEndpointReleaseNotFound)
	})
}

func TestDeleteEndpointRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rr := testutil.NewEndpointRelease(testNamespace, testProjectName, testEndpointName, testReleaseName)
		svc := newService(t, rr)

		err := svc.DeleteEndpointRelease(ctx, testNamespace, testReleaseName)
		require.NoError(t, err)

		_, err = svc.GetEndpointRelease(ctx, testNamespace, testReleaseName)
		require.ErrorIs(t, err, ErrEndpointReleaseNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteEndpointRelease(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEndpointReleaseNotFound)
	})
}
