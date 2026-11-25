// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
)

var _ = Describe("buildRules", func() {
	// rulesFor builds the rules with the named backends resolved, and each
	// backend's effective mirror and match as resolveBackends sets them.
	rulesFor := func(
		b *openchoreov1alpha1.EndpointReleaseBinding, release *openchoreov1alpha1.EndpointRelease, names ...string,
	) []endpointpipeline.Rule {
		res := &resolvedBackends{matches: map[string]*openchoreov1alpha1.EndpointMatch{}}
		for _, ref := range release.Spec.BackendRefs {
			res.statuses = append(res.statuses, openchoreov1alpha1.ResolvedEndpointBackend{
				Name: ref.Name, Mirror: effectiveMirror(b, ref),
			})
			if m := effectiveMatch(b, ref); m != nil {
				res.matches[ref.Name] = m
			}
		}
		for i, n := range names {
			res.backends = append(res.backends, endpointpipeline.Backend{Name: n, ServiceName: n + "-svc", Weight: int32(100 - i*100)})
		}
		return buildRules(release, res)
	}

	It("puts one rule per matched backend before the weighted default rule", func() {
		canary := componentBackend("canary", "greeter-v2", 0)
		canary.Match = &openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "X-Canary", Value: "true"}},
		}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), canary)

		rules := rulesFor(newBinding("rules", testRelease), release, "stable", "canary")

		Expect(rules).To(HaveLen(2))
		Expect(rules[0].Name).To(Equal("canary"))
		Expect(rules[0].Backends).To(ConsistOf(And(HaveField("Name", "canary"), HaveField("Weight", int32(1)), HaveField("Percent", int32(100)))),
			"a match sends every request it matches, whatever the weights")
		Expect(rules[0].Matches[0].Conditions).To(Equal([]endpointpipeline.HeaderMatch{
			{Name: "x-canary", Type: endpointpipeline.HeaderMatchExact, Value: "true"},
		}))
		Expect(rules[1].Name).To(Equal(defaultRuleName))
		Expect(rules[1].Matches).To(BeEmpty())
		Expect(rules[1].Backends).To(HaveLen(2))
	})

	It("leaves out a matched backend that is not resolved here", func() {
		preview := componentBackend("preview", "greeter-v2", 0)
		preview.Match = &openchoreov1alpha1.EndpointMatch{
			Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/next"},
		}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), preview)

		rules := rulesFor(newBinding("rules", testRelease), release, "stable")

		Expect(rules).To(HaveLen(1))
		Expect(rules[0].Name).To(Equal(defaultRuleName))
	})

	It("copies a share of the default rule's requests to a mirror, with the environment's share", func() {
		shadow := componentBackend("shadow", "greeter-v2", 0)
		shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 10}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), shadow)

		rules := rulesFor(newBinding("rules", testRelease), release, "stable", "shadow")
		Expect(rules).To(HaveLen(1))
		Expect(rules[0].Mirrors).To(ConsistOf(
			And(HaveField("Backend.Name", "shadow"), HaveField("Percent", int32(10)))))

		b := newBinding("rules", testRelease)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
			{Name: "shadow", Mirror: &openchoreov1alpha1.EndpointMirror{Percent: 100}},
		}
		rules = rulesFor(b, release, "stable", "shadow")
		Expect(rules[0].Mirrors).To(ConsistOf(HaveField("Percent", int32(100))))

		b.Spec.BackendOverrides[0].Mirror.Percent = 0
		Expect(rulesFor(b, release, "stable", "shadow")[0].Mirrors).To(BeEmpty(),
			"a mirror share of 0 stops copying")
	})

	It("ignores and reports a mirror override on a backend that is not a mirror", func() {
		// A mirror turned into a canary keeps its old mirror override.
		canary := componentBackend("canary", "greeter-v2", 0)
		canary.Match = &openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}},
		}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), canary)
		b := newBinding("rules", testRelease)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
			{Name: "canary", Mirror: &openchoreov1alpha1.EndpointMirror{Percent: 50}},
		}
		Expect(ignoredOverrides(release, b)).To(ContainSubstring("no mirror"))
		rules := rulesFor(b, release, "stable", "canary")
		Expect(rules[len(rules)-1].Mirrors).To(BeEmpty(), "the stale mirror share is ignored")
	})

	It("refuses an override mirror share the type doesn't declare", func() {
		shadow := componentBackend("shadow", "greeter-v2", 0)
		shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 0}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), shadow)
		release.Spec.EndpointType.Spec.Capabilities = []openchoreov1alpha1.EndpointCapability{
			openchoreov1alpha1.EndpointCapabilityMirror,
		}
		b := newBinding("rules", testRelease)
		Expect(overridesUnsupported(b, release)).To(BeEmpty(), "the release copies nothing")

		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
			{Name: "shadow", Mirror: &openchoreov1alpha1.EndpointMirror{Percent: 10}},
		}
		Expect(overridesUnsupported(b, release)).To(ConsistOf(ContainSubstring("MirrorPercent")))

		b.Spec.BackendOverrides[0].Mirror.Percent = 100
		Expect(overridesUnsupported(b, release)).To(BeEmpty(), "copying every request needs only Mirror")
	})

	It("keeps a mirror backend at weight 0 whatever an override says", func() {
		shadow := componentBackend("shadow", "greeter-v2", 0)
		shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 20}
		b := newBinding("rules", testRelease)
		weight := int32(50)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{Name: "shadow", Weight: &weight}}

		Expect(effectiveWeight(b, []openchoreov1alpha1.EndpointBackendRef{shadow}, shadow)).To(BeZero(), "a mirror only receives copies")
		Expect(effectiveMirror(b, shadow).Percent).To(Equal(int32(20)))
	})

	It("gives a backend the environment's own split doesn't name no share", func() {
		python := componentBackend("trips-python", "trips-python", 50)
		goRef := componentBackend("trips-go", "trips-go", 0)
		stale := componentBackend("trips-stale", "trips-stale", 50)
		refs := []openchoreov1alpha1.EndpointBackendRef{python, goRef, stale}
		b := newBinding("rules", testRelease)
		Expect(effectiveWeight(b, refs, stale)).To(Equal(int32(50)), "without an own split the release's weight applies")

		eighty, twenty := int32(80), int32(20)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
			{Name: "trips-python", Weight: &eighty},
			{Name: "trips-go", Weight: &twenty},
		}
		Expect(effectiveWeight(b, refs, python)).To(Equal(int32(80)))
		Expect(effectiveWeight(b, refs, goRef)).To(Equal(int32(20)))
		Expect(effectiveWeight(b, refs, stale)).To(BeZero(), "a backend new to the release joins the own split at 0")

		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{Name: "trips-java", Weight: &eighty}}
		Expect(effectiveWeight(b, refs, stale)).To(Equal(int32(50)), "an override for a backend the release lacks is no own split")
	})

	It("reports a weight override on a mirror backend as ignored", func() {
		shadow := componentBackend("shadow", "greeter-v2", 0)
		shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 20}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), shadow)
		b := newBinding("rules", testRelease)
		weight := int32(50)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{Name: "shadow", Weight: &weight}}

		Expect(ignoredOverrides(release, b)).To(ContainSubstring("weight set on mirror backend(s)"))
	})

	It("matches the environment's own condition instead of the release's", func() {
		preview := componentBackend("preview", "greeter-v2", 0)
		preview.Match = &openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-preview", Value: "true"}},
		}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), preview)
		b := newBinding("rules", testRelease)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{
			Name:  "preview",
			Match: &openchoreov1alpha1.EndpointMatch{Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/beta"}},
		}}

		rules := rulesFor(b, release, "stable", "preview")
		Expect(rules).To(HaveLen(2))
		Expect(rules[0].Name).To(Equal("preview"))
		Expect(rules[0].Matches[0].Conditions).To(BeEmpty(), "the release's header is replaced")
		Expect(rules[0].Matches[0].Path).NotTo(BeNil())
		Expect(rules[0].Matches[0].Path.Value).To(Equal("/beta"))
		Expect(ignoredOverrides(release, b)).To(BeEmpty())
	})

	It("ignores and reports a match override on a backend with no match", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), componentBackend("next", "greeter-v2", 0))
		b := newBinding("rules", testRelease)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{
			Name:  "next",
			Match: &openchoreov1alpha1.EndpointMatch{Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/beta"}},
		}}

		Expect(ignoredOverrides(release, b)).To(ContainSubstring("match set on backend(s) that have no match"))
		Expect(rulesFor(b, release, "stable", "next")).To(HaveLen(1), "next stays in the weighted split only")
	})

	It("refuses an override match the type doesn't declare", func() {
		preview := componentBackend("preview", "greeter-v2", 0)
		preview.Match = &openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-preview", Value: "true"}},
		}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), preview)
		release.Spec.EndpointType.Spec.Capabilities = []openchoreov1alpha1.EndpointCapability{
			openchoreov1alpha1.EndpointCapabilityMatch,
		}
		b := newBinding("rules", testRelease)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{
			Name:  "preview",
			Match: &openchoreov1alpha1.EndpointMatch{Cookies: []openchoreov1alpha1.EndpointCookieMatch{{Name: "group", Value: "beta"}}},
		}}

		Expect(overridesUnsupported(b, release)).To(ConsistOf(ContainSubstring("RegularExpression")))
	})

	It("runs a release's match checks on the environment's own matches", func() {
		preview := componentBackend("preview", "greeter-v2", 0)
		preview.Match = &openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-preview", Value: "true"}},
		}
		beta := componentBackend("beta", "greeter-v3", 0)
		beta.Match = &openchoreov1alpha1.EndpointMatch{Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/beta"}}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), preview, beta)
		b := newBinding("rules", testRelease)
		Expect(overrideMatchProblems(b, release)).To(BeEmpty(), "no match override, nothing to check")

		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{
			Name: "preview",
			Match: &openchoreov1alpha1.EndpointMatch{Headers: []openchoreov1alpha1.EndpointHeaderMatch{
				{Name: "x-preview", Type: openchoreov1alpha1.EndpointHeaderMatchRegularExpression, Value: "("},
			}},
		}}
		Expect(overrideMatchProblems(b, release)).To(ConsistOf(ContainSubstring("RE2")))

		b.Spec.BackendOverrides[0].Match = &openchoreov1alpha1.EndpointMatch{
			Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/beta"},
		}
		Expect(overrideMatchProblems(b, release)).To(ConsistOf(ContainSubstring("has the same match")))
	})

	It("reports every ignored field of an override", func() {
		shadow := componentBackend("shadow", "greeter-v2", 0)
		shadow.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: 20}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), shadow)
		b := newBinding("rules", testRelease)
		weight := int32(5)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{
			Name:   "shadow",
			Weight: &weight,
			Match:  &openchoreov1alpha1.EndpointMatch{Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/x"}},
		}}

		msg := ignoredOverrides(release, b)
		Expect(msg).To(ContainSubstring("weight set on mirror backend(s)"))
		Expect(msg).To(ContainSubstring("match set on backend(s) that have no match"))
	})
})
