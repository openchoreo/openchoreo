// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func newComponent(project, name string) *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: project},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/service"},
		},
	}
}

var _ = Describe("Endpoint ownership", func() {
	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	})

	reconcileOwners := func(ep *openchoreov1alpha1.Endpoint) []metav1.OwnerReference {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ep)})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.Endpoint{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), updated)).To(Succeed())
		return updated.OwnerReferences
	}

	expectSoleOwner := func(refs []metav1.OwnerReference, kind string, owner client.Object) {
		Expect(refs).To(HaveLen(1))
		Expect(refs[0].Kind).To(Equal(kind))
		Expect(refs[0].Name).To(Equal(owner.GetName()))
		Expect(refs[0].UID).To(Equal(owner.GetUID()))
		Expect(refs[0].Controller).To(BeNil(), "the owner reference must not claim controller ownership")
	}

	It("makes the source component the owner of an Endpoint routing only to it", func() {
		project, comp := newProject("p-own-comp"), newComponent("p-own-comp", "greeter")
		ep := newEndpoint("ep-own-comp", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", comp.Name, "http", 1))
		ep.Labels = map[string]string{labels.LabelKeySourceComponent: comp.Name}
		create(project, comp, ep)

		expectSoleOwner(reconcileOwners(ep), "Component", comp)
	})

	It("moves ownership to the project once the Endpoint routes to another component", func() {
		project := newProject("p-own-move")
		comp, other := newComponent(project.Name, "greeter"), newComponent(project.Name, "greeter-v2")
		ep := newEndpoint("ep-own-move", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", comp.Name, "http", 1))
		ep.Labels = map[string]string{labels.LabelKeySourceComponent: comp.Name}
		create(project, comp, other, ep)

		expectSoleOwner(reconcileOwners(ep), "Component", comp)

		current := &openchoreov1alpha1.Endpoint{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), current)).To(Succeed())
		current.Spec.BackendRefs = append(current.Spec.BackendRefs,
			componentBackend("canary", other.Name, "http", 1))
		Expect(k8sClient.Update(ctx, current)).To(Succeed())

		expectSoleOwner(reconcileOwners(ep), "Project", project)
	})

	It("gives an Endpoint without a source component to its project", func() {
		project := newProject("p-own-proj")
		ep := newEndpoint("ep-own-proj", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", "greeter", "http", 1))
		create(project, ep)

		expectSoleOwner(reconcileOwners(ep), "Project", project)
	})

	It("keeps unrelated owner references and falls back to the project until the component exists", func() {
		project := newProject("p-own-late")
		ep := newEndpoint("ep-own-late", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", "greeter", "http", 1))
		ep.Labels = map[string]string{labels.LabelKeySourceComponent: "greeter"}
		foreign := metav1.OwnerReference{APIVersion: "example.com/v1", Kind: "Thing", Name: "t", UID: "foreign-uid"}
		ep.OwnerReferences = []metav1.OwnerReference{foreign}
		create(project, ep)

		refs := reconcileOwners(ep)
		Expect(refs).To(HaveLen(2))
		Expect(refs[0]).To(Equal(foreign))
		Expect(refs[1].Kind).To(Equal("Project"))

		comp := newComponent(project.Name, "greeter")
		create(comp)
		refs = reconcileOwners(ep)
		Expect(refs).To(HaveLen(2))
		Expect(refs[0]).To(Equal(foreign))
		Expect(refs[1].Kind).To(Equal("Component"))
		Expect(refs[1].UID).To(Equal(comp.UID))
	})

	It("leaves a component-owned Endpoint to garbage collection once the component is gone", func() {
		project, comp := newProject("p-own-gone"), newComponent("p-own-gone", "greeter")
		ep := newEndpoint("ep-own-gone", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", comp.Name, "http", 1))
		ep.Labels = map[string]string{labels.LabelKeySourceComponent: comp.Name}
		create(project, comp, ep)
		expectSoleOwner(reconcileOwners(ep), "Component", comp)

		Expect(k8sClient.Delete(ctx, comp)).To(Succeed())
		Eventually(func() bool {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(comp), &openchoreov1alpha1.Component{}) != nil
		}).Should(BeTrue())

		expectSoleOwner(reconcileOwners(ep), "Component", comp)
	})
})
