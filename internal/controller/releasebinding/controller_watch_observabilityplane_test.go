// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func observabilityPlaneObj(generation int64) *openchoreov1alpha1.ObservabilityPlane {
	return &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Generation: generation},
	}
}

func TestObservabilityPlaneChangedPredicate(t *testing.T) {
	p := observabilityPlaneChangedPredicate()

	assert.True(t, p.Create(event.CreateEvent{Object: observabilityPlaneObj(1)}), "create should pass")
	assert.True(t, p.Delete(event.DeleteEvent{Object: observabilityPlaneObj(1)}), "delete should pass")
	assert.False(t, p.Generic(event.GenericEvent{Object: observabilityPlaneObj(1)}), "generic should be ignored")
	assert.True(t, p.Update(event.UpdateEvent{
		ObjectOld: observabilityPlaneObj(1),
		ObjectNew: observabilityPlaneObj(2),
	}), "spec generation change should pass")
	assert.False(t, p.Update(event.UpdateEvent{
		ObjectOld: observabilityPlaneObj(1),
		ObjectNew: observabilityPlaneObj(1),
	}), "status-only update should be ignored")
}

func dataPlaneWithObservabilityRef(
	namespace, name string,
	ref *openchoreov1alpha1.ObservabilityPlaneRef,
) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ObservabilityPlaneRef: ref,
		},
	}
}

func clusterDataPlaneWithObservabilityRef(
	name string,
	ref *openchoreov1alpha1.ClusterObservabilityPlaneRef,
) *openchoreov1alpha1.ClusterDataPlane {
	return &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			ObservabilityPlaneRef: ref,
		},
	}
}

func TestFindReleaseBindingsForObservabilityPlane(t *testing.T) {
	ctx := context.Background()
	r := newReconcilerWith(t,
		dataPlaneWithObservabilityRef("org", "dp-a", &openchoreov1alpha1.ObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
			Name: "obs-a",
		}),
		dataPlaneWithObservabilityRef("org", "dp-b", &openchoreov1alpha1.ObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
			Name: "obs-b",
		}),
		dataPlaneWithObservabilityRef("org", "dp-cluster", &openchoreov1alpha1.ObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane,
			Name: "obs-a",
		}),
		dataPlaneWithObservabilityRef("org", "dp-default", nil),
		envRef("org", "env-a", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-a"),
		envRef("org", "env-b", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-b"),
		envRef("org", "env-cluster", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-cluster"),
		envRef("org", "env-default", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-default"),
		bindingFor("org", "rb-a", "env-a"),
		bindingFor("org", "rb-b", "env-b"),
		bindingFor("org", "rb-cluster", "env-cluster"),
		bindingFor("org", "rb-default", "env-default"),
	)

	t.Run("matches namespaced references by kind and name", func(t *testing.T) {
		reqs := r.findReleaseBindingsForObservabilityPlane(ctx, &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: "obs-a"},
		})
		assert.ElementsMatch(t, []string{"rb-a"}, requestNames(reqs))
	})

	t.Run("matches implicit default references", func(t *testing.T) {
		reqs := r.findReleaseBindingsForObservabilityPlane(ctx, &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: "default"},
		})
		assert.ElementsMatch(t, []string{"rb-default"}, requestNames(reqs))
	})
}

func TestFindReleaseBindingsForClusterObservabilityPlane(t *testing.T) {
	ctx := context.Background()
	r := newReconcilerWith(t,
		dataPlaneWithObservabilityRef("org-a", "dp-shared", &openchoreov1alpha1.ObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane,
			Name: "shared",
		}),
		dataPlaneWithObservabilityRef("org-a", "dp-local", &openchoreov1alpha1.ObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane,
			Name: "shared",
		}),
		dataPlaneWithObservabilityRef("org-a", "dp-default", nil),
		clusterDataPlaneWithObservabilityRef("cdp-shared", &openchoreov1alpha1.ClusterObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
			Name: "shared",
		}),
		clusterDataPlaneWithObservabilityRef("cdp-default", nil),
		envRef("org-a", "env-shared", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-shared"),
		envRef("org-a", "env-local", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-local"),
		envRef("org-a", "env-default", openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp-default"),
		envRef("org-b", "env-cdp", openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "cdp-shared"),
		envRef("org-b", "env-cdp-default", openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "cdp-default"),
		bindingFor("org-a", "rb-shared", "env-shared"),
		bindingFor("org-a", "rb-local", "env-local"),
		bindingFor("org-a", "rb-default", "env-default"),
		bindingFor("org-b", "rb-cdp", "env-cdp"),
		bindingFor("org-b", "rb-cdp-default", "env-cdp-default"),
	)

	t.Run("matches namespaced and cluster data plane references", func(t *testing.T) {
		reqs := r.findReleaseBindingsForClusterObservabilityPlane(ctx,
			&openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "shared"}})
		assert.ElementsMatch(t, []string{"rb-shared", "rb-cdp"}, requestNames(reqs))
	})

	t.Run("matches implicit default references", func(t *testing.T) {
		reqs := r.findReleaseBindingsForClusterObservabilityPlane(ctx,
			&openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
		assert.ElementsMatch(t, []string{"rb-default", "rb-cdp-default"}, requestNames(reqs))
	})
}
