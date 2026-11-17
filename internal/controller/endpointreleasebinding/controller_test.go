// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
)

// These run against the API server. They cover the paths that settle before
// backend resolution, which is exercised with a fake client (it needs the
// shared ReleaseBinding field index, which a direct envtest client lacks).
var _ = Describe("EndpointReleaseBinding Controller", func() {
	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Pipeline: endpointpipeline.NewPipeline()}
	})

	create := func(objs ...client.Object) {
		for _, obj := range objs {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })
		}
	}

	// reconcileTwice adds the finalizer on the first pass and reconciles on the
	// second, then returns the persisted binding.
	reconcileTwice := func(b *openchoreov1alpha1.EndpointReleaseBinding) *openchoreov1alpha1.EndpointReleaseBinding {
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}
		for range 2 {
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
		}
		updated := &openchoreov1alpha1.EndpointReleaseBinding{}
		Expect(k8sClient.Get(ctx, req.NamespacedName, updated)).To(Succeed())
		// Let the AfterEach-style cleanup through the finalizer.
		DeferCleanup(func() {
			latest := &openchoreov1alpha1.EndpointReleaseBinding{}
			if k8sClient.Get(ctx, req.NamespacedName, latest) == nil {
				latest.Finalizers = nil
				_ = k8sClient.Update(ctx, latest)
			}
		})
		return updated
	}

	expectCondition := func(b *openchoreov1alpha1.EndpointReleaseBinding, ct string, status metav1.ConditionStatus, reason string) *metav1.Condition {
		cond := meta.FindStatusCondition(b.Status.Conditions, ct)
		Expect(cond).NotTo(BeNil(), "expected %s condition", ct)
		Expect(cond.Status).To(Equal(status))
		Expect(cond.Reason).To(Equal(reason))
		return cond
	}

	It("adds the cleanup finalizer and reports Synced=False when no release is pinned", func() {
		b := newBinding("erb-no-pin", "")
		create(b)

		updated := reconcileTwice(b)
		Expect(updated.Finalizers).To(ContainElement(EndpointReleaseBindingFinalizer))
		expectCondition(updated, string(ConditionSynced), metav1.ConditionFalse, string(ReasonEndpointReleaseNotSet))
		expectCondition(updated, string(ConditionResourcesReady), metav1.ConditionUnknown, string(ReasonSyncedNotReady))
		cond := expectCondition(updated, string(ConditionReady), metav1.ConditionFalse, string(ReasonEndpointReleaseNotSet))
		Expect(cond.Message).To(ContainSubstring("spec.endpointRelease is unset"))
	})

	It("reports EndpointReleaseNotFound when the pinned release does not exist", func() {
		b := newBinding("erb-missing-release", "does-not-exist")
		create(b)

		updated := reconcileTwice(b)
		cond := expectCondition(updated, string(ConditionSynced), metav1.ConditionFalse, string(ReasonEndpointReleaseNotFound))
		Expect(cond.Message).To(Equal(`EndpointRelease "does-not-exist" not found`))
	})

	It("rejects a release owned by a different Endpoint", func() {
		release := newRelease("erb-foreign-release", componentBackend("stable", "greeter", 1))
		release.Spec.Owner.EndpointName = "someone-else"
		b := newBinding("erb-foreign", release.Name)
		create(release, b)

		updated := reconcileTwice(b)
		cond := expectCondition(updated, string(ConditionSynced), metav1.ConditionFalse, string(ReasonInvalidReleaseConfiguration))
		Expect(cond.Message).To(ContainSubstring(`endpoint: "someone-else"`))
	})

	It("reports InvalidEnvironmentConfigs when overrides violate the type's schema", func() {
		release := newRelease("erb-bad-envconfigs", componentBackend("stable", "greeter", 1))
		release.Spec.EndpointType.Spec.EnvironmentConfigs = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(
				`{"type":"object","properties":{"timeout":{"type":"integer"}}}`)},
		}
		b := newBinding("erb-bad-envconfigs", release.Name)
		b.Spec.EndpointTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"timeout":"soon"}`)}
		create(release, b)

		updated := reconcileTwice(b)
		cond := expectCondition(updated, string(ConditionSynced), metav1.ConditionFalse, string(ReasonInvalidEnvironmentConfigs))
		Expect(cond.Message).To(ContainSubstring("timeout"))
	})

	It("stamps lastSpecUpdateTime when the spec changes, and only then", func() {
		b := newBinding("erb-spec-update", "")
		create(b)
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		updated := reconcileTwice(b)
		Expect(updated.Status.LastSpecUpdateTime).NotTo(BeNil(), "first observation of the spec")

		// Backdate the stamp so a later one is distinguishable at second precision.
		past := metav1.NewTime(time.Now().Add(-24 * time.Hour).Truncate(time.Second))
		updated.Status.LastSpecUpdateTime = &past
		Expect(k8sClient.Status().Update(ctx, updated)).To(Succeed())

		// A reconcile without a spec change keeps the stamp.
		_, err := reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, updated)).To(Succeed())
		Expect(updated.Status.LastSpecUpdateTime.Equal(&past)).To(BeTrue())

		// Pinning a release (a deploy) bumps the generation and the stamp.
		updated.Spec.EndpointRelease = "erb-spec-update-release"
		Expect(k8sClient.Update(ctx, updated)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, updated)).To(Succeed())
		Expect(updated.Status.LastSpecUpdateTime.After(past.Time)).To(BeTrue())
	})

	It("reports EnvironmentNotFound when the target environment does not exist", func() {
		release := newRelease("erb-no-env", componentBackend("stable", "greeter", 1))
		b := newBinding("erb-no-env", release.Name)
		create(release, b)

		updated := reconcileTwice(b)
		expectCondition(updated, string(ConditionSynced), metav1.ConditionFalse, string(ReasonEnvironmentNotFound))
	})
})
