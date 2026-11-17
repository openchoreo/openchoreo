// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/projectreleasebinding"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

var _ = Describe("ResourceReleaseBinding controller — namespace from the ProjectReleaseBinding", func() {
	// The suite creates Project test-project and Resource test-resource in
	// default. Each test uses its own environment, so the bindings' releases
	// don't collide.
	const project = "test-project"

	reconciler := func() *Reconciler {
		return &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Pipeline: resourcepipeline.NewPipeline()}
	}

	// makeBinding creates a pinned release that renders a ConfigMap into
	// metadata.namespace, an Environment and DataPlane named after env, and a
	// managed binding for test-resource there.
	makeBinding := func(env string) *openchoreov1alpha1.ResourceReleaseBinding {
		release := &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: env + "-release", Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseSpec{
				Owner: openchoreov1alpha1.ResourceReleaseOwner{ProjectName: project, ResourceName: "test-resource"},
				ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "cache",
					Spec: openchoreov1alpha1.ResourceTypeSpec{
						Resources: []openchoreov1alpha1.ResourceTypeManifest{{
							ID: "config",
							Template: &runtime.RawExtension{Raw: []byte(
								`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cache","namespace":"${metadata.namespace}"}}`)},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		dp := &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: env + "-dp", Namespace: "default"}}
		Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dp) })
		environment := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: env, Namespace: "default"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane, Name: dp.Name},
			},
		}
		Expect(k8sClient.Create(ctx, environment)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, environment) })

		b := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       env + "-binding",
				Namespace:  "default",
				Finalizers: []string{ResourceReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:           openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: project, ResourceName: "test-resource"},
				Environment:     env,
				ResourceRelease: release.Name,
			},
		}
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			rr := &openchoreov1alpha1.RenderedRelease{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: makeRenderedReleaseName(b), Namespace: "default"}, rr); err == nil {
				_ = k8sClient.Delete(ctx, rr)
			}
			current := &openchoreov1alpha1.ResourceReleaseBinding{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(b), current); err == nil {
				current.Finalizers = nil
				_ = k8sClient.Update(ctx, current)
				_ = k8sClient.Delete(ctx, current)
			}
		})
		return b
	}

	makePRB := func(name, env string, mode openchoreov1alpha1.BindingMode, reported string) {
		prb := &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: project},
				Environment: env,
				Mode:        mode,
			},
		}
		if mode == openchoreov1alpha1.BindingModeObserve {
			prb.Spec.Observe = &openchoreov1alpha1.ProjectReleaseBindingObserve{Namespace: "team-ns"}
		}
		Expect(k8sClient.Create(ctx, prb)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, prb) })
		if reported != "" {
			prb.Status.Namespace = reported
			Expect(k8sClient.Status().Update(ctx, prb)).To(Succeed())
		}
	}

	reconcileBinding := func(r *Reconciler, b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		GinkgoHelper()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return updated
	}

	releaseOf := func(b *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.RenderedRelease, error) {
		rr := &openchoreov1alpha1.RenderedRelease{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: makeRenderedReleaseName(b), Namespace: "default"}, rr)
		return rr, err
	}

	renderedNamespace := func(b *openchoreov1alpha1.ResourceReleaseBinding) string {
		GinkgoHelper()
		rr, err := releaseOf(b)
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.Spec.Resources).To(HaveLen(1))
		obj := map[string]any{}
		Expect(json.Unmarshal(rr.Spec.Resources[0].Object.Raw, &obj)).To(Succeed())
		return obj["metadata"].(map[string]any)["namespace"].(string)
	}

	expectHeld := func(b *openchoreov1alpha1.ResourceReleaseBinding, reason string) {
		GinkgoHelper()
		synced := meta.FindStatusCondition(b.Status.Conditions, string(ConditionSynced))
		Expect(synced).NotTo(BeNil())
		Expect(synced.Status).To(Equal(metav1.ConditionFalse))
		Expect(synced.Reason).To(Equal(reason))
		ready := meta.FindStatusCondition(b.Status.Conditions, string(ConditionReady))
		Expect(ready).NotTo(BeNil())
		Expect(ready.Reason).To(Equal(reason))
		_, err := releaseOf(b)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no release should be created")
	}

	It("renders into the generated dp-* namespace without a ProjectReleaseBinding", func() {
		b := makeBinding("rns-none")
		reconcileBinding(reconciler(), b)
		Expect(renderedNamespace(b)).To(Equal(dpkubernetes.DataPlaneNamespace("default", project, "rns-none")))
	})

	It("renders into the namespace a managed ProjectReleaseBinding reports", func() {
		makePRB("rns-managed", "rns-managed", openchoreov1alpha1.BindingModeManage, "team-runtime")
		b := makeBinding("rns-managed")
		reconcileBinding(reconciler(), b)
		Expect(renderedNamespace(b)).To(Equal("team-runtime"))
	})

	It("waits while a managed ProjectReleaseBinding has not reported its namespace", func() {
		makePRB("rns-pending", "rns-pending", openchoreov1alpha1.BindingModeManage, "")
		b := makeBinding("rns-pending")
		expectHeld(reconcileBinding(reconciler(), b), string(projectreleasebinding.ReasonProjectNamespacePending))
	})

	It("renders nothing under an observed ProjectReleaseBinding, pinned or not", func() {
		makePRB("rns-observed", "rns-observed", openchoreov1alpha1.BindingModeObserve, "team-ns")
		b := makeBinding("rns-observed")
		expectHeld(reconcileBinding(reconciler(), b), string(projectreleasebinding.ReasonProjectObserved))

		current := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), current)).To(Succeed())
		current.Spec.ResourceRelease = ""
		Expect(k8sClient.Update(ctx, current)).To(Succeed())
		expectHeld(reconcileBinding(reconciler(), b), string(projectreleasebinding.ReasonProjectObserved))
	})

	It("leaves an existing release alone under an observed ProjectReleaseBinding", func() {
		b := makeBinding("rns-existing")
		r := reconciler()
		reconcileBinding(r, b)
		before, err := releaseOf(b)
		Expect(err).NotTo(HaveOccurred())

		makePRB("rns-existing", "rns-existing", openchoreov1alpha1.BindingModeObserve, "team-ns")
		updated := reconcileBinding(r, b)

		after, err := releaseOf(b)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.Generation).To(Equal(before.Generation))
		Expect(after.DeletionTimestamp).To(BeNil())
		synced := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(synced.Reason).To(Equal(string(projectreleasebinding.ReasonProjectObserved)))
	})

	It("reports a conflict with several ProjectReleaseBindings", func() {
		makePRB("rns-several-a", "rns-several", openchoreov1alpha1.BindingModeManage, "team-runtime")
		makePRB("rns-several-b", "rns-several", openchoreov1alpha1.BindingModeManage, "team-runtime")
		b := makeBinding("rns-several")
		expectHeld(reconcileBinding(reconciler(), b), string(projectreleasebinding.ReasonProjectReleaseBindingConflict))
	})
})
