// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Watch mappers", func() {
	watchCtx := context.Background()

	inNamespace := func(ep *openchoreov1alpha1.Endpoint, ns string) *openchoreov1alpha1.Endpoint {
		ep.Namespace = ns
		return ep
	}

	buildReconciler := func(objs ...client.Object) *Reconciler {
		cli := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(objs...).
			WithIndex(&openchoreov1alpha1.Endpoint{}, endpointTypeRefIndex, indexEndpointTypeRef).
			WithIndex(&openchoreov1alpha1.Endpoint{}, backendComponentIndex, indexBackendComponents).
			Build()
		return &Reconciler{Client: cli, Scheme: scheme.Scheme}
	}

	request := func(ns, name string) reconcile.Request {
		return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}
	}

	It("indexes each distinct backend component once, scoped to the Endpoint's project", func() {
		ep := newEndpoint("ep", "p", clusterTypeRef("t"),
			componentBackend("stable", "greeter", "http", 9),
			componentBackend("admin", "greeter", "admin", 1),
			componentBackend("canary", "greeter-v2", "http", 1))
		Expect(indexBackendComponents(ep)).To(Equal([]string{"p/greeter", "p/greeter-v2"}))
	})

	It("indexes an omitted type kind as a namespaced EndpointType", func() {
		ep := newEndpoint("ep", "p", openchoreov1alpha1.EndpointTypeRef{Name: "http"})
		Expect(indexEndpointTypeRef(ep)).To(Equal([]string{"EndpointType:http"}))
	})

	It("maps a namespaced EndpointType to referencing Endpoints in its namespace only", func() {
		r := buildReconciler(
			inNamespace(newEndpoint("a", "p", openchoreov1alpha1.EndpointTypeRef{Name: "http"}), "ns1"),
			inNamespace(newEndpoint("b", "p", openchoreov1alpha1.EndpointTypeRef{Name: "http"}), "ns2"),
			inNamespace(newEndpoint("c", "p", clusterTypeRef("http")), "ns1"),
		)
		et := &openchoreov1alpha1.EndpointType{ObjectMeta: metav1.ObjectMeta{Name: "http", Namespace: "ns1"}}
		Expect(r.listEndpointsForEndpointType(watchCtx, et)).To(ConsistOf(request("ns1", "a")))
	})

	It("maps a ClusterEndpointType to referencing Endpoints in every namespace", func() {
		r := buildReconciler(
			inNamespace(newEndpoint("a", "p", clusterTypeRef("http")), "ns1"),
			inNamespace(newEndpoint("b", "p", clusterTypeRef("http")), "ns2"),
			inNamespace(newEndpoint("c", "p", openchoreov1alpha1.EndpointTypeRef{Name: "http"}), "ns1"),
		)
		cet := &openchoreov1alpha1.ClusterEndpointType{ObjectMeta: metav1.ObjectMeta{Name: "http"}}
		Expect(r.listEndpointsForClusterEndpointType(watchCtx, cet)).
			To(ConsistOf(request("ns1", "a"), request("ns2", "b")))
	})

	It("maps a Workload to the Endpoints routing to its component in the same project", func() {
		r := buildReconciler(
			inNamespace(newEndpoint("routes", "p", clusterTypeRef("t"), componentBackend("s", "greeter", "http", 1)), "ns1"),
			inNamespace(newEndpoint("other-project", "q", clusterTypeRef("t"), componentBackend("s", "greeter", "http", 1)), "ns1"),
			inNamespace(newEndpoint("other-component", "p", clusterTypeRef("t"), componentBackend("s", "billing", "http", 1)), "ns1"),
		)
		w := newWorkload("p", "greeter", "http")
		w.Namespace = "ns1"
		Expect(r.listEndpointsForWorkload(watchCtx, w)).To(ConsistOf(request("ns1", "routes")))
	})
})
