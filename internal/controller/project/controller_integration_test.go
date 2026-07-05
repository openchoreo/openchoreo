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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dp "github.com/openchoreo/openchoreo/internal/controller/dataplane"
	deppip "github.com/openchoreo/openchoreo/internal/controller/deploymentpipeline"
	env "github.com/openchoreo/openchoreo/internal/controller/environment"
	"github.com/openchoreo/openchoreo/internal/controller/testutils"
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
			Labels:    map[string]string{},
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
			Labels:    map[string]string{},
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

// createProjectType creates a minimal valid ProjectType in the given namespace
// so that resolveType succeeds and the project controller cuts a ProjectRelease.
func createProjectType(namespaceName, ptName string) {
	pt := &openchoreov1alpha1.ProjectType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ptName,
			Namespace: namespaceName,
		},
		Spec: openchoreov1alpha1.ProjectTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTemplate{
				{
					ID: "namespace",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"${metadata.namespace}"}}`),
					},
				},
			},
		},
	}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: ptName, Namespace: namespaceName}, &openchoreov1alpha1.ProjectType{})
	if err != nil && errors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, pt)).To(Succeed())
	}
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
					Labels:    map[string]string{},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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
					Labels:    map[string]string{},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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
					Labels:    map[string]string{},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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
					Labels:    map[string]string{},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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
					Labels:    map[string]string{},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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
					Labels:    map[string]string{},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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
					Labels:    map[string]string{},
					Annotations: map[string]string{
						controller.AnnotationKeyDisplayName: "Lifecycle Test Project",
						controller.AnnotationKeyDescription: "A project for lifecycle testing",
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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

			By("Creating an owned Resource")
			resource := &openchoreov1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "it-lifecycle-res",
					Namespace: nsName,
				},
				Spec: openchoreov1alpha1.ResourceSpec{
					Owner: openchoreov1alpha1.ResourceOwner{
						ProjectName: projName,
					},
					Type: openchoreov1alpha1.ResourceTypeRef{
						Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
						Name: "some-type",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

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

			By("Verifying project is fully deleted")
			Eventually(func() bool {
				_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
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
					Name:       projName,
					Namespace:  nsName,
					Finalizers: []string{ProjectCleanupFinalizer},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
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

	Context("ProjectRelease pin seeding", func() {
		const (
			nsName   = "it-seed-ns"
			dpName   = "it-seed-dp"
			envName  = "it-seed-env"
			pipName  = "it-seed-pip"
			projName = "it-seed-proj"
			ptName   = "it-seed-pt"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		newProject := func() *openchoreov1alpha1.Project {
			return &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: ptName,
					},
				},
			}
		}

		newBinding := func(name, envName, releaseName string) *openchoreov1alpha1.ProjectReleaseBinding {
			return &openchoreov1alpha1.ProjectReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: nsName,
				},
				Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
					Owner: openchoreov1alpha1.ProjectReleaseBindingOwner{
						ProjectName: projName,
					},
					Environment:    envName,
					ProjectRelease: releaseName,
				},
			}
		}

		// waitForCacheVisibility blocks until the manager cache serves the
		// binding, so index-based lists inside the reconciler see it.
		waitForCacheVisibility := func(name string) {
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: nsName},
					&openchoreov1alpha1.ProjectReleaseBinding{})
			}, itTimeout, itInterval).Should(Succeed())
		}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
			createProjectType(nsName, ptName)
		})

		AfterEach(func() {
			forceDeleteProject(nn)
			Expect(k8sClient.DeleteAllOf(ctx, &openchoreov1alpha1.ProjectReleaseBinding{},
				client.InNamespace(nsName))).To(Succeed())
		})

		It("should seed empty pins and leave explicit pins untouched", func() {
			// Externally authored bindings created before the project exists:
			// one unpinned (to be seeded), one explicitly pinned (untouchable).
			unpinned := newBinding("custom-binding-dev", envName, "")
			Expect(k8sClient.Create(ctx, unpinned)).To(Succeed())
			pinned := newBinding("custom-binding-manual", "env-manual", "user-pinned-release")
			Expect(k8sClient.Create(ctx, pinned)).To(Succeed())
			waitForCacheVisibility("custom-binding-dev")
			waitForCacheVisibility("custom-binding-manual")

			Expect(k8sClient.Create(ctx, newProject())).To(Succeed())

			r := itReconciler()

			// First reconcile adds the finalizer; the second cuts the release
			// and seeds pins. Eventually tolerates cache lag on the list.
			var latestRelease string
			Eventually(func(g Gomega) {
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				g.Expect(err).NotTo(HaveOccurred())

				project := &openchoreov1alpha1.Project{}
				g.Expect(k8sClient.Get(ctx, nn, project)).To(Succeed())
				g.Expect(project.Status.LatestRelease).NotTo(BeNil())
				latestRelease = project.Status.LatestRelease.Name

				seeded := &openchoreov1alpha1.ProjectReleaseBinding{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "custom-binding-dev", Namespace: nsName}, seeded)).To(Succeed())
				g.Expect(seeded.Spec.ProjectRelease).To(Equal(latestRelease))
			}, itTimeout, itInterval).Should(Succeed())

			// The explicit pin is never touched.
			untouched := &openchoreov1alpha1.ProjectReleaseBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "custom-binding-manual", Namespace: nsName}, untouched)).To(Succeed())
			Expect(untouched.Spec.ProjectRelease).To(Equal("user-pinned-release"))
		})

		It("should seed a binding created after the release exists", func() {
			Expect(k8sClient.Create(ctx, newProject())).To(Succeed())

			r := itReconciler()

			// Reconcile until the release is cut.
			Eventually(func(g Gomega) {
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				g.Expect(err).NotTo(HaveOccurred())
				project := &openchoreov1alpha1.Project{}
				g.Expect(k8sClient.Get(ctx, nn, project)).To(Succeed())
				g.Expect(project.Status.LatestRelease).NotTo(BeNil())
			}, itTimeout, itInterval).Should(Succeed())

			// A binding authored after the release exists gets seeded on the
			// next reconcile (in production the PRB watch triggers it).
			late := newBinding("custom-binding-late", "env-late", "")
			Expect(k8sClient.Create(ctx, late)).To(Succeed())
			waitForCacheVisibility("custom-binding-late")

			Eventually(func(g Gomega) {
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				g.Expect(err).NotTo(HaveOccurred())

				project := &openchoreov1alpha1.Project{}
				g.Expect(k8sClient.Get(ctx, nn, project)).To(Succeed())

				seeded := &openchoreov1alpha1.ProjectReleaseBinding{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "custom-binding-late", Namespace: nsName}, seeded)).To(Succeed())
				g.Expect(seeded.Spec.ProjectRelease).To(Equal(project.Status.LatestRelease.Name))
			}, itTimeout, itInterval).Should(Succeed())
		})
	})

	Context("Finalization with owned Resources", func() {
		const (
			nsName   = "it-finalize-res-ns"
			dpName   = "it-finalize-res-dp"
			envName  = "it-finalize-res-env"
			pipName  = "it-finalize-res-pip"
			projName = "it-finalize-res-proj"
		)

		nn := types.NamespacedName{Name: projName, Namespace: nsName}

		BeforeEach(func() {
			setupDependencies(nsName, dpName, envName, pipName)
		})

		It("should delete owned Resources when Project is deleted", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projName,
					Namespace: nsName,
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
						Name: pipName,
					},
					Type: openchoreov1alpha1.ProjectTypeRef{
						Name: "default",
					},
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())

			r := itReconciler()

			// Reconcile project to add finalizer
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Reconcile project to set Created condition
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Create owned Resource
			resource := &openchoreov1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "it-finalize-res-resource",
					Namespace: nsName,
				},
				Spec: openchoreov1alpha1.ResourceSpec{
					Owner: openchoreov1alpha1.ResourceOwner{
						ProjectName: projName,
					},
					Type: openchoreov1alpha1.ResourceTypeRef{
						Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
						Name: "some-type",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			// Wait for resource to be visible in cache/client
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "it-finalize-res-resource", Namespace: nsName}, &openchoreov1alpha1.Resource{})
			}, itTimeout, itInterval).Should(Succeed())

			// Delete the Project
			updated := &openchoreov1alpha1.Project{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())

			// Verify the owned Resource is deleted
			Eventually(func() bool {
				_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "it-finalize-res-resource", Namespace: nsName}, &openchoreov1alpha1.Resource{})
				return errors.IsNotFound(err)
			}, itTimeout, itInterval).Should(BeTrue())

			// Verify the project is deleted
			Eventually(func() bool {
				_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.Project{})
				return errors.IsNotFound(err)
			}, itTimeout, itInterval).Should(BeTrue())
		})
	})

})
