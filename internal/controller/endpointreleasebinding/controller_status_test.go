// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
)

var _ = Describe("Status evaluation", func() {
	withCondition := func(b *openchoreov1alpha1.EndpointReleaseBinding, ct controller.ConditionType,
		status metav1.ConditionStatus, reason string, generation int64) {
		meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
			Type: string(ct), Status: status, Reason: reason, Message: reason, ObservedGeneration: generation,
		})
	}
	ready := func(b *openchoreov1alpha1.EndpointReleaseBinding) *metav1.Condition {
		return meta.FindStatusCondition(b.Status.Conditions, string(ConditionReady))
	}

	Describe("setReadyCondition", func() {
		It("is Ready when every sub-condition is True for the current generation", func() {
			b := newBinding("b", testRelease)
			b.Generation = 2
			withCondition(b, ConditionBackendsResolved, metav1.ConditionTrue, "Resolved", 2)
			withCondition(b, ConditionSynced, metav1.ConditionTrue, "Synced", 2)
			withCondition(b, ConditionResourcesReady, metav1.ConditionTrue, "Ready", 2)
			withCondition(b, ConditionURLsResolved, metav1.ConditionTrue, "URLsResolved", 2)

			setReadyCondition(b)

			Expect(ready(b).Status).To(Equal(metav1.ConditionTrue))
			Expect(ready(b).ObservedGeneration).To(Equal(int64(2)))
		})

		It("is not Ready on sub-conditions left over from an earlier generation", func() {
			// A repinned binding whose reconcile failed before re-evaluating anything.
			b := newBinding("b", testRelease)
			b.Generation = 3
			withCondition(b, ConditionBackendsResolved, metav1.ConditionTrue, "Resolved", 2)
			withCondition(b, ConditionSynced, metav1.ConditionTrue, "Synced", 2)
			withCondition(b, ConditionResourcesReady, metav1.ConditionTrue, "Ready", 2)

			setReadyCondition(b)

			Expect(ready(b).Status).To(Equal(metav1.ConditionFalse))
			Expect(ready(b).Reason).To(Equal(string(ReasonSyncedNotReady)))
		})

		It("surfaces the first failing current sub-condition", func() {
			b := newBinding("b", testRelease)
			b.Generation = 3
			withCondition(b, ConditionBackendsResolved, metav1.ConditionTrue, "Resolved", 2) // stale
			withCondition(b, ConditionSynced, metav1.ConditionFalse, string(ReasonEnvironmentNotFound), 3)

			setReadyCondition(b)

			Expect(ready(b).Status).To(Equal(metav1.ConditionFalse))
			Expect(ready(b).Reason).To(Equal(string(ReasonEnvironmentNotFound)))
		})
	})

	Describe("evaluateResourcesReady", func() {
		renderedWith := func(generation int64, applied *metav1.Condition) *openchoreov1alpha1.RenderedRelease {
			rr := &openchoreov1alpha1.RenderedRelease{
				ObjectMeta: metav1.ObjectMeta{Name: "rr", Generation: generation},
				Spec: openchoreov1alpha1.RenderedReleaseSpec{
					Resources: []openchoreov1alpha1.RenderedManifest{{ID: "route"}},
				},
				Status: openchoreov1alpha1.RenderedReleaseStatus{
					Resources: []openchoreov1alpha1.RenderedManifestStatus{{
						ID: "route", Kind: "HTTPRoute", HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
					}},
				},
			}
			if applied != nil {
				rr.Status.Conditions = []metav1.Condition{*applied}
			}
			return rr
		}
		appliedAt := func(status metav1.ConditionStatus, generation int64) *metav1.Condition {
			return &metav1.Condition{
				Type: renderedrelease.ConditionResourcesApplied, Status: status,
				Reason: "r", Message: "apply failed", ObservedGeneration: generation,
			}
		}
		resourcesReady := func(rr *openchoreov1alpha1.RenderedRelease) *metav1.Condition {
			b := newBinding("b", testRelease)
			evaluateResourcesReady(b, rr)
			return meta.FindStatusCondition(b.Status.Conditions, string(ConditionResourcesReady))
		}

		It("is ready once the current generation is applied and healthy", func() {
			Expect(resourcesReady(renderedWith(4, appliedAt(metav1.ConditionTrue, 4))).Status).
				To(Equal(metav1.ConditionTrue))
		})

		It("is progressing while only an earlier generation has been applied", func() {
			cond := resourcesReady(renderedWith(4, appliedAt(metav1.ConditionTrue, 3)))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonResourcesProgressing)))
		})

		It("is progressing before any apply is reported", func() {
			cond := resourcesReady(renderedWith(1, nil))
			Expect(cond.Reason).To(Equal(string(ReasonResourcesProgressing)))
		})

		It("reports an apply failure of the current generation", func() {
			cond := resourcesReady(renderedWith(4, appliedAt(metav1.ConditionFalse, 4)))
			Expect(cond.Reason).To(Equal(string(ReasonResourceApplyFailed)))
			Expect(cond.Message).To(Equal("apply failed"))
		})
	})
})
