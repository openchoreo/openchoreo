// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// The Hook controller has one job: tell operators (and the portal) that a Hook was
// admitted and can be bound. Without Available=True and an up-to-date
// observedGeneration a pipeline author cannot tell a typo'd hookRef from a hook
// that is still being processed.
var _ = Describe("Hook Controller", func() {
	It("marks a Hook Available with the observed generation", func() {
		key := types.NamespacedName{Name: "scan", Namespace: "default"}
		hook := &openchoreov1alpha1.Hook{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
			Spec: openchoreov1alpha1.HookSpec{
				Type:        openchoreov1alpha1.HookTypeWorkflow,
				WorkflowRef: &openchoreov1alpha1.WorkflowRef{Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow, Name: "trivy"},
			},
		}
		Expect(k8sClient.Create(ctx, hook)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, hook) })

		r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		got := &openchoreov1alpha1.Hook{}
		Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))
		Expect(meta.IsStatusConditionTrue(got.Status.Conditions, ConditionAvailable)).To(BeTrue())

		By("a second reconcile is a no-op and keeps the status stable")
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		again := &openchoreov1alpha1.Hook{}
		Expect(k8sClient.Get(ctx, key, again)).To(Succeed())
		Expect(again.ResourceVersion).To(Equal(got.ResourceVersion))
	})
})
