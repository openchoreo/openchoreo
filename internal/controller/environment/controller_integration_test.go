// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"time"

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
)

// newTestReconciler returns a Reconciler wired to the envtest API server.
// K8sClientMgr is intentionally nil — it is only accessed in the finalization
// path after makeEnvironmentContext succeeds, which requires a DataPlane to be
// present; tests that don't exercise that path are safe with nil.
func newTestReconciler() *Reconciler {
	return &Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
}

// forceDeleteEnv strips the cleanup finalizer and deletes the Environment,
// then waits until the API server confirms it is gone.
func forceDeleteEnv(nn types.NamespacedName) {
	env := &openchoreov1alpha1.Environment{}
	if err := k8sClient.Get(ctx, nn, env); err != nil {
		return
	}
	controllerutil.RemoveFinalizer(env, EnvCleanupFinalizer)
	_ = k8sClient.Update(ctx, env)
	_ = k8sClient.Delete(ctx, env)
	Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.Environment{}))
	}, "5s", "100ms").Should(BeTrue())
}

var _ = Describe("Environment Controller", func() {

	// All integration tests use the "default" namespace that envtest creates
	// automatically. Each Context uses a unique resource name to avoid
	// interference between tests that run in the same shared namespace.
	const ns = "default"

	// -------------------------------------------------------------------------
	// Reconcile: non-existent resource
	// -------------------------------------------------------------------------
	Describe("Reconcile non-existent resource", func() {
		It("should return no error and not requeue", func() {
			r := newTestReconciler()
			nn := types.NamespacedName{Namespace: ns, Name: "non-existent-env-xyz"}

			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	// -------------------------------------------------------------------------
	// Reconcile: first reconcile adds finalizer
	// -------------------------------------------------------------------------
	Describe("First reconcile", func() {
		var nn types.NamespacedName

		BeforeEach(func() {
			nn = types.NamespacedName{Namespace: ns, Name: "env-first-reconcile"}
			env := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nn.Name,
					Namespace: ns,
				},
				Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
			}
			Expect(k8sClient.Create(ctx, env)).To(Succeed())
		})

		AfterEach(func() { forceDeleteEnv(nn) })

		It("should add finalizer and return without requeue", func() {
			By("reconciling once")
			r := newTestReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			By("verifying finalizer was added")
			env := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(env, EnvCleanupFinalizer)).To(BeTrue())

			By("verifying no status conditions were set yet")
			Expect(env.Status.Conditions).To(BeEmpty())
		})
	})

	// -------------------------------------------------------------------------
	// Reconcile: second reconcile sets Ready condition
	// -------------------------------------------------------------------------
	Describe("Subsequent reconcile (finalizer already present)", func() {
		var nn types.NamespacedName

		BeforeEach(func() {
			nn = types.NamespacedName{Namespace: ns, Name: "env-second-reconcile"}
			env := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       nn.Name,
					Namespace:  ns,
					Finalizers: []string{EnvCleanupFinalizer},
				},
				Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
			}
			Expect(k8sClient.Create(ctx, env)).To(Succeed())
		})

		AfterEach(func() { forceDeleteEnv(nn) })

		It("should set the Ready condition to True", func() {
			r := newTestReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			env := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())

			cond := apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonDeploymentReady)))
			Expect(cond.Message).To(Equal("Environment is ready"))
		})

		It("should be idempotent across repeated reconciles", func() {
			r := newTestReconciler()
			for range 3 {
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			}

			env := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())

			count := 0
			for _, c := range env.Status.Conditions {
				if c.Type == ConditionReady.String() {
					count++
				}
			}
			Expect(count).To(Equal(1), "expected exactly one Ready condition")
		})
	})

	// -------------------------------------------------------------------------
	// Reconcile: full two-step lifecycle
	// -------------------------------------------------------------------------
	Describe("Full lifecycle (no pre-set finalizer)", func() {
		var nn types.NamespacedName

		BeforeEach(func() {
			nn = types.NamespacedName{Namespace: ns, Name: "env-full-lifecycle"}
			env := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nn.Name,
					Namespace: ns,
				},
				Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: true},
			}
			Expect(k8sClient.Create(ctx, env)).To(Succeed())
		})

		AfterEach(func() { forceDeleteEnv(nn) })

		It("should add finalizer on first reconcile, then set Ready on second", func() {
			r := newTestReconciler()

			By("first reconcile – finalizer added, no Ready condition yet")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			env := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(env, EnvCleanupFinalizer)).To(BeTrue())
			Expect(env.Status.Conditions).To(BeEmpty())

			By("second reconcile – Ready=True condition set")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
			cond := apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	// -------------------------------------------------------------------------
	// Status persistence via status subresource
	// -------------------------------------------------------------------------
	Describe("Status subresource persistence", func() {
		var nn types.NamespacedName

		BeforeEach(func() {
			nn = types.NamespacedName{Namespace: ns, Name: "env-status-persist"}
			env := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:       nn.Name,
					Namespace:  ns,
					Finalizers: []string{EnvCleanupFinalizer},
				},
				Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
			}
			Expect(k8sClient.Create(ctx, env)).To(Succeed())
		})

		AfterEach(func() { forceDeleteEnv(nn) })

		It("should persist status conditions written via the status subresource", func() {
			env := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())

			env.Status.Conditions = []metav1.Condition{
				NewEnvironmentReadyCondition(env.Generation),
			}
			Expect(k8sClient.Status().Update(ctx, env)).To(Succeed())

			fetched := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, ConditionReady.String())
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	// -------------------------------------------------------------------------
	// Finalization
	// -------------------------------------------------------------------------
	Describe("Finalization", func() {

		Context("deletion blocked by deployment pipeline (source ref)", func() {
			var nn types.NamespacedName
			var pipelineNN types.NamespacedName

			BeforeEach(func() {
				nn = types.NamespacedName{Namespace: ns, Name: "env-blocked-src"}
				env := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:       nn.Name,
						Namespace:  ns,
						Finalizers: []string{EnvCleanupFinalizer},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
				}
				Expect(k8sClient.Create(ctx, env)).To(Succeed())

				pipelineNN = types.NamespacedName{Namespace: ns, Name: "pipeline-src-ref"}
				pipeline := &openchoreov1alpha1.DeploymentPipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pipelineNN.Name,
						Namespace: ns,
					},
					Spec: openchoreov1alpha1.DeploymentPipelineSpec{
						PromotionPaths: []openchoreov1alpha1.PromotionPath{
							{
								SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: nn.Name},
								TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
									{Name: "production"},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
				// Wait for the DeploymentPipeline to be reflected in the manager cache
				Eventually(func() error {
					return k8sClient.Get(ctx, pipelineNN, &openchoreov1alpha1.DeploymentPipeline{})
				}, "5s", "100ms").Should(Succeed())

				Expect(k8sClient.Delete(ctx, env)).To(Succeed())
			})

			AfterEach(func() {
				forceDeleteEnv(nn)
				_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DeploymentPipeline{
					ObjectMeta: metav1.ObjectMeta{Name: pipelineNN.Name, Namespace: ns},
				})
			})

			It("should set DeletionBlocked condition and requeue", func() {
				r := newTestReconciler()
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(30 * time.Second))

				env := &openchoreov1alpha1.Environment{}
				Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())

				cond := apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(string(ReasonDeletionBlocked)))
				Expect(cond.Message).To(ContainSubstring(pipelineNN.Name))
			})
		})

		Context("deletion blocked by deployment pipeline (target ref)", func() {
			var nn types.NamespacedName
			var pipelineNN types.NamespacedName

			BeforeEach(func() {
				nn = types.NamespacedName{Namespace: ns, Name: "env-blocked-tgt"}
				env := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:       nn.Name,
						Namespace:  ns,
						Finalizers: []string{EnvCleanupFinalizer},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
				}
				Expect(k8sClient.Create(ctx, env)).To(Succeed())

				pipelineNN = types.NamespacedName{Namespace: ns, Name: "pipeline-tgt-ref"}
				pipeline := &openchoreov1alpha1.DeploymentPipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pipelineNN.Name,
						Namespace: ns,
					},
					Spec: openchoreov1alpha1.DeploymentPipelineSpec{
						PromotionPaths: []openchoreov1alpha1.PromotionPath{
							{
								SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "development"},
								TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
									{Name: nn.Name},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
				// Wait for the DeploymentPipeline to be reflected in the manager cache
				Eventually(func() error {
					return k8sClient.Get(ctx, pipelineNN, &openchoreov1alpha1.DeploymentPipeline{})
				}, "5s", "100ms").Should(Succeed())

				Expect(k8sClient.Delete(ctx, env)).To(Succeed())
			})

			AfterEach(func() {
				forceDeleteEnv(nn)
				_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DeploymentPipeline{
					ObjectMeta: metav1.ObjectMeta{Name: pipelineNN.Name, Namespace: ns},
				})
			})

			It("should set DeletionBlocked condition and requeue", func() {
				r := newTestReconciler()
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(30 * time.Second))

				env := &openchoreov1alpha1.Environment{}
				Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())

				cond := apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(string(ReasonDeletionBlocked)))
				Expect(cond.Message).To(ContainSubstring(pipelineNN.Name))
			})
		})

		Context("deletion unblocked after pipeline reference removed", func() {
			var nn types.NamespacedName
			var pipelineNN types.NamespacedName

			BeforeEach(func() {
				nn = types.NamespacedName{Namespace: ns, Name: "env-unblock"}
				env := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:       nn.Name,
						Namespace:  ns,
						Finalizers: []string{EnvCleanupFinalizer},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
				}
				Expect(k8sClient.Create(ctx, env)).To(Succeed())

				pipelineNN = types.NamespacedName{Namespace: ns, Name: "pipeline-unblock"}
				pipeline := &openchoreov1alpha1.DeploymentPipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pipelineNN.Name,
						Namespace: ns,
					},
					Spec: openchoreov1alpha1.DeploymentPipelineSpec{
						PromotionPaths: []openchoreov1alpha1.PromotionPath{
							{
								SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: nn.Name},
								TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
									{Name: "production"},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
				// Wait for the DeploymentPipeline to be reflected in the manager cache
				Eventually(func() error {
					return k8sClient.Get(ctx, pipelineNN, &openchoreov1alpha1.DeploymentPipeline{})
				}, "5s", "100ms").Should(Succeed())

				Expect(k8sClient.Delete(ctx, env)).To(Succeed())
			})

			AfterEach(func() {
				forceDeleteEnv(nn)
				_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DeploymentPipeline{
					ObjectMeta: metav1.ObjectMeta{Name: pipelineNN.Name, Namespace: ns},
				})
			})

			It("should transition from DeletionBlocked to Finalizing after pipeline is removed", func() {
				r := newTestReconciler()

				By("first reconcile — blocked")
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(30 * time.Second))

				env := &openchoreov1alpha1.Environment{}
				Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
				cond := apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
				Expect(cond).NotTo(BeNil())
				Expect(cond.Reason).To(Equal(string(ReasonDeletionBlocked)))

				By("removing the pipeline reference")
				pipeline := &openchoreov1alpha1.DeploymentPipeline{}
				Expect(k8sClient.Get(ctx, pipelineNN, pipeline)).To(Succeed())
				Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
				Eventually(func() bool {
					return apierrors.IsNotFound(k8sClient.Get(ctx, pipelineNN, &openchoreov1alpha1.DeploymentPipeline{}))
				}, "5s", "100ms").Should(BeTrue())

				By("second reconcile — sets Finalizing condition")
				result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())

				Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
				cond = apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
				Expect(cond).NotTo(BeNil())
				Expect(cond.Reason).To(Equal(string(ReasonEnvironmentFinalizing)))
			})
		})

		Context("first reconcile after deletion with no pipeline references", func() {
			var nn types.NamespacedName

			BeforeEach(func() {
				nn = types.NamespacedName{Namespace: ns, Name: "env-finalize-first"}
				env := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:       nn.Name,
						Namespace:  ns,
						Finalizers: []string{EnvCleanupFinalizer},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
				}
				Expect(k8sClient.Create(ctx, env)).To(Succeed())
				Expect(k8sClient.Delete(ctx, env)).To(Succeed())
			})

			AfterEach(func() { forceDeleteEnv(nn) })

			It("should set the Finalizing condition and return without error", func() {
				r := newTestReconciler()
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())

				env := &openchoreov1alpha1.Environment{}
				Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
				Expect(env.DeletionTimestamp).NotTo(BeNil())

				cond := apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(string(ReasonEnvironmentFinalizing)))
				Expect(cond.Message).To(Equal("Environment is finalizing"))
			})
		})

		Context("waiting for release bindings during finalization", func() {
			var nn types.NamespacedName
			var rbNN types.NamespacedName

			BeforeEach(func() {
				nn = types.NamespacedName{Namespace: ns, Name: "env-rb-cleanup"}
				env := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:       nn.Name,
						Namespace:  ns,
						Finalizers: []string{EnvCleanupFinalizer},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
				}
				Expect(k8sClient.Create(ctx, env)).To(Succeed())

				rbNN = types.NamespacedName{Namespace: ns, Name: "rb-for-env-cleanup"}
				rb := &openchoreov1alpha1.ReleaseBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rbNN.Name,
						Namespace: ns,
					},
					Spec: openchoreov1alpha1.ReleaseBindingSpec{
						Owner: openchoreov1alpha1.ReleaseBindingOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Environment: nn.Name,
					},
				}
				Expect(k8sClient.Create(ctx, rb)).To(Succeed())

				Expect(k8sClient.Delete(ctx, env)).To(Succeed())
			})

			AfterEach(func() {
				forceDeleteEnv(nn)
				// Clean up ReleaseBinding if it still exists
				rb := &openchoreov1alpha1.ReleaseBinding{}
				if err := k8sClient.Get(ctx, rbNN, rb); err == nil {
					controllerutil.RemoveFinalizer(rb, "openchoreo.dev/releasebinding-cleanup")
					_ = k8sClient.Update(ctx, rb)
					_ = k8sClient.Delete(ctx, rb)
				}
			})

			It("should wait for release bindings without deleting them and requeue", func() {
				r := newTestReconciler()

				By("first reconcile — sets Finalizing condition")
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())

				By("second reconcile — detects release binding and requeues")
				result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(5 * time.Second))

				By("verifying the release binding is NOT deleted (controller only waits)")
				rb := &openchoreov1alpha1.ReleaseBinding{}
				Expect(k8sClient.Get(ctx, rbNN, rb)).To(Succeed())
				Expect(rb.DeletionTimestamp).To(BeNil())

				By("verifying the ReleaseBindingsPending condition is set")
				env := &openchoreov1alpha1.Environment{}
				Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
				cond := apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(string(ReasonReleaseBindingsPending)))
			})

			It("should proceed with finalization after release bindings are externally removed", func() {
				r := newTestReconciler()

				By("first reconcile — sets Finalizing condition")
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())

				By("second reconcile — waits for release bindings")
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(5 * time.Second))

				By("externally removing the release binding")
				Expect(k8sClient.Delete(ctx, &openchoreov1alpha1.ReleaseBinding{
					ObjectMeta: metav1.ObjectMeta{Name: rbNN.Name, Namespace: ns},
				})).To(Succeed())
				Eventually(func() bool {
					return apierrors.IsNotFound(k8sClient.Get(ctx, rbNN, &openchoreov1alpha1.ReleaseBinding{}))
				}, "5s", "100ms").Should(BeTrue())

				By("third reconcile — no release bindings, re-sets Finalizing condition")
				_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())

				By("fourth reconcile — proceeds to namespace cleanup (DataPlane not found, removes finalizer)")
				_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())

				By("verifying the environment is gone")
				Eventually(func() bool {
					return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.Environment{}))
				}, "5s", "100ms").Should(BeTrue())
			})
		})

		Context("full finalization without DataPlane (skips namespace cleanup)", func() {
			var nn types.NamespacedName

			BeforeEach(func() {
				nn = types.NamespacedName{Namespace: ns, Name: "env-finalize-no-dp"}
				env := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:       nn.Name,
						Namespace:  ns,
						Finalizers: []string{EnvCleanupFinalizer},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
				}
				Expect(k8sClient.Create(ctx, env)).To(Succeed())
				Expect(k8sClient.Delete(ctx, env)).To(Succeed())
			})

			// No AfterEach needed — the environment should be fully deleted.

			It("should complete finalization by removing the finalizer", func() {
				r := newTestReconciler()

				By("first reconcile — sets Finalizing condition")
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())

				By("second reconcile — no release bindings, DataPlane not found, removes finalizer")
				result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())

				By("verifying the environment is gone (finalizer removed, K8s garbage collects)")
				Eventually(func() bool {
					return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.Environment{}))
				}, "5s", "100ms").Should(BeTrue())
			})
		})

		Context("release bindings for other environments are not deleted", func() {
			var nn types.NamespacedName
			var otherRBNN types.NamespacedName

			BeforeEach(func() {
				nn = types.NamespacedName{Namespace: ns, Name: "env-rb-isolation"}
				env := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:       nn.Name,
						Namespace:  ns,
						Finalizers: []string{EnvCleanupFinalizer},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
				}
				Expect(k8sClient.Create(ctx, env)).To(Succeed())

				// ReleaseBinding for a DIFFERENT environment
				otherRBNN = types.NamespacedName{Namespace: ns, Name: "rb-other-env"}
				rb := &openchoreov1alpha1.ReleaseBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      otherRBNN.Name,
						Namespace: ns,
					},
					Spec: openchoreov1alpha1.ReleaseBindingSpec{
						Owner: openchoreov1alpha1.ReleaseBindingOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Environment: "some-other-environment",
					},
				}
				Expect(k8sClient.Create(ctx, rb)).To(Succeed())

				Expect(k8sClient.Delete(ctx, env)).To(Succeed())
			})

			AfterEach(func() {
				forceDeleteEnv(nn)
				_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ReleaseBinding{
					ObjectMeta: metav1.ObjectMeta{Name: otherRBNN.Name, Namespace: ns},
				})
			})

			It("should not delete release bindings belonging to other environments", func() {
				r := newTestReconciler()

				By("first reconcile — sets Finalizing condition")
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())

				By("second reconcile — no matching release bindings, proceeds to namespace cleanup")
				_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())

				By("verifying the other environment's release binding is untouched")
				rb := &openchoreov1alpha1.ReleaseBinding{}
				Expect(k8sClient.Get(ctx, otherRBNN, rb)).To(Succeed())
				Expect(rb.DeletionTimestamp).To(BeNil())
			})
		})
	})

	// -------------------------------------------------------------------------
	// CRD-level validation: dataPlaneRef immutability
	// -------------------------------------------------------------------------
	Describe("DataPlaneRef immutability", func() {
		var nn types.NamespacedName

		BeforeEach(func() {
			nn = types.NamespacedName{Namespace: ns, Name: "env-dp-immutable"}
			env := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nn.Name,
					Namespace: ns,
				},
				Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: false},
			}
			Expect(k8sClient.Create(ctx, env)).To(Succeed())
		})

		AfterEach(func() { forceDeleteEnv(nn) })

		It("should allow setting dataPlaneRef when previously unset", func() {
			env := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
			env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "my-dataplane",
			}
			Expect(k8sClient.Update(ctx, env)).To(Succeed())
		})

		It("should reject changing dataPlaneRef once set", func() {
			By("setting the initial dataPlaneRef")
			env := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
			env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "original-dp",
			}
			Expect(k8sClient.Update(ctx, env)).To(Succeed())

			By("attempting to change to a different dataPlaneRef")
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
			env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "different-dp",
			}
			err := k8sClient.Update(ctx, env)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dataPlaneRef is immutable once set"))
		})

		It("should allow updating other spec fields while keeping dataPlaneRef the same", func() {
			env := &openchoreov1alpha1.Environment{}
			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
			env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "my-dp",
			}
			env.Spec.IsProduction = true
			Expect(k8sClient.Update(ctx, env)).To(Succeed())

			Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
			Expect(env.Spec.IsProduction).To(BeTrue())
		})
	})
})
