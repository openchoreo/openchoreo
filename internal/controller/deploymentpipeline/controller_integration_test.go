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

// forceDeleteProject deletes a Project, ignoring not-found errors.
func forceDeleteProject(nn types.NamespacedName) {
	project := &openchoreov1alpha1.Project{}
	if err := k8sClient.Get(ctx, nn, project); err != nil {
		return
	}
	_ = k8sClient.Delete(ctx, project)
	Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.Project{}))
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

	// -------------------------------------------------------------------------
	// Finalization: auto-clears refs from referencing Projects
	// -------------------------------------------------------------------------
	Context("When deleting a DeploymentPipeline that is referenced by Projects", func() {
		const pipelineName = "dp-finalize-with-ref"
		const projectName = "proj-refs-dp"
		pipelineNN := types.NamespacedName{Name: pipelineName, Namespace: ns}
		projectNN := types.NamespacedName{Name: projectName, Namespace: ns}

		BeforeEach(func() {
			pipeline := &openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:       pipelineName,
					Namespace:  ns,
					Finalizers: []string{PipelineCleanupFinalizer},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projectName,
					Namespace: ns,
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: &openchoreov1alpha1.DeploymentPipelineRef{Name: pipelineName},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			// Wait for the Project to appear in the cache (field index requires cache)
			Eventually(func() error {
				return k8sClient.Get(ctx, projectNN, &openchoreov1alpha1.Project{})
			}, "5s", "100ms").Should(Succeed())
		})

		AfterEach(func() {
			forceDeleteProject(projectNN)
			forceDeletePipeline(pipelineNN)
		})

		It("should clear deploymentPipelineRef from the project and delete the pipeline", func() {
			pipeline := &openchoreov1alpha1.DeploymentPipeline{}
			Expect(k8sClient.Get(ctx, pipelineNN, pipeline)).To(Succeed())
			Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: pipelineNN})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the project's deploymentPipelineRef is cleared")
			project := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, projectNN, project)).To(Succeed())
			Expect(project.Spec.DeploymentPipelineRef).To(BeNil())

			By("verifying the pipeline is deleted")
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, pipelineNN, &openchoreov1alpha1.DeploymentPipeline{}))
			}, "5s", "100ms").Should(BeTrue())
		})
	})

	// -------------------------------------------------------------------------
	// Finalization: auto-clears refs from multiple referencing Projects
	// -------------------------------------------------------------------------
	Context("When deleting a DeploymentPipeline referenced by multiple Projects", func() {
		const pipelineName = "dp-finalize-multi-ref"
		const project1Name = "proj1-refs-dp"
		const project2Name = "proj2-refs-dp"
		pipelineNN := types.NamespacedName{Name: pipelineName, Namespace: ns}
		project1NN := types.NamespacedName{Name: project1Name, Namespace: ns}
		project2NN := types.NamespacedName{Name: project2Name, Namespace: ns}

		BeforeEach(func() {
			pipeline := &openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:       pipelineName,
					Namespace:  ns,
					Finalizers: []string{PipelineCleanupFinalizer},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			for _, name := range []string{project1Name, project2Name} {
				project := &openchoreov1alpha1.Project{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: ns,
					},
					Spec: openchoreov1alpha1.ProjectSpec{
						DeploymentPipelineRef: &openchoreov1alpha1.DeploymentPipelineRef{Name: pipelineName},
					},
				}
				Expect(k8sClient.Create(ctx, project)).To(Succeed())
			}

			// Wait for the Projects to appear in the cache
			Eventually(func() error {
				if err := k8sClient.Get(ctx, project1NN, &openchoreov1alpha1.Project{}); err != nil {
					return err
				}
				return k8sClient.Get(ctx, project2NN, &openchoreov1alpha1.Project{})
			}, "5s", "100ms").Should(Succeed())
		})

		AfterEach(func() {
			forceDeleteProject(project1NN)
			forceDeleteProject(project2NN)
			forceDeletePipeline(pipelineNN)
		})

		It("should clear refs from all projects and delete the pipeline", func() {
			pipeline := &openchoreov1alpha1.DeploymentPipeline{}
			Expect(k8sClient.Get(ctx, pipelineNN, pipeline)).To(Succeed())
			Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: pipelineNN})
			Expect(err).NotTo(HaveOccurred())

			By("verifying both projects have nil deploymentPipelineRef")
			for _, nn := range []types.NamespacedName{project1NN, project2NN} {
				project := &openchoreov1alpha1.Project{}
				Expect(k8sClient.Get(ctx, nn, project)).To(Succeed())
				Expect(project.Spec.DeploymentPipelineRef).To(BeNil(), "project %s should have nil ref", nn.Name)
			}

			By("verifying the pipeline is deleted")
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, pipelineNN, &openchoreov1alpha1.DeploymentPipeline{}))
			}, "5s", "100ms").Should(BeTrue())
		})
	})
})
