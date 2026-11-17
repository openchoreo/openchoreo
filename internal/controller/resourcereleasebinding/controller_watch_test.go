// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

var _ = Describe("ResourceRelease watch mapper", func() {
	mapperCtx := context.Background()

	newBinding := func(name, ns, releaseName string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "p",
					ResourceName: "r",
				},
				Environment:     "dev",
				ResourceRelease: releaseName,
			},
		}
	}

	buildClient := func(objs ...client.Object) client.Client {
		s := runtime.NewScheme()
		Expect(openchoreov1alpha1.AddToScheme(s)).To(Succeed())
		return fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
				resourceReleaseRefIndex, indexResourceReleaseRef).
			Build()
	}

	It("enqueues only bindings in the same namespace whose spec.resourceRelease matches", func() {
		matching := newBinding("matches", "ns1", "r-abc123")
		differentRelease := newBinding("other-release", "ns1", "r-def456")
		differentNs := newBinding("other-ns", "ns2", "r-abc123")
		unsetPin := newBinding("unset-pin", "ns1", "")

		r := &Reconciler{Client: buildClient(matching, differentRelease, differentNs, unsetPin)}
		release := &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r-abc123", Namespace: "ns1"},
		}

		requests := r.listResourceReleaseBindingsForResourceRelease(mapperCtx, release)

		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Namespace).To(Equal("ns1"))
		Expect(requests[0].Name).To(Equal("matches"))
	})

	It("returns no requests when no binding references the release", func() {
		r := &Reconciler{Client: buildClient(newBinding("a", "ns1", "r-other"))}
		release := &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r-abc123", Namespace: "ns1"},
		}

		Expect(r.listResourceReleaseBindingsForResourceRelease(mapperCtx, release)).To(BeEmpty())
	})
})

var _ = Describe("ProjectReleaseBinding watch mapper", func() {
	It("enqueues the bindings of the ProjectReleaseBinding's project and environment", func() {
		binding := func(name, ns, project, env string) *openchoreov1alpha1.ResourceReleaseBinding {
			return &openchoreov1alpha1.ResourceReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
					Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: project, ResourceName: name},
					Environment: env,
				},
			}
		}
		s := runtime.NewScheme()
		Expect(openchoreov1alpha1.AddToScheme(s)).To(Succeed())
		r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(s).WithObjects(
			binding("cache-prod", "ns1", "payments", "prod"),
			binding("db-prod", "ns1", "payments", "prod"),
			binding("cache-dev", "ns1", "payments", "dev"),
			binding("orders-prod", "ns1", "orders", "prod"),
			binding("other-ns-prod", "ns2", "payments", "prod"),
		).Build()}
		prb := &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "payments-prod", Namespace: "ns1"},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: "payments"},
				Environment: "prod",
			},
		}

		requests := r.listResourceReleaseBindingsForProjectReleaseBinding(context.Background(), prb)

		names := make([]string, 0, len(requests))
		for _, req := range requests {
			names = append(names, req.Name)
		}
		Expect(names).To(ConsistOf("cache-prod", "db-prod"))
	})
})

var _ = Describe("Resource watch mapper", func() {
	It("enqueues the bindings of the Resource in its namespace", func() {
		binding := func(name, ns, resource string) *openchoreov1alpha1.ResourceReleaseBinding {
			return &openchoreov1alpha1.ResourceReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
					Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "p", ResourceName: resource},
					Environment: name,
					Mode:        openchoreov1alpha1.BindingModeObserve,
				},
			}
		}
		s := runtime.NewScheme()
		Expect(openchoreov1alpha1.AddToScheme(s)).To(Succeed())
		r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(s).
			WithObjects(
				binding("cache-dev", "ns1", "cache"),
				binding("cache-prod", "ns1", "cache"),
				binding("db-dev", "ns1", "db"),
				binding("cache-other-ns", "ns2", "cache"),
			).
			WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
				controller.IndexKeyResourceReleaseBindingOwnerResourceName, controller.IndexResourceReleaseBindingOwner).
			Build()}
		resource := &openchoreov1alpha1.Resource{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns1"}}

		requests := r.listResourceReleaseBindingsForResource(context.Background(), resource)

		names := make([]string, 0, len(requests))
		for _, req := range requests {
			names = append(names, req.Name)
		}
		Expect(names).To(ConsistOf("cache-dev", "cache-prod"))
	})

	It("passes creates and deletes, not updates", func() {
		p := createOrDeletePredicate()
		resource := &openchoreov1alpha1.Resource{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns1"}}
		Expect(p.Create(event.CreateEvent{Object: resource})).To(BeTrue())
		Expect(p.Delete(event.DeleteEvent{Object: resource})).To(BeTrue())
		Expect(p.Update(event.UpdateEvent{ObjectOld: resource, ObjectNew: resource})).To(BeFalse())
	})
})
