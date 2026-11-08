// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/planenamespace"
)

var _ = Describe("DataPlane plane namespace cleanup", func() {
	const dpName = "dp-kv"

	newPlane := func(cpNamespace string) client.Client {
		return fake.NewClientBuilder().WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: planenamespace.KV.Name(cpNamespace), Labels: planenamespace.KV.Labels(cpNamespace),
		}}).Build()
	}

	kvExists := func(plane client.Client, cpNamespace string) bool {
		err := plane.Get(ctx, client.ObjectKey{Name: planenamespace.KV.Name(cpNamespace)}, &corev1.Namespace{})
		if apierrors.IsNotFound(err) {
			return false
		}
		Expect(err).NotTo(HaveOccurred())
		return true
	}

	// A nil plane gives a provider with no expectations, which fails the test if called.
	reconcilerFor := func(plane client.Client) *Reconciler {
		provider := k8sMocks.NewMockDataPlaneClientProvider(GinkgoT())
		if plane != nil {
			provider.EXPECT().DataPlaneClient(mock.Anything).Return(plane, nil).Maybe()
		}
		r := testReconciler()
		r.PlaneClientProvider = provider
		r.APIReader = k8sClient
		return r
	}

	createNamespaceWithDataPlane := func(nsName string) types.NamespacedName {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}})).To(Succeed())
		nn := types.NamespacedName{Name: dpName, Namespace: nsName}
		Expect(k8sClient.Create(ctx, &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{
			Name: nn.Name, Namespace: nn.Namespace, Finalizers: []string{DataPlaneCleanupFinalizer},
		}})).To(Succeed())
		DeferCleanup(forceDeleteDP, ctx, nn)
		return nn
	}

	deleteDataPlane := func(nn types.NamespacedName) {
		dp := &openchoreov1alpha1.DataPlane{}
		Expect(k8sClient.Get(ctx, nn, dp)).To(Succeed())
		Expect(k8sClient.Delete(ctx, dp)).To(Succeed())
	}

	finalizeUntilGone := func(r *Reconciler, nn types.NamespacedName) {
		Eventually(func() bool {
			_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.DataPlane{}))
		}, "10s", "200ms").Should(BeTrue())
	}

	It("deletes the kv namespace when its control plane namespace is deleted", func() {
		const nsName = "kv-dp-terminating"
		nn := createNamespaceWithDataPlane(nsName)

		By("deleting the control plane namespace, which envtest leaves terminating")
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}})).To(Succeed())
		Eventually(func(g Gomega) {
			ns := &corev1.Namespace{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, ns)).To(Succeed())
			g.Expect(ns.DeletionTimestamp).NotTo(BeNil())
		}, "10s", "200ms").Should(Succeed())
		deleteDataPlane(nn)

		plane := newPlane(nsName)
		finalizeUntilGone(reconcilerFor(plane), nn)
		Expect(kvExists(plane, nsName)).To(BeFalse())
	})

	It("keeps the kv namespace and waits for secrets when only the DataPlane is deleted", func() {
		const nsName = "kv-dp-live"
		nn := createNamespaceWithDataPlane(nsName)
		sr := &openchoreov1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{
				Name: "db", Namespace: nsName,
				Labels: map[string]string{"openchoreo.dev/managed-by": "openchoreo-api"},
			},
			Spec: openchoreov1alpha1.SecretReferenceSpec{
				TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: "DataPlane", Name: dpName},
				Data: []openchoreov1alpha1.SecretDataSource{{
					SecretKey: "password",
					RemoteRef: openchoreov1alpha1.RemoteReference{Key: nsName + "/generic/db", Property: "password"},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())
		Eventually(func(g Gomega) {
			var list openchoreov1alpha1.SecretReferenceList
			g.Expect(k8sClient.List(ctx, &list, client.InNamespace(nsName))).To(Succeed())
			g.Expect(list.Items).To(HaveLen(1))
		}, "10s", "200ms").Should(Succeed())
		deleteDataPlane(nn)

		r := reconcilerFor(nil)

		By("blocking while the SecretReference targets the DataPlane")
		result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(controller.PlaneDeletionRetryInterval))
		dp := &openchoreov1alpha1.DataPlane{}
		Expect(k8sClient.Get(ctx, nn, dp)).To(Succeed())
		cond := apimeta.FindStatusCondition(dp.Status.Conditions, string(ConditionFinalizing))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).To(Equal(string(ReasonDeletionBlocked)))
		Expect(cond.Message).To(ContainSubstring("1 secret(s)"))

		By("finalizing without touching the plane once the SecretReference is gone")
		Expect(k8sClient.Delete(ctx, sr)).To(Succeed())
		finalizeUntilGone(r, nn)
	})
})
