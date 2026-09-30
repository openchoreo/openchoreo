// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Fake-client specs for the branches the envtest spec cannot reach cleanly:
// API errors, deletion and a stale observedGeneration.
var _ = Describe("ClusterHook Controller (fake client)", func() {
	var (
		key    = types.NamespacedName{Name: "trivy-image-scan"}
		errAPI = errors.New("api down")
	)

	newHook := func(generation int64, conds ...metav1.Condition) *openchoreov1alpha1.ClusterHook {
		return &openchoreov1alpha1.ClusterHook{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Generation: generation},
			Status:     openchoreov1alpha1.HookStatus{ObservedGeneration: generation - 1, Conditions: conds},
		}
	}
	newReconciler := func(funcs *interceptor.Funcs, objs ...client.Object) (*Reconciler, client.Client) {
		b := fake.NewClientBuilder().WithScheme(scheme.Scheme).
			WithObjects(objs...).WithStatusSubresource(&openchoreov1alpha1.ClusterHook{})
		if funcs != nil {
			b = b.WithInterceptorFuncs(*funcs)
		}
		c := b.Build()
		return &Reconciler{Client: c, Scheme: scheme.Scheme}, c
	}
	reconcileHook := func(r *Reconciler) error {
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		return err
	}

	// A ClusterHook deleted before its event is processed must not fail the reconcile,
	// or the workqueue retries a ClusterHook that no longer exists.
	It("ignores a ClusterHook that no longer exists", func() {
		r, _ := newReconciler(nil)
		Expect(reconcileHook(r)).To(Succeed())
	})

	// Any other read failure must be returned so the request is retried.
	// Swallowing it would leave the ClusterHook without Available.
	It("returns a Get error", func() {
		r, _ := newReconciler(&interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errAPI
			},
		})
		Expect(reconcileHook(r)).To(MatchError(errAPI))
	})

	// A terminating ClusterHook must not be marked Available. Doing so would tell
	// pipeline authors they can still bind a ClusterHook that is being removed.
	It("leaves a terminating ClusterHook untouched", func() {
		h := newHook(1)
		h.Finalizers = []string{"test/keep"}
		now := metav1.Now()
		h.DeletionTimestamp = &now
		r, c := newReconciler(nil, h)

		Expect(reconcileHook(r)).To(Succeed())
		got := &openchoreov1alpha1.ClusterHook{}
		Expect(c.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.Conditions).To(BeEmpty())
		Expect(got.Status.ObservedGeneration).To(BeZero())
	})

	// A spec edit bumps the generation. Available=True from the old
	// generation must be refreshed, or observedGeneration would suggest the
	// edit was never processed.
	It("refreshes a stale observedGeneration even when already Available", func() {
		r, c := newReconciler(nil, newHook(3, metav1.Condition{
			Type: ConditionAvailable, Status: metav1.ConditionTrue, Reason: ReasonAvailable, ObservedGeneration: 2,
		}))

		Expect(reconcileHook(r)).To(Succeed())
		got := &openchoreov1alpha1.ClusterHook{}
		Expect(c.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.ObservedGeneration).To(Equal(int64(3)))
		cond := meta.FindStatusCondition(got.Status.Conditions, ConditionAvailable)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.ObservedGeneration).To(Equal(int64(3)))
	})

	// An up-to-date ClusterHook must not be written again. Every write triggers another
	// watch event, so an unconditional write would reconcile it forever.
	It("skips the status write when already Available at the current generation", func() {
		h := newHook(2, metav1.Condition{
			Type: ConditionAvailable, Status: metav1.ConditionTrue, Reason: ReasonAvailable, ObservedGeneration: 2,
		})
		h.Status.ObservedGeneration = 2
		r, _ := newReconciler(&interceptor.Funcs{
			SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
				Fail("status must not be written for an up-to-date ClusterHook")
				return nil
			},
		}, h)
		Expect(reconcileHook(r)).To(Succeed())
	})

	// A failed status write must be returned so the ClusterHook is requeued rather
	// than stuck without Available.
	It("returns a status update error", func() {
		r, _ := newReconciler(&interceptor.Funcs{
			SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
				return errAPI
			},
		}, newHook(1))
		Expect(reconcileHook(r)).To(MatchError(errAPI))
	})

	// Without this registration the manager never reconciles ClusterHooks, so none of
	// them would ever become Available.
	It("registers with a manager", func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  scheme.Scheme,
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect((&Reconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr)).To(Succeed())
	})
})
