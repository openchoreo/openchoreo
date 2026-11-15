// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

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
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func observePRB(mode openchoreov1alpha1.BindingMode, namespace string) *openchoreov1alpha1.ProjectReleaseBinding {
	return &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-prod", Namespace: "acme"},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: "payments"},
			Environment: "prod",
			Mode:        mode,
		},
		Status: openchoreov1alpha1.ProjectReleaseBindingStatus{Namespace: namespace},
	}
}

func TestProjectReleaseBindingObserveChangedPredicate(t *testing.T) {
	p := projectReleaseBindingObserveChangedPredicate()
	observe := openchoreov1alpha1.BindingModeObserve

	assert.True(t, p.Create(event.CreateEvent{Object: observePRB(observe, "")}), "create should pass")
	assert.True(t, p.Delete(event.DeleteEvent{Object: observePRB(observe, "payments")}), "delete should pass")

	cases := []struct {
		name           string
		oldPRB, newPRB *openchoreov1alpha1.ProjectReleaseBinding
		want           bool
	}{
		{"namespace_reported", observePRB(observe, ""), observePRB(observe, "payments"), true},
		{"namespace_cleared", observePRB(observe, "payments"), observePRB(observe, ""), true},
		{"mode_changed", observePRB(openchoreov1alpha1.BindingModeManage, "payments"), observePRB(observe, "payments"), true},
		{"unchanged", observePRB(observe, "payments"), observePRB(observe, "payments"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, p.Update(event.UpdateEvent{ObjectOld: tc.oldPRB, ObjectNew: tc.newPRB}))
		})
	}
}

func TestFindReleaseBindingsForProjectReleaseBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	binding := func(name, project, env string, mode openchoreov1alpha1.BindingMode) client.Object {
		return &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "acme"},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: project, ComponentName: name},
				Environment: env,
				Mode:        mode,
			},
		}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		binding("api-prod", "payments", "prod", openchoreov1alpha1.BindingModeObserve),
		binding("worker-prod", "payments", "prod", openchoreov1alpha1.BindingModeManage),
		binding("api-dev", "payments", "dev", openchoreov1alpha1.BindingModeObserve),
		binding("orders-prod", "orders", "prod", openchoreov1alpha1.BindingModeObserve),
	).Build()
	r := &Reconciler{Client: c}

	got := r.findReleaseBindingsForProjectReleaseBinding(context.Background(), observePRB(openchoreov1alpha1.BindingModeObserve, "payments"))

	require.Len(t, got, 1)
	assert.Equal(t, types.NamespacedName{Namespace: "acme", Name: "api-prod"}, got[0].NamespacedName)
}

func TestFindProjectReleaseBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	prb := func(name, project, env string) client.Object {
		return &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "acme"},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: project},
				Environment: env,
			},
		}
	}
	r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		prb("payments-prod", "payments", "prod"),
		prb("payments-dev", "payments", "dev"),
		prb("orders-prod-a", "orders", "prod"),
		prb("orders-prod-b", "orders", "prod"),
	).Build()}
	binding := func(project, env string) *openchoreov1alpha1.ReleaseBinding {
		return &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "api-" + env, Namespace: "acme"},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: project, ComponentName: "api"},
				Environment: env,
			},
		}
	}
	ctx := context.Background()

	found, err := r.findProjectReleaseBinding(ctx, binding("payments", "prod"))
	require.NoError(t, err)
	assert.Equal(t, "payments-prod", found.Name)

	found, err = r.findProjectReleaseBinding(ctx, binding("payments", "staging"))
	require.NoError(t, err)
	assert.Nil(t, found)

	_, err = r.findProjectReleaseBinding(ctx, binding("orders", "prod"))
	assert.ErrorIs(t, err, errAmbiguousProjectReleaseBinding)
}
