// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func bindingsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	return scheme
}

func bindingOwner(name, uid string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "acme", UID: types.UID(uid)},
	}
}

func TestEmitObserved(t *testing.T) {
	scheme := bindingsScheme(t)
	owner := bindingOwner("api-prod", "owner-uid")
	other := bindingOwner("worker-prod", "other-uid")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, other).Build()
	ctx := context.Background()
	key := types.NamespacedName{Name: "api-prod", Namespace: "acme"}
	labels := map[string]string{"team": "payments"}
	spec := openchoreov1alpha1.RenderedReleaseSpec{
		EnvironmentName: "prod",
		TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
		ObservedResources: []openchoreov1alpha1.ObservedResource{
			{ID: "deployment-api", APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: "payments"},
		},
	}

	rr, op, err := EmitObserved(ctx, c, scheme, owner, key, labels, spec)
	require.NoError(t, err)
	assert.Equal(t, controllerutil.OperationResultCreated, op)
	assert.Equal(t, openchoreov1alpha1.RenderedReleaseModeObserve, rr.Spec.Mode, "the mode is always Observe")
	assert.Equal(t, labels, rr.Labels)
	assert.True(t, metav1.IsControlledBy(rr, owner))

	_, op, err = EmitObserved(ctx, c, scheme, owner, key, labels, spec)
	require.NoError(t, err)
	assert.Equal(t, controllerutil.OperationResultNone, op)

	// The fake client sets no UID, so seed the release the way the API server
	// returns it.
	held := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-prod", Namespace: "acme", UID: "release-uid"},
	}
	require.NoError(t, controllerutil.SetControllerReference(other, held, scheme))
	c = fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, other, held).Build()
	_, _, err = EmitObserved(ctx, c, scheme, owner, types.NamespacedName{Name: "worker-prod", Namespace: "acme"}, labels, spec)
	assert.ErrorIs(t, err, ErrNotOwned)
}

func TestGetObserving(t *testing.T) {
	scheme := bindingsScheme(t)
	owner := bindingOwner("api-prod", "owner-uid")
	other := bindingOwner("worker-prod", "other-uid")
	release := func(name string, mode openchoreov1alpha1.RenderedReleaseMode, controlledBy client.Object) *openchoreov1alpha1.RenderedRelease {
		rr := &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "acme"},
			Spec:       openchoreov1alpha1.RenderedReleaseSpec{Mode: mode},
		}
		require.NoError(t, controllerutil.SetControllerReference(controlledBy, rr, scheme))
		return rr
	}
	deleting := release("deleting", openchoreov1alpha1.RenderedReleaseModeObserve, owner)
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"test"}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		release("observing", openchoreov1alpha1.RenderedReleaseModeObserve, owner),
		release("managing", openchoreov1alpha1.RenderedReleaseModeManage, owner),
		release("others", openchoreov1alpha1.RenderedReleaseModeObserve, other),
		deleting,
	).Build()

	cases := []struct {
		name string
		want bool
	}{
		{"observing", true},
		{"managing", false},
		{"others", false},
		{"deleting", false},
		{"missing", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr, err := GetObserving(context.Background(), c, owner, types.NamespacedName{Name: tc.name, Namespace: "acme"})
			require.NoError(t, err)
			assert.Equal(t, tc.want, rr != nil)
		})
	}
}
