// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func watchTestReconciler(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	cli := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithIndex(&openchoreov1alpha1.Component{}, componentTypeIndex, indexComponentTypeRef).
		WithIndex(&openchoreov1alpha1.Component{}, traitsIndex, indexComponentTraits).
		WithIndex(&openchoreov1alpha1.ComponentType{}, traitsIndex, indexComponentTypeTraits).
		WithIndex(&openchoreov1alpha1.ClusterComponentType{}, traitsIndex, indexClusterComponentTypeTraits).
		Build()
	return &Reconciler{Client: cli, Scheme: s}
}

func watchComponent(ns, name string, typeKind openchoreov1alpha1.ComponentTypeRefKind, typeName string,
	traits ...openchoreov1alpha1.ComponentTrait) *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: openchoreov1alpha1.ComponentSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Kind: typeKind, Name: typeName},
			Traits:        traits,
		},
	}
}

func watchComponentType(ns, name string, traits ...openchoreov1alpha1.ComponentTypeTrait) *openchoreov1alpha1.ComponentType {
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment", Traits: traits},
	}
}

func watchRequest(ns, name string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}
}

func assertRequests(t *testing.T, got []reconcile.Request, want ...reconcile.Request) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d requests %v, want %v", len(got), got, want)
	}
	gotSet := make(map[reconcile.Request]bool, len(got))
	for _, r := range got {
		gotSet[r] = true
	}
	for _, w := range want {
		if !gotSet[w] {
			t.Fatalf("missing request %v in %v", w, got)
		}
	}
}

const ctKind = openchoreov1alpha1.ComponentTypeRefKindComponentType
const cctKind = openchoreov1alpha1.ComponentTypeRefKindClusterComponentType

func TestIndexTraitsDefaultsKind(t *testing.T) {
	const traitA, clusterTraitB = "Trait:a", "ClusterTrait:b"
	comp := watchComponent("ns", "c", ctKind, "deployment/web",
		openchoreov1alpha1.ComponentTrait{Name: "a", InstanceName: "a1"},
		openchoreov1alpha1.ComponentTrait{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "b", InstanceName: "b1"})
	if got := indexComponentTraits(comp); len(got) != 2 || got[0] != traitA || got[1] != clusterTraitB {
		t.Fatalf("component index = %v", got)
	}
	ct := watchComponentType("ns", "web",
		openchoreov1alpha1.ComponentTypeTrait{Name: "a", InstanceName: "a1"},
		openchoreov1alpha1.ComponentTypeTrait{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "b", InstanceName: "b1"})
	if got := indexComponentTypeTraits(ct); len(got) != 2 || got[0] != traitA || got[1] != clusterTraitB {
		t.Fatalf("component type index = %v", got)
	}
	cct := &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			Traits: []openchoreov1alpha1.ClusterComponentTypeTrait{{Name: "b", InstanceName: "b1"}},
		},
	}
	if got := indexClusterComponentTypeTraits(cct); len(got) != 1 || got[0] != clusterTraitB {
		t.Fatalf("cluster component type index = %v", got)
	}
}

func TestListComponentsUsingTraitIsNamespaceScoped(t *testing.T) {
	ref := openchoreov1alpha1.ComponentTrait{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "autoscaler", InstanceName: "as"}
	r := watchTestReconciler(t,
		watchComponent("ns1", "direct", ctKind, "deployment/web", ref),
		watchComponent("ns2", "other-ns", ctKind, "deployment/web", ref),
	)
	trait := &openchoreov1alpha1.Trait{ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "autoscaler"}}
	assertRequests(t, r.listComponentsUsingTrait(context.Background(), trait), watchRequest("ns1", "direct"))
}

func TestListComponentsUsingTraitIncludesEmbeddedTraits(t *testing.T) {
	embedded := openchoreov1alpha1.ComponentTypeTrait{Name: "autoscaler", InstanceName: "as"}
	direct := openchoreov1alpha1.ComponentTrait{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "autoscaler", InstanceName: "as2"}
	r := watchTestReconciler(t,
		watchComponentType("ns1", "web", embedded),
		watchComponentType("ns1", "plain"),
		watchComponentType("ns2", "web", embedded),
		watchComponent("ns1", "via-type", ctKind, "deployment/web"),
		watchComponent("ns1", "both", ctKind, "deployment/web", direct),
		watchComponent("ns1", "plain-type", ctKind, "deployment/plain"),
		watchComponent("ns2", "other-ns", ctKind, "deployment/web"),
	)
	trait := &openchoreov1alpha1.Trait{ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "autoscaler"}}
	assertRequests(t, r.listComponentsUsingTrait(context.Background(), trait),
		watchRequest("ns1", "via-type"), watchRequest("ns1", "both"))
}

func TestListComponentsUsingClusterTraitSpansNamespacesAndTypes(t *testing.T) {
	clusterRef := openchoreov1alpha1.ComponentTrait{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "rate-limiter", InstanceName: "rl"}
	nsRef := openchoreov1alpha1.ComponentTrait{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "rate-limiter", InstanceName: "rl"}
	cct := &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "svc"},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			WorkloadType: "deployment",
			Traits:       []openchoreov1alpha1.ClusterComponentTypeTrait{{Name: "rate-limiter", InstanceName: "rl"}},
		},
	}
	r := watchTestReconciler(t,
		cct,
		watchComponentType("ns2", "web", openchoreov1alpha1.ComponentTypeTrait{
			Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "rate-limiter", InstanceName: "rl"}),
		watchComponent("ns1", "direct", ctKind, "deployment/other", clusterRef),
		watchComponent("ns2", "via-type", ctKind, "deployment/web"),
		watchComponent("ns3", "via-cluster-type", cctKind, "deployment/svc"),
		watchComponent("ns1", "namespaced-trait", ctKind, "deployment/other", nsRef),
	)
	clusterTrait := &openchoreov1alpha1.ClusterTrait{ObjectMeta: metav1.ObjectMeta{Name: "rate-limiter"}}
	assertRequests(t, r.listComponentsUsingClusterTrait(context.Background(), clusterTrait),
		watchRequest("ns1", "direct"), watchRequest("ns2", "via-type"), watchRequest("ns3", "via-cluster-type"))
}
