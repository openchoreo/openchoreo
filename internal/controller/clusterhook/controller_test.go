// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Same contract as the Hook controller: Available=True with the observed generation
// is the only signal a pipeline author has that a ClusterHook can be bound.
var _ = Describe("ClusterHook Controller", func() {
	It("marks a ClusterHook Available with the observed generation", func() {
		key := types.NamespacedName{Name: "trivy-image-scan"}
		hook := &openchoreov1alpha1.ClusterHook{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name},
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

		got := &openchoreov1alpha1.ClusterHook{}
		Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))
		Expect(meta.IsStatusConditionTrue(got.Status.Conditions, ConditionAvailable)).To(BeTrue())
	})
})
