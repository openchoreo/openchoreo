// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("matchProblems", func() {
	withMatch := func(name string, m openchoreov1alpha1.EndpointMatch) openchoreov1alpha1.EndpointBackendRef {
		ref := componentBackend(name, name, "http", 0)
		ref.Match = &m
		return ref
	}
	endpointWith := func(refs ...openchoreov1alpha1.EndpointBackendRef) *openchoreov1alpha1.Endpoint {
		all := append([]openchoreov1alpha1.EndpointBackendRef{componentBackend("stable", "greeter", "http", 100)}, refs...)
		return newEndpoint("ep", "p", clusterTypeRef("cet"), all...)
	}
	messages := func(ep *openchoreov1alpha1.Endpoint) []string {
		problems := matchProblems(ep)
		out := make([]string, 0, len(problems))
		for _, p := range problems {
			Expect(p.reason).To(Equal(ReasonInvalidMatch))
			out = append(out, p.message)
		}
		return out
	}

	It("accepts header, path and cookie matches the data planes agree on", func() {
		Expect(messages(endpointWith(
			withMatch("testers", openchoreov1alpha1.EndpointMatch{
				Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}},
			}),
			withMatch("preview", openchoreov1alpha1.EndpointMatch{
				Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/next"},
			}),
			withMatch("group-b", openchoreov1alpha1.EndpointMatch{
				Cookies: []openchoreov1alpha1.EndpointCookieMatch{{Name: "exp", Value: "b"}},
			}),
		))).To(BeEmpty())
	})

	It("refuses a header twice, a cookie next to a Cookie header, and a bad regex", func() {
		Expect(messages(endpointWith(withMatch("canary", openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{
				{Name: "X-Canary", Value: "true"},
				{Name: "x-canary", Value: "yes"},
				{Name: "Cookie", Value: "a=b"},
				{Name: "user-agent", Type: openchoreov1alpha1.EndpointHeaderMatchRegularExpression, Value: "(?=Mobi)"},
			},
			Cookies: []openchoreov1alpha1.EndpointCookieMatch{{Name: "exp", Value: "b"}},
		})))).To(ConsistOf(
			ContainSubstring(`header "x-canary" appears more than once`),
			ContainSubstring("not both"),
			ContainSubstring("RE2"),
		))
	})

	It("refuses . and .. path segments and two backends with the same match", func() {
		Expect(messages(endpointWith(
			withMatch("a", openchoreov1alpha1.EndpointMatch{
				Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/next/../admin"},
			}),
			withMatch("b", openchoreov1alpha1.EndpointMatch{
				Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "X-Beta", Value: "1"}},
			}),
			withMatch("c", openchoreov1alpha1.EndpointMatch{
				Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-beta", Value: "1"}},
			}),
		))).To(ConsistOf(
			ContainSubstring(". or .. segments"),
			ContainSubstring(`backend "b" has the same match`),
		))
	})

	It("treats a path prefix with and without a trailing slash as the same match", func() {
		Expect(messages(endpointWith(
			withMatch("a", openchoreov1alpha1.EndpointMatch{
				Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/next"},
			}),
			withMatch("b", openchoreov1alpha1.EndpointMatch{
				Path: &openchoreov1alpha1.EndpointPathMatch{Value: "/next/"},
			}),
		))).To(ConsistOf(ContainSubstring(`backend "a" has the same match`)))
	})
})

var _ = Describe("backend names", func() {
	It("refuses a matched backend named default, the weighted rule's name", func(ctx SpecContext) {
		canary := componentBackend("default", "greeter-v2", "http", 0)
		canary.Match = &openchoreov1alpha1.EndpointMatch{
			Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}},
		}
		ep := newEndpoint("default-rule", "p", clusterTypeRef("cet"), componentBackend("stable", "greeter", "http", 100), canary)
		Expect(k8sClient.Create(ctx, ep)).To(MatchError(ContainSubstring("default names the rule")))
	})

	It("accepts a plain weighted backend named default", func(ctx SpecContext) {
		ep := newEndpoint("default-plain", "p", clusterTypeRef("cet"), componentBackend("default", "greeter", "http", 100))
		Expect(k8sClient.Create(ctx, ep)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			ep.Finalizers = nil
			Expect(client.IgnoreNotFound(k8sClient.Update(ctx, ep))).To(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ep))).To(Succeed())
		})
	})
})
