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
	resolved := func(names ...string) []endpointpipeline.Backend {
		out := make([]endpointpipeline.Backend, 0, len(names))
		for i, n := range names {
			out = append(out, endpointpipeline.Backend{Name: n, ServiceName: n + "-svc", Weight: int32(100 - i*100)})
		}
		return out
	}

	It("puts one rule per matched backend before the weighted default rule", func() {
		canary := componentBackend("canary", "greeter-v2", 0)
		canary.Match = &openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "X-Canary", Value: "true"}},
		}
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), canary)

		rules := buildRules(release, resolved("stable", "canary"))

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

		rules := buildRules(release, resolved("stable"))

		Expect(rules).To(HaveLen(1))
		Expect(rules[0].Name).To(Equal(defaultRuleName))
	})
})
