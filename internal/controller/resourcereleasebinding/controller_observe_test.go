// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ResourceReleaseBinding — Observe mode", func() {
	deployment := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "apps/v1", Kind: "Deployment", Name: "redis-cart"}
	service := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "v1", Kind: "Service", Name: "redis-cart"}

	newBinding := func(name string, mode openchoreov1alpha1.BindingMode,
		claims ...openchoreov1alpha1.ObservedResourceClaim) *openchoreov1alpha1.ResourceReleaseBinding {
		b := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "test-project", ResourceName: "test-resource"},
				Environment: name,
				Mode:        mode,
			},
		}
		if len(claims) > 0 {
			b.Spec.Observe = &openchoreov1alpha1.ResourceReleaseBindingObserve{Resources: claims}
		}
		return b
	}

	create := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		GinkgoHelper()
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, b) })
		return b
	}

	Context("validation", func() {
		It("defaults mode to Manage", func() {
			b := create(newBinding("rrb-default-mode", ""))
			Expect(b.Spec.Mode).To(Equal(openchoreov1alpha1.BindingModeManage))
		})

		It("rejects an unknown mode", func() {
			b := newBinding("rrb-unknown-mode", "Watch")
			Expect(k8sClient.Create(ctx, b)).To(MatchError(ContainSubstring("spec.mode")))
		})

		It("rejects the same claim twice", func() {
			b := newBinding("rrb-duplicate-claim", openchoreov1alpha1.BindingModeObserve, deployment, deployment)
			Expect(k8sClient.Create(ctx, b)).To(MatchError(ContainSubstring("Duplicate value")))
		})

		It("takes the same name under another kind or version", func() {
			hpaV1 := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "redis-cart"}
			hpaV2 := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "redis-cart"}
			b := create(newBinding("rrb-same-name", openchoreov1alpha1.BindingModeObserve, deployment, service, hpaV1, hpaV2))
			Expect(b.Spec.Observe.Resources).To(HaveLen(4))
		})

		It("lets the mode change", func() {
			b := create(newBinding("rrb-mode-change", openchoreov1alpha1.BindingModeManage))
			current := &openchoreov1alpha1.ResourceReleaseBinding{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), current)).To(Succeed())
			current.Spec.Mode = openchoreov1alpha1.BindingModeObserve
			current.Spec.Observe = &openchoreov1alpha1.ResourceReleaseBindingObserve{
				Resources: []openchoreov1alpha1.ObservedResourceClaim{deployment},
			}
			Expect(k8sClient.Update(ctx, current)).To(Succeed())
			Expect(current.Spec.IsObserve()).To(BeTrue())
		})
	})
})
