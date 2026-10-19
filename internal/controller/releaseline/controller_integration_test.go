// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/component"
)

const (
	ns       = "default"
	timeout  = 15 * time.Second
	interval = 250 * time.Millisecond
)

var _ = Describe("ReleaseLine Controller", func() {
	const (
		ctName       = "rl-ct"
		compName     = "rl-comp"
		project      = "rl-proj"
		pipelineName = "rl-pipe"
		mainLine     = "main"
		v2Line       = "v2"
	)
	var (
		r       *Reconciler
		objects []client.Object
		comp    *openchoreov1alpha1.Component
	)

	newLine := func(name string) *openchoreov1alpha1.ReleaseLine {
		return &openchoreov1alpha1.ReleaseLine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.ReleaseLineSpec{
				Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: project, ComponentName: compName},
			},
		}
	}
	newWorkload := func(name, line, image string) *openchoreov1alpha1.Workload {
		return &openchoreov1alpha1.Workload{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.WorkloadSpec{
				Owner: openchoreov1alpha1.WorkloadOwner{ProjectName: project, ComponentName: compName, ReleaseLine: line},
				WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
					Container: openchoreov1alpha1.Container{Image: image},
				},
			},
		}
	}
	fetchLine := func(g Gomega, name string) *openchoreov1alpha1.ReleaseLine {
		l := &openchoreov1alpha1.ReleaseLine{}
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, l)).To(Succeed())
		return l
	}
	create := func(objs ...client.Object) {
		for _, obj := range objs {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			objects = append(objects, obj)
		}
	}
	// reconcileUntil reconciles the line until its Ready condition has the reason.
	reconcileUntil := func(name string, reason controller.ConditionReason) *openchoreov1alpha1.ReleaseLine {
		GinkgoHelper()
		var line *openchoreov1alpha1.ReleaseLine
		Eventually(func(g Gomega) {
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}})
			g.Expect(err).NotTo(HaveOccurred())
			line = fetchLine(g, name)
			cond := meta.FindStatusCondition(line.Status.Conditions, string(ConditionReady))
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Reason).To(Equal(string(reason)), cond.Message)
		}, timeout, interval).Should(Succeed())
		return line
	}
	bindingsOf := func(g Gomega) []openchoreov1alpha1.ReleaseBinding {
		list := &openchoreov1alpha1.ReleaseBindingList{}
		g.Expect(k8sClient.List(ctx, list, client.InNamespace(ns),
			client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: compName})).To(Succeed())
		return list.Items
	}

	BeforeEach(func() {
		r = &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(100),
			steps:    component.NewSteps(k8sClient, k8sClient.Scheme(), record.NewFakeRecorder(100)),
		}
		objects = nil
		comp = &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: compName, Namespace: ns},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:               openchoreov1alpha1.ComponentOwner{ProjectName: project},
				ComponentType:       openchoreov1alpha1.ComponentTypeRef{Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType, Name: "deployment/" + ctName},
				AutoDeploy:          true,
				ReleaseLinesEnabled: true,
				Parameters:          &runtime.RawExtension{Raw: []byte(`{"replicas":1,"resources":{"cpu":"100m","memory":"128Mi"}}`)},
			},
		}
		create(
			&openchoreov1alpha1.ComponentType{
				ObjectMeta: metav1.ObjectMeta{Name: ctName, Namespace: ns},
				Spec: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					ReleaseLines: openchoreov1alpha1.ReleaseLinePolicyAllowed,
					Resources: []openchoreov1alpha1.ResourceTemplate{{
						ID:       "deployment",
						Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`)},
					}},
				},
			},
			&openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Name: project, Namespace: ns},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: pipelineName},
					Type:                  openchoreov1alpha1.ProjectTypeRef{Name: "default"},
				},
			},
			&openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: pipelineName, Namespace: ns},
				Spec: openchoreov1alpha1.DeploymentPipelineSpec{
					PromotionPaths: []openchoreov1alpha1.PromotionPath{{
						SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: "dev"},
						TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "staging"}},
					}},
				},
			},
		)
	})

	AfterEach(func() {
		for _, list := range []client.ObjectList{
			&openchoreov1alpha1.ReleaseLineList{}, &openchoreov1alpha1.ComponentReleaseList{},
			&openchoreov1alpha1.ReleaseBindingList{}, &openchoreov1alpha1.WorkloadList{},
		} {
			Expect(k8sClient.List(ctx, list, client.InNamespace(ns))).To(Succeed())
			Expect(meta.EachListItem(list, func(o runtime.Object) error {
				obj := o.(client.Object)
				obj.SetFinalizers(nil)
				_ = k8sClient.Update(ctx, obj)
				_ = k8sClient.Delete(ctx, obj)
				return nil
			})).To(Succeed())
		}
		_ = k8sClient.Delete(ctx, comp)
		for _, obj := range objects {
			_ = k8sClient.Delete(ctx, obj)
		}
	})

	Context("When the component has release lines enabled", func() {
		BeforeEach(func() {
			v2 := newLine(v2Line)
			v2.Spec.Parameters = &runtime.RawExtension{Raw: []byte(`{"resources":{"memory":"256Mi"}}`)}
			create(comp, newLine(mainLine), v2,
				newWorkload("rl-wl-main", mainLine, "nginx:1"),
				newWorkload("rl-wl-v2", v2Line, "nginx:2"))
		})

		It("releases and binds every line independently", func() {
			releases := map[string]string{}
			for _, name := range []string{mainLine, v2Line} {
				line := reconcileUntil(name, ReasonComponentReleaseReady)
				Expect(line.Finalizers).To(ContainElement(ReleaseLineFinalizer))
				Expect(line.Status.LatestRelease).NotTo(BeNil())
				Expect(line.Status.LatestRelease.Name).To(HavePrefix(compName + "-" + name + "-"))
				releases[name] = line.Status.LatestRelease.Name
			}

			By("Verifying the v2 release carries its line, its workload, and merged parameters")
			cr := &openchoreov1alpha1.ComponentRelease{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: releases[v2Line], Namespace: ns}, cr)).To(Succeed())
			Expect(cr.Spec.Owner.ReleaseLine).To(Equal(v2Line))
			Expect(cr.Spec.Workload.Container.Image).To(Equal("nginx:2"))
			var params map[string]any
			Expect(json.Unmarshal(cr.Spec.ComponentProfile.Parameters.Raw, &params)).To(Succeed())
			Expect(params["resources"]).To(Equal(map[string]any{"cpu": "100m", "memory": "256Mi"}))

			By("Verifying each line has its own binding in the first environment")
			Eventually(func(g Gomega) {
				byLine := map[string]string{}
				for _, b := range bindingsOf(g) {
					g.Expect(b.Spec.Environment).To(Equal("dev"))
					byLine[b.Spec.Owner.ReleaseLine] = b.Spec.ReleaseName
				}
				g.Expect(byLine).To(Equal(releases))
			}, timeout, interval).Should(Succeed())

			By("Verifying the line reports its environments")
			Eventually(func(g Gomega) {
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: v2Line, Namespace: ns}})
				g.Expect(err).NotTo(HaveOccurred())
				envs := fetchLine(g, v2Line).Status.Environments
				g.Expect(envs).To(HaveLen(1))
				g.Expect(envs[0].Environment).To(Equal("dev"))
				g.Expect(envs[0].ReleaseBinding).To(Equal(compName + "-" + v2Line + "-dev"))
				g.Expect(envs[0].Release).To(Equal(releases[v2Line]))
			}, timeout, interval).Should(Succeed())
		})

		It("honors a line's autoDeploy override", func() {
			Eventually(func(g Gomega) {
				l := fetchLine(g, v2Line)
				l.Spec.AutoDeploy = ptr.To(false)
				g.Expect(k8sClient.Update(ctx, l)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			line := reconcileUntil(v2Line, ReasonReconciled)
			Expect(line.Status.LatestRelease).To(BeNil())
			reconcileUntil(mainLine, ReasonComponentReleaseReady)
		})

		It("does not release a line while its component type does not allow release lines", func() {
			Eventually(func(g Gomega) {
				ct := &openchoreov1alpha1.ComponentType{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ctName, Namespace: ns}, ct)).To(Succeed())
				ct.Spec.ReleaseLines = ""
				g.Expect(k8sClient.Update(ctx, ct)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			line := reconcileUntil(mainLine, ReasonReleaseLinesNotAllowed)
			Expect(line.Status.LatestRelease).To(BeNil())
			Eventually(func(g Gomega) {
				g.Expect(bindingsOf(g)).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("waits for the line's workload", func() {
			create(newLine("v3"))
			reconcileUntil("v3", ReasonWorkloadNotFound)
		})

		It("removes the line's release, binding and workload when the line is deleted", func() {
			reconcileUntil(mainLine, ReasonComponentReleaseReady)
			reconcileUntil(v2Line, ReasonComponentReleaseReady)

			Expect(k8sClient.Delete(ctx, newLine(v2Line))).To(Succeed())
			Eventually(func(g Gomega) {
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: v2Line, Namespace: ns}})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, types.NamespacedName{Name: v2Line, Namespace: ns}, &openchoreov1alpha1.ReleaseLine{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				lines := map[string]bool{}
				for _, b := range bindingsOf(g) {
					lines[b.Spec.Owner.ReleaseLine] = true
				}
				g.Expect(lines).To(Equal(map[string]bool{mainLine: true}))

				wl := &openchoreov1alpha1.Workload{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "rl-wl-v2", Namespace: ns}, wl)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rl-wl-main", Namespace: ns}, wl)).To(Succeed())
			}, timeout, interval).Should(Succeed())
		})
	})

	It("flags a line whose component does not have release lines enabled", func() {
		comp.Spec.ReleaseLinesEnabled = false
		create(comp, newLine(v2Line))
		reconcileUntil(v2Line, ReasonReleaseLinesNotEnabled)
	})

	It("flags a line whose component does not exist", func() {
		create(newLine(v2Line))
		reconcileUntil(v2Line, ReasonComponentNotFound)
	})
})
