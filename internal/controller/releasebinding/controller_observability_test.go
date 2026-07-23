// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func obsSchemeForTest(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(s))
	return s
}

func obsFixtures() (
	*openchoreov1alpha1.ReleaseBinding,
	*openchoreov1alpha1.ComponentRelease,
	*openchoreov1alpha1.DataPlane,
	[]openchoreov1alpha1.RenderedManifest,
) {
	rb := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "ns", UID: "rb-uid"},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "proj", ComponentName: "comp"},
			Environment: "dev",
		},
	}
	cr := &openchoreov1alpha1.ComponentRelease{
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{ComponentName: "comp"},
		},
	}
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "obs",
			},
		},
	}
	obsResources := []openchoreov1alpha1.RenderedManifest{
		{ID: "res-1", Object: &runtime.RawExtension{Raw: []byte(`{}`)}},
	}
	return rb, cr, dp, obsResources
}

// A transient ObservabilityPlane lookup failure must requeue (return an error) rather than
// silently skipping, and it must not tear down an already-managed observability Release.
func TestReconcileObservabilityRelease_transientFailureRequeuesAndKeepsRelease(t *testing.T) {
	scheme := obsSchemeForTest(t)
	rb, cr, dp, obsResources := obsFixtures()
	releaseName := makeObservabilityReleaseName(cr, rb)

	existing := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: releaseName, Namespace: rb.Namespace},
	}
	require.NoError(t, controllerutil.SetControllerReference(rb, existing, scheme))

	transientErr := errors.New("etcd unavailable")
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*openchoreov1alpha1.ObservabilityPlane); ok {
					return transientErr
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	r := &Reconciler{Client: c, Scheme: scheme}
	dpResult := &controller.DataPlaneResult{DataPlane: dp}

	_, err := r.reconcileObservabilityRelease(context.Background(), rb, cr, dpResult, obsResources)

	// assert (not require) so the release-survival check below still runs on the buggy code path,
	// where a missing requeue is exactly what leads to the existing release being torn down.
	assert.Error(t, err, "a transient ObservabilityPlane lookup failure must requeue, not skip")
	assert.ErrorIs(t, err, transientErr)

	got := &openchoreov1alpha1.RenderedRelease{}
	require.NoError(t,
		c.Get(context.Background(), types.NamespacedName{Name: releaseName, Namespace: rb.Namespace}, got),
		"an existing observability Release must survive a transient lookup failure")

	// The transient failure must be surfaced on the status so Ready does not stay stale-True,
	// matching the create/update error path in the same reconcile.
	cond := apimeta.FindStatusCondition(rb.Status.Conditions, string(ConditionReleaseSynced))
	require.NotNil(t, cond, "ReleaseSynced condition must be set on a transient failure")
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, string(ReasonReleaseUpdateFailed), cond.Reason)
}

// A genuinely absent ObservabilityPlane is a real skip, not a transient failure, so the reconcile
// must not requeue. This also proves not-found is distinguished from transient errors.
func TestReconcileObservabilityRelease_notFoundSkipsWithoutRequeue(t *testing.T) {
	scheme := obsSchemeForTest(t)
	rb, cr, dp, obsResources := obsFixtures()

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: c, Scheme: scheme}
	dpResult := &controller.DataPlaneResult{DataPlane: dp}

	res, err := r.reconcileObservabilityRelease(context.Background(), rb, cr, dpResult, obsResources)

	require.NoError(t, err, "a genuinely absent ObservabilityPlane is a skip, not a retry")
	assert.False(t, res.managed)
	assert.NotEmpty(t, res.skipReason)
}

// A transient failure reading the existing observability Release during cleanup must requeue,
// otherwise a stale Release could be left behind with no retry.
func TestReconcileObservabilityRelease_cleanupGetFailureRequeues(t *testing.T) {
	scheme := obsSchemeForTest(t)
	rb, cr, dp, _ := obsFixtures()

	transientErr := errors.New("etcd unavailable")
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*openchoreov1alpha1.RenderedRelease); ok {
					return transientErr
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	r := &Reconciler{Client: c, Scheme: scheme}
	dpResult := &controller.DataPlaneResult{DataPlane: dp}

	// No observability resources routes into the cleanup path, where the Release lookup fails.
	_, err := r.reconcileObservabilityRelease(context.Background(), rb, cr, dpResult, nil)

	require.Error(t, err, "a transient failure reading the existing Release for cleanup must requeue")
	assert.ErrorIs(t, err, transientErr)
}
