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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func dataPlaneObj(generation int64, annotations map[string]string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Generation: generation, Annotations: annotations},
	}
}

// TestDataPlaneRenderInputsChangedPredicate locks the contract that a data plane re-renders
// dependent bindings on a spec change or an openchoreo.dev/-prefixed annotation change, but
// not on status-only churn or third-party annotations (GitOps stamps, kubectl metadata).
func TestDataPlaneRenderInputsChangedPredicate(t *testing.T) {
	p := dataPlaneRenderInputsChangedPredicate()

	assert.True(t, p.Create(event.CreateEvent{Object: dataPlaneObj(1, nil)}), "create should pass")
	assert.False(t, p.Delete(event.DeleteEvent{Object: dataPlaneObj(1, nil)}), "delete should be ignored")
	assert.False(t, p.Generic(event.GenericEvent{Object: dataPlaneObj(1, nil)}), "generic should be ignored")

	const oc = "openchoreo.dev/scaling"
	cases := []struct {
		name         string
		oldDP, newDP *openchoreov1alpha1.DataPlane
		want         bool
	}{
		{"openchoreo_annotation_added", dataPlaneObj(1, nil), dataPlaneObj(1, map[string]string{oc: "knative"}), true},
		{"openchoreo_annotation_value_changed", dataPlaneObj(1, map[string]string{oc: "knative"}), dataPlaneObj(1, map[string]string{oc: "keda"}), true},
		{"openchoreo_annotation_removed", dataPlaneObj(1, map[string]string{oc: "knative"}), dataPlaneObj(1, nil), true},
		{"spec_generation_changed", dataPlaneObj(1, map[string]string{oc: "knative"}), dataPlaneObj(2, map[string]string{oc: "knative"}), true},
		{"third_party_annotation_added", dataPlaneObj(1, nil), dataPlaneObj(1, map[string]string{"fluxcd.io/sync": "ts"}), false},
		{"last_applied_config_added", dataPlaneObj(1, nil), dataPlaneObj(1, map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"}), false},
		{"third_party_changes_openchoreo_stable", dataPlaneObj(1, map[string]string{oc: "knative", "fluxcd.io/sync": "a"}), dataPlaneObj(1, map[string]string{oc: "knative", "fluxcd.io/sync": "b"}), false},
		{"no_op_same_gen_and_annotations", dataPlaneObj(1, map[string]string{oc: "knative"}), dataPlaneObj(1, map[string]string{oc: "knative"}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Update(event.UpdateEvent{ObjectOld: tc.oldDP, ObjectNew: tc.newDP})
			assert.Equal(t, tc.want, got)
		})
	}
}

func watchTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(s))
	return s
}

func envRef(ns, name string, kind openchoreov1alpha1.DataPlaneRefKind, dpName string) *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{Kind: kind, Name: dpName},
		},
	}
}

func bindingFor(ns, name, environment string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       openchoreov1alpha1.ReleaseBindingSpec{Environment: environment},
	}
}

func requestNames(reqs []reconcile.Request) []string {
	out := make([]string, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, r.Name)
	}
	return out
}

func newReconcilerWith(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(watchTestScheme(t)).WithObjects(objs...).Build()
	return &Reconciler{Client: c}
}

// TestFindReleaseBindingsForDataPlane: a namespaced DataPlane change enqueues only the bindings
// whose target environment references that DataPlane by matching kind+name, within its namespace.
func TestFindReleaseBindingsForDataPlane(t *testing.T) {
	ctx := context.Background()
	r := newReconcilerWith(t,
		envRef("org", "env-a", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-a"),
		envRef("org", "env-b", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-b"),
		envRef("org", "env-cdp", openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "dp-a"), // kind mismatch
		bindingFor("org", "rb-a", "env-a"),
		bindingFor("org", "rb-b", "env-b"),
		bindingFor("org", "rb-cdp", "env-cdp"),
	)

	t.Run("matches_only_bindings_for_that_dataplane", func(t *testing.T) {
		reqs := r.findReleaseBindingsForDataPlane(ctx,
			&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: "dp-a"}})
		assert.ElementsMatch(t, []string{"rb-a"}, requestNames(reqs))
	})

	t.Run("no_referencing_environment_yields_nothing", func(t *testing.T) {
		reqs := r.findReleaseBindingsForDataPlane(ctx,
			&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: "dp-none"}})
		assert.Empty(t, reqs)
	})

	t.Run("environment_without_binding_yields_nothing", func(t *testing.T) {
		r2 := newReconcilerWith(t,
			envRef("org", "env-only", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-x"),
		)
		reqs := r2.findReleaseBindingsForDataPlane(ctx,
			&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: "dp-x"}})
		assert.Empty(t, reqs)
	})
}

// TestFindReleaseBindingsForClusterDataPlane: a cluster-scoped DataPlane change scans every
// namespace and enqueues the bindings of environments that reference it (kind ClusterDataPlane),
// while a namespaced DataPlane of the same name must not match.
func TestFindReleaseBindingsForClusterDataPlane(t *testing.T) {
	ctx := context.Background()
	r := newReconcilerWith(t,
		envRef("org1", "env-1", openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "cdp"),
		envRef("org2", "env-2", openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "cdp"),
		envRef("org1", "env-ns", openchoreov1alpha1.DataPlaneRefKindDataPlane, "cdp"), // kind mismatch
		bindingFor("org1", "rb-1", "env-1"),
		bindingFor("org2", "rb-2", "env-2"),
		bindingFor("org1", "rb-ns", "env-ns"),
	)

	reqs := r.findReleaseBindingsForClusterDataPlane(ctx,
		&openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "cdp"}})
	assert.ElementsMatch(t, []string{"rb-1", "rb-2"}, requestNames(reqs))
}
