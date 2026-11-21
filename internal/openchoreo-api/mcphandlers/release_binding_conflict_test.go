// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	endpointreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	resourcereleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// racingClient returns a fake client whose first `races` Updates are each preceded
// by another writer annotating the same object, so the handler's write carries a
// stale resourceVersion and conflicts, as when the portal edits a binding while
// an MCP tool updates it (#5370). updates counts the handler's Update calls.
func racingClient(t *testing.T, races int, objs ...client.Object) (client.Client, *int) {
	t.Helper()
	updates := 0
	fc := fake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				if updates <= races {
					other := obj.DeepCopyObject().(client.Object)
					require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), other))
					annotations := other.GetAnnotations()
					if annotations == nil {
						annotations = map[string]string{}
					}
					annotations["example/marker"] = "x"
					other.SetAnnotations(annotations)
					require.NoError(t, c.Update(ctx, other))
				}
				return c.Update(ctx, obj, opts...)
			},
		}).
		Build()
	return fc, &updates
}

func TestUpdateReleaseBindingRetriesOnConflict(t *testing.T) {
	ctx := context.Background()
	release := "web-r2"
	seed := func() []client.Object {
		// dev is the first environment of the pipeline, so any release may be set on it.
		return []client.Object{
			testutil.NewReleaseBinding(testNS, "proj", "web", "dev", "web-dev"),
			testutil.NewProject(testNS, "proj"),
			testutil.NewDeploymentPipeline(testNS, "default"),
		}
	}

	t.Run("a concurrent edit is kept and the release is still set", func(t *testing.T) {
		fc, updates := racingClient(t, 1, seed()...)
		h := newTestHandler(withReleaseBindingService(releasebindingsvc.NewService(fc, testutil.TestLogger())))

		_, err := h.UpdateReleaseBinding(ctx, testNS, "web-dev", &gen.ReleaseBindingSpec{ReleaseName: &release})
		require.NoError(t, err)
		assert.Equal(t, 2, *updates, "the conflicting write is retried once")

		stored := &openchoreov1alpha1.ReleaseBinding{}
		require.NoError(t, fc.Get(ctx, client.ObjectKey{Namespace: testNS, Name: "web-dev"}, stored))
		assert.Equal(t, release, stored.Spec.ReleaseName)
		assert.Equal(t, "x", stored.Annotations["example/marker"], "the concurrent edit must not be overwritten")
	})

	t.Run("gives up with a conflict when every write races", func(t *testing.T) {
		fc, updates := racingClient(t, 100, seed()...)
		h := newTestHandler(withReleaseBindingService(releasebindingsvc.NewService(fc, testutil.TestLogger())))

		_, err := h.UpdateReleaseBinding(ctx, testNS, "web-dev", &gen.ReleaseBindingSpec{ReleaseName: &release})
		require.ErrorIs(t, err, services.ErrConflict)
		assert.Equal(t, 5, *updates)
	})
}

func TestUpdateResourceReleaseBindingRetriesOnConflict(t *testing.T) {
	ctx := context.Background()
	release := "db-r2"
	fc, updates := racingClient(t, 1, testutil.NewResourceReleaseBinding(testNS, "proj", "db", "dev", "db-dev"))
	h := newTestHandler(withResourceReleaseBindingService(resourcereleasebindingsvc.NewService(fc, testutil.TestLogger())))

	req := &gen.ResourceReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "db-dev"},
		Spec:     &gen.ResourceReleaseBindingSpec{ResourceRelease: &release},
	}
	_, err := h.UpdateResourceReleaseBinding(ctx, testNS, req)
	require.NoError(t, err)
	assert.Equal(t, 2, *updates, "the conflicting write is retried once")

	stored := &openchoreov1alpha1.ResourceReleaseBinding{}
	require.NoError(t, fc.Get(ctx, client.ObjectKey{Namespace: testNS, Name: "db-dev"}, stored))
	assert.Equal(t, release, stored.Spec.ResourceRelease)
	assert.Equal(t, "x", stored.Annotations["example/marker"], "the concurrent edit must not be overwritten")
}

func TestUpdateEndpointReleaseBindingRetriesOnConflict(t *testing.T) {
	ctx := context.Background()
	release := "api-r2"
	fc, updates := racingClient(t, 1, testutil.NewEndpointReleaseBinding(testNS, "proj", "api", "dev", "api-dev"))
	h := newTestHandler(withEndpointReleaseBindingService(endpointreleasebindingsvc.NewService(fc, testutil.TestLogger())))

	req := &gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "api-dev"},
		Spec:     &gen.EndpointReleaseBindingSpec{EndpointRelease: &release},
	}
	_, err := h.UpdateEndpointReleaseBinding(ctx, testNS, req)
	require.NoError(t, err)
	assert.Equal(t, 2, *updates, "the conflicting write is retried once")

	stored := &openchoreov1alpha1.EndpointReleaseBinding{}
	require.NoError(t, fc.Get(ctx, client.ObjectKey{Namespace: testNS, Name: "api-dev"}, stored))
	assert.Equal(t, release, stored.Spec.EndpointRelease)
	assert.Equal(t, "x", stored.Annotations["example/marker"], "the concurrent edit must not be overwritten")
}
