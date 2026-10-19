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
		mainLine     = "rl-comp-main"
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
		ct := minimalCT(ctName, "deployment")
		ct.Spec.ReleaseLines = openchoreov1alpha1.ReleaseLinePolicyAllowed
		create(ct, minimalProject(project, pipelineName), minimalPipeline(pipelineName))
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

	Context("When the component has release lines enabled", func() {
		var v2 *openchoreov1alpha1.ReleaseLine

		BeforeEach(func() {
			comp.Spec.ReleaseLinesEnabled = true
			v2 = newLine(v2Line)
			v2.Spec.Parameters = &runtime.RawExtension{Raw: []byte(`{"resources":{"memory":"256Mi"}}`)}
			create(
				newLine(mainLine), v2,
				newLineWorkload("rl-wl-main", mainLine, "nginx:1"),
				newLineWorkload("rl-wl-v2", v2Line, "nginx:2"),
				comp,
			)
		})

		It("leaves deployment to the ReleaseLine controller and has no implicit line", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonReleaseLinesReconciled)

			Expect(fetchComp(ctx, compName).Status.LatestRelease).To(BeNil())
			bindings := &openchoreov1alpha1.ReleaseBindingList{}
			Expect(k8sClient.List(ctx, bindings, client.InNamespace(itNamespace),
				client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName})).To(Succeed())
			Expect(bindings.Items).To(BeEmpty())
		})

		It("rejects a workload that does not name a line", func() {
			create(minimalWorkload("rl-wl-unlined", project, compName, "nginx:latest"))

			reconcileUntilCondition(ctx, r, compName, ReasonInvalidConfiguration)
			Expect(conditionFor(fetchComp(ctx, compName)).Message).To(ContainSubstring("rl-wl-unlined"))
		})

		It("is held back while its component type does not allow release lines", func() {
			setPolicy := func(policy openchoreov1alpha1.ReleaseLinePolicy) {
				Eventually(func(g Gomega) {
					ct := &openchoreov1alpha1.ComponentType{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ctName, Namespace: itNamespace}, ct)).To(Succeed())
					ct.Spec.ReleaseLines = policy
					g.Expect(k8sClient.Update(ctx, ct)).To(Succeed())
				}, itTimeout, itInterval).Should(Succeed())
			}

			setPolicy("")
			reconcileUntilCondition(ctx, r, compName, ReasonReleaseLinesNotAllowed)
			Expect(conditionFor(fetchComp(ctx, compName)).Message).To(ContainSubstring(ctName))

			setPolicy(openchoreov1alpha1.ReleaseLinePolicyAllowed)
			reconcileUntilCondition(ctx, r, compName, ReasonReleaseLinesReconciled)
		})

		It("cannot be switched off after creation", func() {
			Eventually(func(g Gomega) {
				c := fetchComp(ctx, compName)
				c.Spec.ReleaseLinesEnabled = false
				err := k8sClient.Update(ctx, c)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("releaseLinesEnabled can only be set when the component is created"))
			}, itTimeout, itInterval).Should(Succeed())
		})
	})

	Context("When the component does not have release lines enabled", func() {
		BeforeEach(func() {
			create(minimalWorkload("rl-wl", project, compName, "nginx:1"), newLine(v2Line), comp)
		})

		It("keeps deploying without lines", func() {
			reconcileUntilCondition(ctx, r, compName, ReasonComponentReleaseReady)

			bindings := &openchoreov1alpha1.ReleaseBindingList{}
			Expect(k8sClient.List(ctx, bindings, client.InNamespace(itNamespace),
				client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName})).To(Succeed())
			Expect(bindings.Items).To(HaveLen(1))
			Expect(bindings.Items[0].Spec.Owner.ReleaseLine).To(BeEmpty())
			Expect(bindings.Items[0].Name).To(Equal(compName + "-dev"))
		})

		It("cannot be switched on after creation", func() {
			Eventually(func(g Gomega) {
				c := fetchComp(ctx, compName)
				c.Spec.ReleaseLinesEnabled = true
				err := k8sClient.Update(ctx, c)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("releaseLinesEnabled can only be set when the component is created"))
			}, itTimeout, itInterval).Should(Succeed())
		})
	})
})
