// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func rolloutSteps(name string, percents ...int32) *openchoreov1alpha1.EndpointRolloutSteps {
	steps := &openchoreov1alpha1.EndpointRolloutSteps{}
	for _, p := range percents {
		steps.Steps = append(steps.Steps, openchoreov1alpha1.EndpointRolloutStep{
			Backends: []openchoreov1alpha1.EndpointRolloutStepBackend{{Name: name, Percent: p}},
		})
	}
	return steps
}

func canaryRollout(stable string, candidates ...string) *openchoreov1alpha1.EndpointRollout {
	r := &openchoreov1alpha1.EndpointRollout{Strategy: openchoreov1alpha1.EndpointRolloutCanary, Stable: stable}
	for _, c := range candidates {
		r.Candidates = append(r.Candidates, openchoreov1alpha1.EndpointRolloutCandidate{Name: c})
	}
	r.Canary = rolloutSteps(candidates[0], 5, 25, 100)
	return r
}

var _ = Describe("spec.rollout", func() {
	stableAndCandidate := func(name string) *openchoreov1alpha1.Endpoint {
		return newEndpoint(name, "p", clusterTypeRef("cet"),
			componentBackend("stable", "greeter", "http", 100),
			componentBackend("v2", "greeter-v2", "http", 0))
	}

	dryRun := func(ctx SpecContext, ep *openchoreov1alpha1.Endpoint) error {
		return k8sClient.Create(ctx, ep, client.DryRunAll)
	}

	It("accepts a canary plan", func(ctx SpecContext) {
		ep := stableAndCandidate("rollout-canary")
		ep.Spec.Rollout = canaryRollout("stable", "v2")
		Expect(dryRun(ctx, ep)).To(Succeed())
	})

	DescribeTable("refuses a plan that breaks a rule",
		func(ctx SpecContext, edit func(*openchoreov1alpha1.Endpoint), message string) {
			ep := stableAndCandidate("rollout-bad")
			ep.Spec.Rollout = canaryRollout("stable", "v2")
			edit(ep)
			Expect(dryRun(ctx, ep)).To(MatchError(ContainSubstring(message)))
		},
		Entry("an unknown stable", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.Rollout.Stable = "missing"
		}, "rollout.stable must name a backend"),
		Entry("an unknown candidate", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.Rollout.Candidates = []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "missing"}}
			ep.Spec.Rollout.Canary = rolloutSteps("missing", 5, 100)
		}, "every rollout candidate must name a backend"),
		Entry("stable as the candidate", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.Rollout.Candidates = []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "stable"}}
			ep.Spec.Rollout.Canary = rolloutSteps("stable", 5, 100)
		}, "can't also be stable"),
		Entry("a step naming a backend that isn't a candidate", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.Rollout.Canary = rolloutSteps("stable", 10)
		}, "a rollout step can only name candidates"),
		Entry("a canary without its steps", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.Rollout.Canary = nil
		}, "set canary for a canary rollout only"),
		Entry("blueGreen with canary steps", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.Rollout.Strategy = openchoreov1alpha1.EndpointRolloutBlueGreen
		}, "set canary for a canary rollout only"),
		Entry("blueGreen with two candidates", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.BackendRefs = append(ep.Spec.BackendRefs, componentBackend("v3", "greeter-v3", "http", 0))
			ep.Spec.Rollout.Strategy = openchoreov1alpha1.EndpointRolloutBlueGreen
			ep.Spec.Rollout.Canary = nil
			ep.Spec.Rollout.Candidates = append(ep.Spec.Rollout.Candidates, openchoreov1alpha1.EndpointRolloutCandidate{Name: "v3"})
		}, "must have at most 1 item"),
		Entry("a canary with two candidates", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.BackendRefs = append(ep.Spec.BackendRefs, componentBackend("v3", "greeter-v3", "http", 0))
			ep.Spec.Rollout.Candidates = append(ep.Spec.Rollout.Candidates, openchoreov1alpha1.EndpointRolloutCandidate{Name: "v3"})
		}, "must have at most 1 item"),
		Entry("an abTest with two groups", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.BackendRefs = append(ep.Spec.BackendRefs, componentBackend("v3", "greeter-v3", "http", 0))
			ep.Spec.Rollout.Strategy = openchoreov1alpha1.EndpointRolloutABTest
			ep.Spec.Rollout.Canary = nil
			ep.Spec.Rollout.Candidates = append(ep.Spec.Rollout.Candidates, openchoreov1alpha1.EndpointRolloutCandidate{Name: "v3"})
		}, "must have at most 1 item"),
		Entry("a mirror rollout to a candidate without a mirror", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.Rollout.Strategy = openchoreov1alpha1.EndpointRolloutMirror
			ep.Spec.Rollout.Mirror, ep.Spec.Rollout.Canary = ep.Spec.Rollout.Canary, nil
		}, "a mirror rollout needs a mirror on every candidate"),
		Entry("a canary to a mirror backend", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.BackendRefs[1].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 100}
		}, "only a mirror rollout can have a candidate with a mirror"),
		Entry("an abTest candidate without a cookie match", func(ep *openchoreov1alpha1.Endpoint) {
			ep.Spec.Rollout.Strategy = openchoreov1alpha1.EndpointRolloutABTest
			ep.Spec.Rollout.Canary = nil
			ep.Spec.BackendRefs[1].Match = &openchoreov1alpha1.EndpointMatch{
				Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}},
			}
		}, "needs a cookie match on every candidate"),
		Entry("a mirror backend as stable", func(ep *openchoreov1alpha1.Endpoint) {
			w := int32(0)
			ep.Spec.BackendRefs[0].Weight = &w
			ep.Spec.BackendRefs[0].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 100}
		}, "rollout.stable can't be a mirror backend"),
	)

	It("accepts mirror, blueGreen and abTest plans", func(ctx SpecContext) {
		mirror := stableAndCandidate("rollout-mirror")
		mirror.Spec.BackendRefs[1].Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 10}
		mirror.Spec.Rollout = &openchoreov1alpha1.EndpointRollout{
			Strategy: openchoreov1alpha1.EndpointRolloutMirror, Stable: "stable",
			Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "v2"}},
			Mirror:     rolloutSteps("v2", 10, 50, 100),
		}
		Expect(dryRun(ctx, mirror)).To(Succeed())

		blueGreen := stableAndCandidate("rollout-bluegreen")
		blueGreen.Spec.Rollout = &openchoreov1alpha1.EndpointRollout{
			Strategy: openchoreov1alpha1.EndpointRolloutBlueGreen, Stable: "stable",
			Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "v2"}},
		}
		Expect(dryRun(ctx, blueGreen)).To(Succeed())

		ab := stableAndCandidate("rollout-ab")
		ab.Spec.BackendRefs[1].Match = &openchoreov1alpha1.EndpointMatch{
			Cookies: []openchoreov1alpha1.EndpointCookieMatch{{Name: "group", Value: "b"}},
		}
		ab.Spec.Rollout = &openchoreov1alpha1.EndpointRollout{
			Strategy: openchoreov1alpha1.EndpointRolloutABTest, Stable: "stable",
			Candidates: []openchoreov1alpha1.EndpointRolloutCandidate{{Name: "v2"}},
		}
		Expect(dryRun(ctx, ab)).To(Succeed())
	})

	It("keeps the release when only the plan changes", func(ctx SpecContext) {
		reconciler := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		cet := newClusterEndpointType("cet-rollout", pathPrefixSchema)
		ep := newEndpoint("ep-rollout", "p-rollout", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 100),
			componentBackend("v2", "greeter-v2", "http", 0))
		create(cet, newWorkload("p-rollout", "greeter", "http"), newWorkload("p-rollout", "greeter-v2", "http"), ep)

		reconcileOnce := func() *openchoreov1alpha1.Endpoint {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ep)})
			Expect(err).NotTo(HaveOccurred())
			got := &openchoreov1alpha1.Endpoint{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), got)).To(Succeed())
			return got
		}

		first := reconcileOnce()
		Expect(first.Status.LatestRelease).NotTo(BeNil())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.EndpointRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: first.Status.LatestRelease.Name},
			})
		})

		first.Spec.Rollout = canaryRollout("stable", "v2")
		Expect(k8sClient.Update(ctx, first)).To(Succeed())
		Expect(reconcileOnce().Status.LatestRelease.Name).To(Equal(first.Status.LatestRelease.Name))
	})
})
