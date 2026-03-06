// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

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

		Context("first reconcile after deletion (Finalizing condition not yet set)", func() {
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
				// Deletion sets DeletionTimestamp but resource stays because of finalizer.
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

		Context("repeated reconciles after deletion", func() {
			var nn types.NamespacedName

			BeforeEach(func() {
				nn = types.NamespacedName{Namespace: ns, Name: "env-finalize-repeat"}
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

			It("should keep the Finalizing condition set across multiple reconciles", func() {
				r := newTestReconciler()

				By("first reconcile after deletion sets Finalizing condition")
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())

				env := &openchoreov1alpha1.Environment{}
				Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
				cond := apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(string(ReasonEnvironmentFinalizing)))

				By("second reconcile is idempotent — Finalizing condition remains set")
				_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				// The second reconcile either returns nil (condition already set and DataPlane
				// lookup fails gracefully) or returns an error (DataPlane not found). Either
				// way the Finalizing condition must still be present.
				_ = err

				Expect(k8sClient.Get(ctx, nn, env)).To(Succeed())
				cond = apimeta.FindStatusCondition(env.Status.Conditions, ConditionReady.String())
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(string(ReasonEnvironmentFinalizing)))
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
