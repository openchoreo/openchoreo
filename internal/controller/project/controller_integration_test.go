// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dp "github.com/openchoreo/openchoreo/internal/controller/dataplane"
	deppip "github.com/openchoreo/openchoreo/internal/controller/deploymentpipeline"
	env "github.com/openchoreo/openchoreo/internal/controller/environment"
	"github.com/openchoreo/openchoreo/internal/controller/testutils"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// ── test helpers ─────────────────────────────────────────────────────────────

const (
	itTimeout  = time.Second * 15
	itInterval = time.Millisecond * 250
)

func itReconciler() *Reconciler {
	return &Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
}

func forceDeleteProject(nn types.NamespacedName) {
	project := &openchoreov1alpha1.Project{}
	if err := k8sClient.Get(ctx, nn, project); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(project, ProjectCleanupFinalizer) {
		controllerutil.RemoveFinalizer(project, ProjectCleanupFinalizer)
		_ = k8sClient.Update(ctx, project)
	}
	_ = k8sClient.Delete(ctx, project)
}

// setupDependencies creates the namespace, dataplane, environment, and deployment pipeline
// needed by the project controller. Returns the namespace name for cleanup.
func setupDependencies(namespaceName, dpName, envName, deppipName string) {
	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
	}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, ns)
	if err != nil && errors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	}

	// Create and reconcile dataplane
	dataplane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: namespaceName,
		},
	}
	dpReconciler := &dp.Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
	testutils.CreateAndReconcileResource(ctx, k8sClient, dataplane,
		dpReconciler, types.NamespacedName{Name: dpName, Namespace: namespaceName})

	// Create and reconcile environment
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envName,
			Namespace: namespaceName,
			Labels: map[string]string{
				labels.LabelKeyNamespaceName: namespaceName,
				labels.LabelKeyName:          envName,
			},
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: "Test Environment",
				controller.AnnotationKeyDescription: "Test Environment Description",
			},
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: dpName,
			},
			IsProduction: false,
		},
	}
	envReconciler := &env.Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
	testutils.CreateAndReconcileResource(ctx, k8sClient, environment,
		envReconciler, types.NamespacedName{Name: envName, Namespace: namespaceName})

	// Create and reconcile deployment pipeline
	depPip := &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deppipName,
			Namespace: namespaceName,
			Labels: map[string]string{
				labels.LabelKeyNamespaceName: namespaceName,
				labels.LabelKeyName:          deppipName,
			},
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: "Test Deployment Pipeline",
				controller.AnnotationKeyDescription: "Test Deployment Pipeline Description",
			},
		},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: envName},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{},
				},
			},
		},
	}
	depPipReconciler := &deppip.Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
	testutils.CreateAndReconcileResource(ctx, k8sClient, depPip,
		depPipReconciler, types.NamespacedName{Name: deppipName, Namespace: namespaceName})
}

// ── Integration tests ────────────────────────────────────────────────────────

var _ = Describe("Project Controller", func() {

	Context("Reconcile non-existent resource", func() {
		It("should return no error for non-existent project", func() {
			r := itReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-project",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("First reconcile adds finalizer", func() {
		const (
			nsName   = "it-finalizer-ns"
			dpName   = "it-finalizer-dp"
			envName  = "it-finalizer-env"
			pipName  = "it-finalizer-pip"
			projName = "it-finalizer-proj"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
		})

		AfterEach(func() {
			forceDeleteProject(nn)
		})

		It("should add finalizer on first reconcile", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsName,
						labels.LabelKeyName:          projName,
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was added
			updated := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updated, ProjectCleanupFinalizer)).To(BeTrue())
		})
	})

	Context("Subsequent reconcile sets Created condition", func() {
		const (
			nsName   = "it-created-ns"
			dpName   = "it-created-dp"
			envName  = "it-created-env"
			pipName  = "it-created-pip"
			projName = "it-created-proj"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
		})

		AfterEach(func() {
			forceDeleteProject(nn)
		})

		It("should set Created condition after finalizer is added", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsName,
						labels.LabelKeyName:          projName,
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()

			// First reconcile: adds finalizer
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: sets Created condition
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify Created condition
			updated := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())

			cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonProjectCreated)))
			Expect(cond.Message).To(Equal("Project is created"))
		})

		It("should set ObservedGeneration on condition", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsName,
						labels.LabelKeyName:          projName,
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()

			// First reconcile: adds finalizer
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: sets status
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			updated := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())
			// The ObservedGeneration is tracked on the condition itself
			cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.ObservedGeneration).To(Equal(updated.Generation))
		})
	})

	Context("Idempotent reconcile", func() {
		const (
			nsName   = "it-idempotent-ns"
			dpName   = "it-idempotent-dp"
			envName  = "it-idempotent-env"
			pipName  = "it-idempotent-pip"
			projName = "it-idempotent-proj"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
		})

		AfterEach(func() {
			forceDeleteProject(nn)
		})

		It("should be idempotent on multiple reconciles", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsName,
						labels.LabelKeyName:          projName,
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()

			// First reconcile: adds finalizer
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: sets conditions
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Third reconcile: should be no-op
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify conditions are still correct
			updated := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updated, ProjectCleanupFinalizer)).To(BeTrue())
			cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Finalization with no owned resources", func() {
		const (
			nsName   = "it-finalize-ns"
			dpName   = "it-finalize-dp"
			envName  = "it-finalize-env"
			pipName  = "it-finalize-pip"
			projName = "it-finalize-proj"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
		})

		It("should finalize and delete project with no child resources", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsName,
						labels.LabelKeyName:          projName,
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()

			// First reconcile: adds finalizer
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: sets Created condition
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Delete the project
			updated := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())

			// Verify deletion timestamp is set
			Eventually(func() bool {
				p := &openchoreov1alpha1.Project{}
				if err := k8sClient.Get(ctx, nn, p); err != nil {
					return false
				}
				return !p.DeletionTimestamp.IsZero()
			}, itTimeout, itInterval).Should(BeTrue())

			// Reconcile to set Finalizing condition
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify Finalizing condition is set
			finalizingProject := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, finalizingProject)).To(Succeed())
			cond := meta.FindStatusCondition(finalizingProject.Status.Conditions, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			// Reconcile again to complete finalization (delete child resources + remove finalizer)
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify project is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.Project{})
				return errors.IsNotFound(err)
			}, itTimeout, itInterval).Should(BeTrue())
		})
	})

	Context("Finalization without finalizer", func() {
		It("should return no error when project has no finalizer and is being deleted", func() {
			const (
				nsName   = "it-nofinalizer-ns"
				projName = "it-nofinalizer-proj"
			)

			// Create namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: nsName},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			}

			nn := types.NamespacedName{Name: projName, Namespace: nsName}

			// Create project without finalizer and immediately delete it
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: "some-pipeline",
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())
			Expect(k8sClient.Delete(ctx, project)).To(Succeed())

			// Reconcile — should complete with no error since there's no finalizer
			r := itReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("Status persistence via status subresource", func() {
		const (
			nsName   = "it-status-ns"
			dpName   = "it-status-dp"
			envName  = "it-status-env"
			pipName  = "it-status-pip"
			projName = "it-status-proj"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
		})

		AfterEach(func() {
			forceDeleteProject(nn)
		})

		It("should persist status updates via status subresource", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsName,
						labels.LabelKeyName:          projName,
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()

			// First reconcile: adds finalizer
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: sets conditions
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and verify status is persisted
			fetched := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Conditions).NotTo(BeEmpty())

			cond := meta.FindStatusCondition(fetched.Status.Conditions, string(ConditionCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonProjectCreated)))
		})
	})

	Context("Full lifecycle: create, reconcile, delete", func() {
		const (
			nsName   = "it-lifecycle-ns"
			dpName   = "it-lifecycle-dp"
			envName  = "it-lifecycle-env"
			pipName  = "it-lifecycle-pip"
			projName = "it-lifecycle-proj"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
		})

		It("should handle full project lifecycle", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsName,
						labels.LabelKeyName:          projName,
					},
					Annotations: map[string]string{
						controller.AnnotationKeyDisplayName: "Lifecycle Test Project",
						controller.AnnotationKeyDescription: "A project for lifecycle testing",
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()

			By("First reconcile: adding finalizer")
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			fetched := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fetched, ProjectCleanupFinalizer)).To(BeTrue())

			By("Second reconcile: setting Created condition")
			result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Spec.DeploymentPipelineRef.Name).To(Equal(pipName))
			cond := meta.FindStatusCondition(fetched.Status.Conditions, string(ConditionCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			By("Deleting project")
			Expect(k8sClient.Delete(ctx, fetched)).To(Succeed())

			Eventually(func() bool {
				p := &openchoreov1alpha1.Project{}
				if err := k8sClient.Get(ctx, nn, p); err != nil {
					return false
				}
				return !p.DeletionTimestamp.IsZero()
			}, itTimeout, itInterval).Should(BeTrue())

			By("Reconcile after deletion: sets Finalizing condition")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile again: completes finalization")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying project is fully deleted")
			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.Project{}))
			}, itTimeout, itInterval).Should(BeTrue())
		})
	})

	Context("Project with pre-set finalizer skips finalizer-add reconcile", func() {
		const (
			nsName   = "it-preset-ns"
			dpName   = "it-preset-dp"
			envName  = "it-preset-env"
			pipName  = "it-preset-pip"
			projName = "it-preset-proj"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
		})

		AfterEach(func() {
			forceDeleteProject(nn)
		})

		It("should set Created condition on first reconcile if finalizer is pre-set", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsName,
						labels.LabelKeyName:          projName,
					},
					Finalizers: []string{ProjectCleanupFinalizer},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()

			// Single reconcile should set Created condition since finalizer already present
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			updated := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())
			cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
