// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package bindingrevision

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func releaseBindingRevision() *openchoreov1alpha1.ReleaseBindingRevision {
	return &openchoreov1alpha1.ReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "rbr-", Namespace: "default"},
		Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{
			Revision:     1,
			Cause:        openchoreov1alpha1.RevisionCauseDeploy,
			SnapshotHash: "hash-1",
			BindingSpec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner:                           openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "proj", ComponentName: "comp"},
				Environment:                     "dev",
				ReleaseName:                     "comp-abc123",
				ComponentTypeEnvironmentConfigs: &runtime.RawExtension{Raw: []byte(`{"replicas":2,"cpu":"100m"}`)},
				TraitEnvironmentConfigs: map[string]runtime.RawExtension{
					"ingress": {Raw: []byte(`{"host":"a.example.com"}`)},
				},
			},
		},
	}
}

func resourceReleaseBindingRevision() *openchoreov1alpha1.ResourceReleaseBindingRevision {
	return &openchoreov1alpha1.ResourceReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "rrbr-", Namespace: "default"},
		Spec: openchoreov1alpha1.ResourceReleaseBindingRevisionSpec{
			Revision:     1,
			Cause:        openchoreov1alpha1.RevisionCauseDeploy,
			SnapshotHash: "hash-1",
			BindingSpec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:                          openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "proj", ResourceName: "db"},
				Environment:                    "dev",
				ResourceRelease:                "db-abc123",
				ResourceTypeEnvironmentConfigs: &runtime.RawExtension{Raw: []byte(`{"size":"small"}`)},
			},
		},
	}
}

func expectRejected(err error) {
	GinkgoHelper()
	Expect(err).To(HaveOccurred())
	Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid error, got %v", err)
	Expect(err.Error()).To(ContainSubstring(ImmutableSpecMessage))
}

var _ = Describe("binding revision webhook", func() {
	Context("ReleaseBindingRevision updates", func() {
		v := &ReleaseBindingRevisionValidator{}

		DescribeTable("rejects any change to the spec",
			func(mutate func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec)) {
				oldRev := releaseBindingRevision()
				newRev := oldRev.DeepCopy()
				mutate(&newRev.Spec)
				_, err := v.ValidateUpdate(ctx, oldRev, newRev)
				expectRejected(err)
			},
			// The case CEL misses: same number of keys, one key replaced.
			Entry("a config key replaced by another", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"count":2,"cpu":"100m"}`)}
			}),
			Entry("a trait config key replaced by another", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.TraitEnvironmentConfigs["ingress"] = runtime.RawExtension{Raw: []byte(`{"path":"a.example.com"}`)}
			}),
			Entry("a config value changed", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":3,"cpu":"100m"}`)}
			}),
			Entry("a config key added", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":2,"cpu":"100m","memory":"1Gi"}`)}
			}),
			Entry("the configs removed", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.ComponentTypeEnvironmentConfigs = nil
			}),
			Entry("the release", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) { s.BindingSpec.ReleaseName = "comp-def456" }),
			Entry("the revision number", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) { s.Revision = 2 }),
			Entry("the change message", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) { s.Message = "edited" }),
		)

		It("accepts the same spec with its configs' keys in another order", func() {
			oldRev := releaseBindingRevision()
			newRev := oldRev.DeepCopy()
			newRev.Spec.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{ "cpu": "100m", "replicas": 2 }`)}
			_, err := v.ValidateUpdate(ctx, oldRev, newRev)
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts metadata and status changes", func() {
			oldRev := releaseBindingRevision()
			newRev := oldRev.DeepCopy()
			newRev.Labels = map[string]string{"openchoreo.dev/release-binding": "comp-dev"}
			newRev.Annotations = map[string]string{"openchoreo.dev/binding-generation": "3"}
			newRev.Status.Active = true
			_, err := v.ValidateUpdate(ctx, oldRev, newRev)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("ResourceReleaseBindingRevision updates", func() {
		v := &ResourceReleaseBindingRevisionValidator{}

		It("rejects a config key replaced by another", func() {
			oldRev := resourceReleaseBindingRevision()
			newRev := oldRev.DeepCopy()
			newRev.Spec.BindingSpec.ResourceTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"tier":"small"}`)}
			_, err := v.ValidateUpdate(ctx, oldRev, newRev)
			expectRejected(err)
		})

		It("accepts metadata changes", func() {
			oldRev := resourceReleaseBindingRevision()
			newRev := oldRev.DeepCopy()
			newRev.Labels = map[string]string{"openchoreo.dev/resource-release-binding": "db-dev"}
			_, err := v.ValidateUpdate(ctx, oldRev, newRev)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// Against a real apiserver with the webhook registered: the key
	// replacement the CRD's CEL rule lets through is rejected.
	Context("installed on an apiserver", func() {
		BeforeEach(func() {
			if k8sClient == nil {
				Skip("envtest apiserver not available")
			}
		})

		It("rejects a config key replacement in a ReleaseBindingRevision", func() {
			rev := releaseBindingRevision()
			Expect(k8sClient.Create(ctx, rev)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rev) })

			updated := rev.DeepCopy()
			updated.Spec.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"count":2,"cpu":"100m"}`)}
			expectRejected(k8sClient.Update(ctx, updated))

			stored := &openchoreov1alpha1.ReleaseBindingRevision{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rev), stored)).To(Succeed())
			Expect(string(stored.Spec.BindingSpec.ComponentTypeEnvironmentConfigs.Raw)).To(MatchJSON(`{"replicas":2,"cpu":"100m"}`))
		})

		It("still accepts the controller's metadata and status writes", func() {
			rev := releaseBindingRevision()
			Expect(k8sClient.Create(ctx, rev)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rev) })

			rev.Labels = map[string]string{"openchoreo.dev/release-binding": "comp-dev"}
			Expect(k8sClient.Update(ctx, rev)).To(Succeed())
			rev.Status.Active = true
			rev.Status.Health = openchoreov1alpha1.RevisionHealthHealthy
			Expect(k8sClient.Status().Update(ctx, rev)).To(Succeed())
		})

		It("rejects a config key replacement in a ResourceReleaseBindingRevision", func() {
			rev := resourceReleaseBindingRevision()
			Expect(k8sClient.Create(ctx, rev)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rev) })

			updated := rev.DeepCopy()
			updated.Spec.BindingSpec.ResourceTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"tier":"small"}`)}
			Expect(k8sClient.Update(ctx, updated)).NotTo(Succeed())
		})
	})
})
