// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Watch mappers", func() {
	newRes := func(name, ns string, kind openchoreov1alpha1.ResourceTypeRefKind, refName string) *openchoreov1alpha1.Resource {
		return &openchoreov1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.ResourceSpec{
				Owner: openchoreov1alpha1.ResourceOwner{ProjectName: "p"},
				Type:  openchoreov1alpha1.ResourceTypeRef{Kind: kind, Name: refName},
			},
		}
	}

	buildClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(objs...).
			WithIndex(&openchoreov1alpha1.Resource{}, resourceTypeRefIndex, indexResourceTypeRef).
			Build()
	}

	mapperCtx := context.Background()

	It("listResourcesForResourceType enqueues only Resources in the same namespace with matching kind+name", func() {
		matching := newRes("res-match", "ns1", openchoreov1alpha1.ResourceTypeRefKindResourceType, "mysql")
		differentName := newRes("res-other-name", "ns1", openchoreov1alpha1.ResourceTypeRefKindResourceType, "redis")
		differentNs := newRes("res-other-ns", "ns2", openchoreov1alpha1.ResourceTypeRefKindResourceType, "mysql")

		r := &Reconciler{Client: buildClient(matching, differentName, differentNs)}
		rt := &openchoreov1alpha1.ResourceType{
			ObjectMeta: metav1.ObjectMeta{Name: "mysql", Namespace: "ns1"},
		}

		requests := r.listResourcesForResourceType(mapperCtx, rt)

		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Namespace).To(Equal("ns1"))
		Expect(requests[0].Name).To(Equal("res-match"))
	})

	It("listResourcesForClusterResourceType enqueues matching Resources across namespaces", func() {
		ns1Match := newRes("res-ns1", "ns1", openchoreov1alpha1.ResourceTypeRefKindClusterResourceType, "shared-cache")
		ns2Match := newRes("res-ns2", "ns2", openchoreov1alpha1.ResourceTypeRefKindClusterResourceType, "shared-cache")
		differentName := newRes("res-other", "ns1", openchoreov1alpha1.ResourceTypeRefKindClusterResourceType, "other")

		r := &Reconciler{Client: buildClient(ns1Match, ns2Match, differentName)}
		crt := &openchoreov1alpha1.ClusterResourceType{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-cache"},
		}

		requests := r.listResourcesForClusterResourceType(mapperCtx, crt)

		Expect(requests).To(HaveLen(2))
		names := []string{requests[0].Name, requests[1].Name}
		Expect(names).To(ConsistOf("res-ns1", "res-ns2"))
	})

	It("kind discriminates: same name under ResourceType vs ClusterResourceType does not collide", func() {
		nsScoped := newRes("ns-scoped", "ns1", openchoreov1alpha1.ResourceTypeRefKindResourceType, "shared-name")
		clusterScoped := newRes("cluster-scoped", "ns1", openchoreov1alpha1.ResourceTypeRefKindClusterResourceType, "shared-name")

		r := &Reconciler{Client: buildClient(nsScoped, clusterScoped)}

		rt := &openchoreov1alpha1.ResourceType{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-name", Namespace: "ns1"},
		}
		requests := r.listResourcesForResourceType(mapperCtx, rt)
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Name).To(Equal("ns-scoped"))

		crt := &openchoreov1alpha1.ClusterResourceType{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-name"},
		}
		requests = r.listResourcesForClusterResourceType(mapperCtx, crt)
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Name).To(Equal("cluster-scoped"))
	})
})
