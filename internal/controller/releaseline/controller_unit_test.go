// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestIndexTraits(t *testing.T) {
	line := &openchoreov1alpha1.ReleaseLine{Spec: openchoreov1alpha1.ReleaseLineSpec{Traits: []openchoreov1alpha1.ComponentTrait{
		{Name: "alert", InstanceName: "a"},
		{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "health-probes", InstanceName: "h"},
	}}}
	assert.Equal(t, []string{"Trait:alert", "ClusterTrait:health-probes"}, indexTraits(line))
}

func TestReconcileWritesStatusAfterAConcurrentWrite(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	line := &openchoreov1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{Name: "next", Namespace: "ns", Finalizers: []string{ReleaseLineFinalizer}},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: "proj", ComponentName: "missing"},
		},
	}
	key := client.ObjectKeyFromObject(line)

	touched := false
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(line).WithStatusSubresource(line).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, k client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := cl.Get(ctx, k, obj, opts...); err != nil {
					return err
				}
				if _, ok := obj.(*openchoreov1alpha1.ReleaseLine); !ok || touched {
					return nil
				}
				// Another writer changes the line after the reconciler has read it.
				touched = true
				latest := &openchoreov1alpha1.ReleaseLine{}
				if err := cl.Get(ctx, k, latest); err != nil {
					return err
				}
				latest.Labels = map[string]string{"touched": "true"}
				return cl.Update(ctx, latest)
			},
		}).Build()

	_, err := (&Reconciler{Client: c, Scheme: scheme}).Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	require.NoError(t, err)

	got := &openchoreov1alpha1.ReleaseLine{}
	require.NoError(t, c.Get(context.Background(), key, got))
	assert.Equal(t, "true", got.Labels["touched"])
	ready := meta.FindStatusCondition(got.Status.Conditions, string(ConditionReady))
	require.NotNil(t, ready)
	assert.Equal(t, string(ReasonComponentNotFound), ready.Reason)
}
