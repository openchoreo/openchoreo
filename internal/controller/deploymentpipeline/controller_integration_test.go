// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// testReconciler returns a Reconciler configured for tests.
func testReconciler() *Reconciler {
	return &Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
}

// forceDeletePipeline strips the cleanup finalizer and deletes the DeploymentPipeline.
func forceDeletePipeline(nn types.NamespacedName) {
	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := k8sClient.Get(ctx, nn, pipeline); err != nil {
		return
	}
	controllerutil.RemoveFinalizer(pipeline, PipelineCleanupFinalizer)
	_ = k8sClient.Update(ctx, pipeline)
	_ = k8sClient.Delete(ctx, pipeline)
	Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.DeploymentPipeline{}))
	}, "5s", "100ms").Should(BeTrue())
}

var _ = Describe("DeploymentPipeline Controller", func() {

	const ns = "default"

	// -------------------------------------------------------------------------
	// Reconcile: non-existent resource
	// -------------------------------------------------------------------------
	Context("When reconciling a non-existent DeploymentPipeline", func() {
		It("should return no error and no requeue", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: ns},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	// -------------------------------------------------------------------------
	// First reconcile: adds finalizer
	// -------------------------------------------------------------------------
	Context("When reconciling a newly created DeploymentPipeline", func() {
		const name = "dp-first-reconcile"
		nn := types.NamespacedName{Name: name, Namespace: ns}

		BeforeEach(func() {
			pipeline := &openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: ns,
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			forceDeletePipeline(nn)
		})

		It("should add finalizer and return empty result", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.DeploymentPipeline{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, PipelineCleanupFinalizer)).To(BeTrue())
		})
	})

	// -------------------------------------------------------------------------
	// Second reconcile: sets Available condition
	// -------------------------------------------------------------------------
	Context("When reconciling a DeploymentPipeline with finalizer already set", func() {
		const name = "dp-second-reconcile"
		nn := types.NamespacedName{Name: name, Namespace: ns}

		BeforeEach(func() {
			pipeline := &openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{PipelineCleanupFinalizer},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			forceDeletePipeline(nn)
		})

		It("should set Available condition and update ObservedGeneration", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.DeploymentPipeline{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())

			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, controller.TypeAvailable)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("DeploymentPipelineAvailable"))
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})
	})

	// -------------------------------------------------------------------------
	// Finalization: no referencing projects
	// -------------------------------------------------------------------------
	Context("When deleting a DeploymentPipeline with no referencing Projects", func() {
		const name = "dp-finalize-no-ref"
		nn := types.NamespacedName{Name: name, Namespace: ns}

		BeforeEach(func() {
			pipeline := &openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  ns,
					Finalizers: []string{PipelineCleanupFinalizer},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			forceDeletePipeline(nn)
		})

		It("should remove finalizer and delete the DeploymentPipeline", func() {
			pipeline := &openchoreov1alpha1.DeploymentPipeline{}
			Expect(k8sClient.Get(ctx, nn, pipeline)).To(Succeed())
			Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.DeploymentPipeline{}))
			}, "5s", "100ms").Should(BeTrue())
		})
	})

})
