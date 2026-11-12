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

// testLineNext is the release line most specs give an Endpoint.
const testLineNext = "next"

func newReleaseLine(project, component, line string) *openchoreov1alpha1.ReleaseLine {
	return &openchoreov1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{Name: line, Namespace: "default"},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: project, ComponentName: component},
		},
	}
}

// lineBackend is a componentBackend to the "http" endpoint of one release line of the component.
func lineBackend(name, component, line string) openchoreov1alpha1.EndpointBackendRef {
	ref := componentBackend(name, component, "http", 1)
	ref.Component.ReleaseLine = line
	return ref
}

// ownedByComponent names comp as the Endpoint's owner in its spec.
func ownedByComponent(ep *openchoreov1alpha1.Endpoint, comp string) *openchoreov1alpha1.Endpoint {
	ep.Spec.Owner.ComponentName = comp
	return ep
}

// ownedByLine names a release line of comp as the Endpoint's owner in its spec.
func ownedByLine(ep *openchoreov1alpha1.Endpoint, comp, line string) *openchoreov1alpha1.Endpoint {
	ep.Spec.Owner.ComponentName = comp
	ep.Spec.Owner.ReleaseLineName = line
	return ep
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

	It("makes the component named in spec.owner the owner", func() {
		project, comp := newProject("p-own-comp"), newComponent("p-own-comp", "greeter")
		ep := ownedByComponent(newEndpoint("ep-own-comp", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", comp.Name, "http", 1)), comp.Name)
		create(project, comp, ep)

		expectSoleOwner(reconcileOwners(ep), "Component", comp)
	})

	It("gives an Endpoint without an owning component to its project", func() {
		project := newProject("p-own-proj")
		ep := newEndpoint("ep-own-proj", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", "greeter", "http", 1))
		create(project, ep)

		expectSoleOwner(reconcileOwners(ep), "Project", project)
	})

	It("moves ownership to the project when spec.owner drops the component", func() {
		project := newProject("p-own-move")
		comp, other := newComponent(project.Name, "greeter"), newComponent(project.Name, "greeter-v2")
		ep := ownedByComponent(newEndpoint("ep-own-move", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", comp.Name, "http", 1)), comp.Name)
		create(project, comp, other, ep)
		expectSoleOwner(reconcileOwners(ep), "Component", comp)

		current := &openchoreov1alpha1.Endpoint{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), current)).To(Succeed())
		current.Spec.Owner.ComponentName = ""
		current.Spec.BackendRefs = append(current.Spec.BackendRefs,
			componentBackend("canary", other.Name, "http", 1))
		Expect(k8sClient.Update(ctx, current)).To(Succeed())

		expectSoleOwner(reconcileOwners(ep), "Project", project)
	})

	It("keeps unrelated owner references and falls back to the project until the component exists", func() {
		project := newProject("p-own-late")
		ep := ownedByComponent(newEndpoint("ep-own-late", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", "greeter", "http", 1)), "greeter")
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

	It("falls back to the project when the owning component belongs to another project", func() {
		project := newProject("p-own-foreign")
		comp := newComponent("some-other-project", "greeter-foreign")
		ep := ownedByComponent(newEndpoint("ep-own-foreign", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", comp.Name, "http", 1)), comp.Name)
		create(project, comp, ep)

		expectSoleOwner(reconcileOwners(ep), "Project", project)
	})

	It("leaves a component-owned Endpoint to garbage collection once the component is gone", func() {
		project, comp := newProject("p-own-gone"), newComponent("p-own-gone", "greeter")
		ep := ownedByComponent(newEndpoint("ep-own-gone", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", comp.Name, "http", 1)), comp.Name)
		create(project, comp, ep)
		expectSoleOwner(reconcileOwners(ep), "Component", comp)

		Expect(k8sClient.Delete(ctx, comp)).To(Succeed())
		Eventually(func() bool {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(comp), &openchoreov1alpha1.Component{}) != nil
		}).Should(BeTrue())

		expectSoleOwner(reconcileOwners(ep), "Component", comp)
	})

	Describe("release lines", func() {
		It("makes the release line named in spec.owner the owner", func() {
			project, comp := newProject("p-own-line"), newComponent("p-own-line", "greeter")
			rl := newReleaseLine(project.Name, comp.Name, testLineNext)
			ep := ownedByLine(newEndpoint("ep-own-line", project.Name, clusterTypeRef("cet-own"),
				lineBackend("greeter", comp.Name, rl.Name)), comp.Name, rl.Name)
			create(project, comp, rl, ep)

			expectSoleOwner(reconcileOwners(ep), "ReleaseLine", rl)
		})

		It("falls back to the component until the line exists, then moves to the line", func() {
			project, comp := newProject("p-own-line-late"), newComponent("p-own-line-late", "greeter")
			ep := ownedByLine(newEndpoint("ep-own-line-late", project.Name, clusterTypeRef("cet-own"),
				lineBackend("greeter", comp.Name, "canary")), comp.Name, "canary")
			create(project, comp, ep)
			expectSoleOwner(reconcileOwners(ep), "Component", comp)

			rl := newReleaseLine(project.Name, comp.Name, "canary")
			create(rl)
			expectSoleOwner(reconcileOwners(ep), "ReleaseLine", rl)
		})

		It("falls back to the component when the line belongs to another component", func() {
			project, comp := newProject("p-own-line-foreign"), newComponent("p-own-line-foreign", "greeter")
			rl := newReleaseLine(project.Name, "billing", "beta")
			ep := ownedByLine(newEndpoint("ep-own-line-foreign", project.Name, clusterTypeRef("cet-own"),
				lineBackend("greeter", comp.Name, rl.Name)), comp.Name, rl.Name)
			create(project, comp, rl, ep)

			expectSoleOwner(reconcileOwners(ep), "Component", comp)
		})

		It("moves ownership to the component when spec.owner drops the line", func() {
			project, comp := newProject("p-own-line-move"), newComponent("p-own-line-move", "greeter")
			rl := newReleaseLine(project.Name, comp.Name, "preview")
			ep := ownedByLine(newEndpoint("ep-own-line-move", project.Name, clusterTypeRef("cet-own"),
				lineBackend("greeter", comp.Name, rl.Name)), comp.Name, rl.Name)
			create(project, comp, rl, ep)
			expectSoleOwner(reconcileOwners(ep), "ReleaseLine", rl)

			current := &openchoreov1alpha1.Endpoint{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), current)).To(Succeed())
			current.Spec.Owner.ReleaseLineName = ""
			current.Spec.BackendRefs = append(current.Spec.BackendRefs, componentBackend("main", comp.Name, "http", 1))
			Expect(k8sClient.Update(ctx, current)).To(Succeed())

			expectSoleOwner(reconcileOwners(ep), "Component", comp)
		})

		It("leaves a line-owned Endpoint to garbage collection once the line is gone", func() {
			project, comp := newProject("p-own-line-gone"), newComponent("p-own-line-gone", "greeter")
			rl := newReleaseLine(project.Name, comp.Name, "old")
			ep := ownedByLine(newEndpoint("ep-own-line-gone", project.Name, clusterTypeRef("cet-own"),
				lineBackend("greeter", comp.Name, rl.Name)), comp.Name, rl.Name)
			create(project, comp, rl, ep)
			expectSoleOwner(reconcileOwners(ep), "ReleaseLine", rl)

			Expect(k8sClient.Delete(ctx, rl)).To(Succeed())
			Eventually(func() bool {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rl), &openchoreov1alpha1.ReleaseLine{}) != nil
			}).Should(BeTrue())

			expectSoleOwner(reconcileOwners(ep), "ReleaseLine", rl)
		})

		It("does not hand an Endpoint to a line recreated under the same name before garbage collection", func() {
			project, comp := newProject("p-own-line-again"), newComponent("p-own-line-again", "greeter")
			rl := newReleaseLine(project.Name, comp.Name, "again")
			ep := ownedByLine(newEndpoint("ep-own-line-again", project.Name, clusterTypeRef("cet-own"),
				lineBackend("greeter", comp.Name, rl.Name)), comp.Name, rl.Name)
			create(project, comp, rl, ep)
			expectSoleOwner(reconcileOwners(ep), "ReleaseLine", rl)

			Expect(k8sClient.Delete(ctx, rl)).To(Succeed())
			Eventually(func() bool {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rl), &openchoreov1alpha1.ReleaseLine{}) != nil
			}).Should(BeTrue())
			successor := newReleaseLine(project.Name, comp.Name, rl.Name)
			create(successor)
			Expect(successor.UID).NotTo(Equal(rl.UID))

			expectSoleOwner(reconcileOwners(ep), "ReleaseLine", rl)
		})
	})

	It("does not hand an Endpoint to a component recreated under the same name before garbage collection", func() {
		project, comp := newProject("p-own-comp-again"), newComponent("p-own-comp-again", "greeter")
		ep := ownedByComponent(newEndpoint("ep-own-comp-again", project.Name, clusterTypeRef("cet-own"),
			componentBackend("greeter", comp.Name, "http", 1)), comp.Name)
		create(project, comp, ep)
		expectSoleOwner(reconcileOwners(ep), "Component", comp)

		Expect(k8sClient.Delete(ctx, comp)).To(Succeed())
		Eventually(func() bool {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(comp), &openchoreov1alpha1.Component{}) != nil
		}).Should(BeTrue())
		successor := newComponent(project.Name, comp.Name)
		create(successor)
		Expect(successor.UID).NotTo(Equal(comp.UID))

		expectSoleOwner(reconcileOwners(ep), "Component", comp)
	})

	Describe("admission", func() {
		It("rejects an owning release line without an owning component", func() {
			ep := newEndpoint("ep-own-cel-nocomp", "p-own-cel", clusterTypeRef("cet-own"),
				lineBackend("greeter", "greeter", testLineNext))
			ep.Spec.Owner.ReleaseLineName = testLineNext

			err := k8sClient.Create(ctx, ep)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.owner.componentName is required when releaseLineName is set"))
		})

		It("rejects main as an owning release line", func() {
			ep := ownedByLine(newEndpoint("ep-own-cel-main", "p-own-cel", clusterTypeRef("cet-own"),
				componentBackend("greeter", "greeter", "http", 1)), "greeter", "main")

			err := k8sClient.Create(ctx, ep)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("main is the implicit line; omit releaseLineName for it"))
		})

		It("rejects an owning release line that is not every backend's line", func() {
			ep := ownedByLine(newEndpoint("ep-own-cel-lines", "p-own-cel", clusterTypeRef("cet-own"),
				lineBackend("greeter", "greeter", testLineNext),
				componentBackend("main", "greeter", "http", 1)), "greeter", testLineNext)

			err := k8sClient.Create(ctx, ep)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.owner.releaseLineName must be the release line every backend routes to"))
		})

		It("rejects an owning component that is not every backend's component", func() {
			ep := ownedByComponent(newEndpoint("ep-own-cel-mix", "p-own-cel", clusterTypeRef("cet-own"),
				componentBackend("greeter", "greeter", "http", 1),
				componentBackend("canary", "greeter-v2", "http", 1)), "greeter")

			err := k8sClient.Create(ctx, ep)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.owner.componentName must be the component every backend routes to"))
		})

		It("lets the owning component change but not the project", func() {
			ep := ownedByComponent(newEndpoint("ep-own-cel-mut", "p-own-cel", clusterTypeRef("cet-own"),
				componentBackend("greeter", "greeter", "http", 1)), "greeter")
			create(ep)

			current := &openchoreov1alpha1.Endpoint{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), current)).To(Succeed())
			current.Spec.Owner.ComponentName = ""
			Expect(k8sClient.Update(ctx, current)).To(Succeed(), "dropping the component is allowed")

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), current)).To(Succeed())
			current.Spec.Owner.ProjectName = "p-own-cel-other"
			err := k8sClient.Update(ctx, current)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.owner.projectName is immutable"))
		})
	})
})
