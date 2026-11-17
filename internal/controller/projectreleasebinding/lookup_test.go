// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func lookupPRB(name, project, env string, mode openchoreov1alpha1.BindingMode, namespace string) *openchoreov1alpha1.ProjectReleaseBinding {
	return &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "acme"},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: project},
			Environment: env,
			Mode:        mode,
		},
		Status: openchoreov1alpha1.ProjectReleaseBindingStatus{Namespace: namespace},
	}
}

func lookupClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestFind(t *testing.T) {
	manage := openchoreov1alpha1.BindingModeManage
	deleting := lookupPRB("payments-dev-old", "payments", "dev", manage, "")
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"test"}
	c := lookupClient(t,
		lookupPRB("payments-prod", "payments", "prod", manage, ""),
		lookupPRB("payments-dev", "payments", "dev", manage, ""),
		deleting,
		lookupPRB("orders-prod-a", "orders", "prod", manage, ""),
		lookupPRB("orders-prod-b", "orders", "prod", manage, ""),
	)
	ctx := context.Background()

	found, err := Find(ctx, c, "acme", "payments", "prod")
	require.NoError(t, err)
	assert.Equal(t, "payments-prod", found.Name)

	found, err = Find(ctx, c, "acme", "payments", "dev")
	require.NoError(t, err, "a binding being deleted is not counted")
	assert.Equal(t, "payments-dev", found.Name)

	found, err = Find(ctx, c, "acme", "payments", "staging")
	require.NoError(t, err)
	assert.Nil(t, found)

	found, err = Find(ctx, c, "other", "payments", "prod")
	require.NoError(t, err, "only the given namespace is searched")
	assert.Nil(t, found)

	_, err = Find(ctx, c, "acme", "orders", "prod")
	assert.ErrorIs(t, err, ErrAmbiguous)
}

func TestObservedNamespace(t *testing.T) {
	observe := openchoreov1alpha1.BindingModeObserve
	notAllowed := lookupPRB("billing-prod", "billing", "prod", observe, "")
	notAllowed.Status.Conditions = []metav1.Condition{{
		Type:    string(ConditionReady),
		Status:  metav1.ConditionFalse,
		Reason:  "NamespaceNotAllowed",
		Message: `Namespace "billing" is not allowed`,
	}}
	c := lookupClient(t,
		lookupPRB("payments-prod", "payments", "prod", observe, "payments"),
		lookupPRB("payments-dev", "payments", "dev", openchoreov1alpha1.BindingModeManage, "dp-acme-payments-dev"),
		notAllowed,
		lookupPRB("orders-prod-a", "orders", "prod", observe, "orders"),
		lookupPRB("orders-prod-b", "orders", "prod", observe, "orders"),
	)

	cases := []struct {
		name, project, env string
		wantNamespace      string
		wantReason         controller.ConditionReason
		wantMessage        string
	}{
		{"observed", "payments", "prod", "payments", "", ""},
		{"none", "payments", "staging", "", ReasonProjectReleaseBindingNotFound, "No ProjectReleaseBinding"},
		{"managed", "payments", "dev", "", ReasonProjectNotObserved, "is in Manage mode"},
		{"not observing", "billing", "prod", "", ReasonNamespaceNotObserved, `is not observing a namespace: Namespace "billing" is not allowed`},
		{"several", "orders", "prod", "", ReasonProjectReleaseBindingConflict, "Several ProjectReleaseBindings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns, reason, msg, err := ObservedNamespace(context.Background(), c, "acme", tc.project, tc.env)
			require.NoError(t, err)
			assert.Equal(t, tc.wantNamespace, ns)
			assert.Equal(t, tc.wantReason, reason)
			assert.Contains(t, msg, tc.wantMessage)
		})
	}
}

func TestObserveChangedPredicate(t *testing.T) {
	p := ObserveChangedPredicate()
	observe := openchoreov1alpha1.BindingModeObserve
	prb := func(mode openchoreov1alpha1.BindingMode, namespace string) *openchoreov1alpha1.ProjectReleaseBinding {
		return lookupPRB("payments-prod", "payments", "prod", mode, namespace)
	}

	assert.True(t, p.Create(event.CreateEvent{Object: prb(observe, "")}), "create should pass")
	assert.True(t, p.Delete(event.DeleteEvent{Object: prb(observe, "payments")}), "delete should pass")

	cases := []struct {
		name           string
		oldPRB, newPRB *openchoreov1alpha1.ProjectReleaseBinding
		want           bool
	}{
		{"namespace_reported", prb(observe, ""), prb(observe, "payments"), true},
		{"namespace_cleared", prb(observe, "payments"), prb(observe, ""), true},
		{"mode_changed", prb(openchoreov1alpha1.BindingModeManage, "payments"), prb(observe, "payments"), true},
		{"unchanged", prb(observe, "payments"), prb(observe, "payments"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, p.Update(event.UpdateEvent{ObjectOld: tc.oldPRB, ObjectNew: tc.newPRB}))
		})
	}
}
