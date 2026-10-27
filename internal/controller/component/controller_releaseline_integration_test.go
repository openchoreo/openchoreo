// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

var _ = Describe("Component Controller — Release lines", func() {
	const (
		ctName       = "rl-ct"
		compName     = "rl-comp"
		project      = "rl-proj"
		pipelineName = "rl-pipe"
		v2Line       = "rl-comp-v2"
	)
	var (
		r       *Reconciler
		objects []client.Object
		comp    *openchoreov1alpha1.Component
	)

	newLine := func(name string) *openchoreov1alpha1.ReleaseLine {
		return &openchoreov1alpha1.ReleaseLine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: itNamespace},
			Spec: openchoreov1alpha1.ReleaseLineSpec{
				Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: project, ComponentName: compName},
			},
		}
	}
	newLineWorkload := func(name, line, image string) *openchoreov1alpha1.Workload {
		wl := minimalWorkload(name, project, compName, image)
		wl.Spec.Owner.ReleaseLine = line
		return wl
	}
	create := func(objs ...client.Object) {
		for _, obj := range objs {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			objects = append(objects, obj)
		}
	}

	BeforeEach(func() {
		r = itReconciler()
		objects = nil
		comp = minimalComp(compName, project, string(openchoreov1alpha1.ComponentTypeRefKindComponentType),
			"deployment/"+ctName, true)
		comp.Spec.Parameters = &runtime.RawExtension{Raw: []byte(`{"replicas":1,"resources":{"cpu":"100m","memory":"128Mi"}}`)}
		create(minimalCT(ctName, "deployment"), minimalProject(project, pipelineName), minimalPipeline(pipelineName))
	})

	AfterEach(func() {
		forceDeleteObj(ctx, comp)
		for _, list := range []client.ObjectList{&openchoreov1alpha1.ComponentReleaseList{}, &openchoreov1alpha1.ReleaseBindingList{}} {
			Expect(k8sClient.List(ctx, list, client.InNamespace(itNamespace))).To(Succeed())
			Expect(meta.EachListItem(list, func(o runtime.Object) error {
				_ = k8sClient.Delete(ctx, o.(client.Object))
				return nil
			})).To(Succeed())
		}
		for _, obj := range objects {
			_ = k8sClient.Delete(ctx, obj)
		}
	})

	Context("When the component has a release line next to its default line", func() {
		BeforeEach(func() {
			v2 := newLine(v2Line)
			v2.Spec.Parameters = &runtime.RawExtension{Raw: []byte(`{"resources":{"memory":"256Mi"}}`)}
			create(
				v2,
				minimalWorkload("rl-wl-default", project, compName, "nginx:1"),
				newLineWorkload("rl-wl-v2", v2Line, "nginx:2"),
				comp,
			)
		})

		It("deploys the default line from the workload without a line and leaves the release line alone", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonComponentReleaseReady)

			bindings := &openchoreov1alpha1.ReleaseBindingList{}
			Expect(k8sClient.List(ctx, bindings, client.InNamespace(itNamespace),
				client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName})).To(Succeed())
			Expect(bindings.Items).To(HaveLen(1))
			Expect(bindings.Items[0].Spec.Owner.ReleaseLine).To(BeEmpty())
			Expect(bindings.Items[0].Name).To(Equal(compName + "-dev"))

			release := &openchoreov1alpha1.ComponentRelease{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindings.Items[0].Spec.ReleaseName, Namespace: itNamespace}, release)).To(Succeed())
			Expect(release.Spec.Owner.ReleaseLine).To(BeEmpty())
			Expect(release.Spec.Workload.Container.Image).To(Equal("nginx:1"))
		})
	})

	Context("When only the release lines have workloads", func() {
		BeforeEach(func() {
			create(newLine(v2Line), newLineWorkload("rl-wl-v2", v2Line, "nginx:2"), comp)
		})

		It("is ready and deploys nothing for the default line", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonReleaseLinesReconciled)

			Expect(fetchComp(ctx, compName).Status.LatestRelease).To(BeNil())
			bindings := &openchoreov1alpha1.ReleaseBindingList{}
			Expect(k8sClient.List(ctx, bindings, client.InNamespace(itNamespace),
				client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName})).To(Succeed())
			Expect(bindings.Items).To(BeEmpty())
		})
	})

	It("rejects a ReleaseLine whose name is not a DNS label", func() {
		// Owner fields that name a line accept only DNS labels, so such a line could never get a Workload.
		err := k8sClient.Create(ctx, newLine("v1.2"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("must consist of lowercase letters, digits and '-'"))
	})

	It("rejects a ReleaseLine named default", func() {
		err := k8sClient.Create(ctx, newLine("default"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("default is the component's implicit line"))
	})

	It("rejects default as an owner release line", func() {
		wl := newLineWorkload("rl-wl-keyword", "default", "nginx:1")
		err := k8sClient.Create(ctx, wl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("omit releaseLine"))
	})

	It("keeps a Workload and a ReleaseBinding on the line they were created on", func() {
		moves := []struct{ name, from, to string }{
			{"to-line", "", v2Line},
			{"to-default", v2Line, ""},
			{"between-lines", v2Line, v2Line + "-next"},
		}
		for _, move := range moves {
			wl := newLineWorkload("rl-wl-"+move.name, move.from, "nginx:1")
			create(wl)
			wl.Spec.Owner.ReleaseLine = move.to
			err := k8sClient.Update(ctx, wl)
			Expect(err).To(HaveOccurred(), "workload moved %s", move.name)
			Expect(err.Error()).To(ContainSubstring("spec.owner is immutable"))

			rb := &openchoreov1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rl-rb-" + move.name, Namespace: itNamespace},
				Spec: openchoreov1alpha1.ReleaseBindingSpec{
					Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: project, ComponentName: compName, ReleaseLine: move.from},
					Environment: "development",
				},
			}
			create(rb)
			rb.Spec.Owner.ReleaseLine = move.to
			err = k8sClient.Update(ctx, rb)
			Expect(err).To(HaveOccurred(), "release binding moved %s", move.name)
			Expect(err.Error()).To(ContainSubstring("spec.owner is immutable"))
		}
	})
})
