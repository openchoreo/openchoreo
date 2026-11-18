// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

// Endpoint rollouts end to end through occ and openchoreo-api. A canary
// starts at 0% with a testers' header, each step writes the development
// binding's backendOverrides and the gateway's route follows, a stale step is
// refused, abort goes back to stable, and finish leaves the new version alone.
// On a second Endpoint, an A/B test routes by cookie and a blue/green
// previews by header and switches. The type renders ${rules}, so header and
// cookie matches reach the gateway as route rules.
var _ = Describe("Endpoint Rollout", Ordered, Label("tier2"), func() {
	const pollInterval = 2 * time.Second

	var dpDevNs string

	bindingName := endpointName + "-" + envDev

	jsonpath := func(g Gomega, resource, name, path string) string {
		out, err := framework.KubectlGetJsonpath(kubeContext, cpNs, resource, name, path)
		g.Expect(err).NotTo(HaveOccurred(), "reading %s %s %s", resource, name, path)
		return out
	}

	rollout := func(args ...string) (string, error) {
		stdout, _, err := occ.Run(append([]string{"endpoint", "rollout"}, append(args, "--namespace", cpNs)...)...)
		return stdout, err
	}

	// routeRules is an Endpoint's rendered route in development: for each
	// rule, the header names it matches on and its backend weights by
	// Service, which is named after the backend's component.
	type routeRule struct {
		headers string
		weights map[string]int
	}
	routeRules := func(g Gomega, endpoint string) []routeRule {
		names, err := framework.GetHTTPRouteNames(kubeContext, dpDevNs, "openchoreo.dev/endpoint="+endpoint)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(HaveLen(1), "expected exactly one HTTPRoute for %s in %s", endpoint, dpDevNs)
		out, err := framework.KubectlGetJsonpath(kubeContext, dpDevNs, "httproute.gateway.networking.k8s.io", names[0],
			`{range .spec.rules[*]}{.matches[0].headers[*].name}|{range .backendRefs[*]}{.name}:{.weight} {end};{end}`)
		g.Expect(err).NotTo(HaveOccurred())
		var rules []routeRule
		for _, raw := range strings.Split(strings.TrimSuffix(out, ";"), ";") {
			headers, refs, _ := strings.Cut(raw, "|")
			rule := routeRule{headers: headers, weights: map[string]int{}}
			for _, ref := range strings.Fields(refs) {
				name, w, _ := strings.Cut(ref, ":")
				weight, err := strconv.Atoi(w)
				g.Expect(err).NotTo(HaveOccurred(), "weight of %s", ref)
				rule.weights[name] = weight
			}
			rules = append(rules, rule)
		}
		return rules
	}

	// routeWeights is the default rule's weights: the rule without header
	// matches, which splits every other request.
	routeWeights := func(g Gomega, endpoint string) map[string]int {
		for _, rule := range routeRules(g, endpoint) {
			if rule.headers == "" {
				return rule.weights
			}
		}
		g.Expect("").To(Equal("a default rule"), "route of %s has no rule without header matches", endpoint)
		return nil
	}

	expectRoute := func(endpoint string, want map[string]int) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			g.Expect(routeWeights(g, endpoint)).To(Equal(want))
		}, 2*time.Minute, pollInterval).Should(Succeed())
	}

	expectRuleCount := func(endpoint string, want int) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			g.Expect(routeRules(g, endpoint)).To(HaveLen(want))
		}, 2*time.Minute, pollInterval).Should(Succeed())
	}

	// expectAnswer waits until a request through the gateway, sent with the
	// given headers from the tester pod, is answered by component want.
	expectAnswer := func(endpoint, want string, headers ...string) {
		GinkgoHelper()
		cmd := []string{"wget", "-q", "-O", "-", "-T", "5"}
		for _, h := range headers {
			cmd = append(cmd, "--header", h)
		}
		cmd = append(cmd, routeURL(endpoint))
		Eventually(func(g Gomega) {
			out, err := framework.KubectlExecByLabel(kubeContext, dpDevNs, testerLabel, testerContainer, cmd...)
			g.Expect(err).NotTo(HaveOccurred(), "request to %s with %v failed", routeURL(endpoint), headers)
			g.Expect(strings.TrimSpace(out)).To(Equal(want), "answer to %v", headers)
		}, 2*time.Minute, time.Second).Should(Succeed())
	}

	// servedWeight is what an Endpoint's development binding reports a
	// backend gets. Status omits a weight of 0, so a reported backend without
	// one has 0.
	servedWeight := func(g Gomega, endpoint, backend string) string {
		binding := endpoint + "-" + envDev
		entry := fmt.Sprintf(`{.status.backends[?(@.name=="%s")]`, backend)
		g.Expect(jsonpath(g, "endpointreleasebinding", binding, entry+".name}")).To(Equal(backend))
		if w := jsonpath(g, "endpointreleasebinding", binding, entry+".weight}"); w != "" {
			return w
		}
		return "0"
	}

	BeforeAll(func() {
		By("creating the control-plane namespace and platform resources")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		By("deploying both versions to development")
		for _, comp := range []string{compStable, compCanary} {
			output, err = framework.KubectlApplyLiteral(kubeContext, backendComponentYAML(comp))
			Expect(err).NotTo(HaveOccurred(), "failed to create %s: %s", comp, output)
		}
		Eventually(func() error {
			var discoverErr error
			dpDevNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for development not found")
		output, err = framework.KubectlApplyLiteral(kubeContext, testerPodYAML(dpDevNs))
		Expect(err).NotTo(HaveOccurred(), "failed to create tester pod: %s", output)
		for _, comp := range []string{compStable, compCanary} {
			framework.WaitForReleaseBindingReady(kubeContext, cpNs, comp+"-"+envDev)
		}
		Eventually(func(g Gomega) {
			framework.AssertAllPodsRunning(g, kubeContext, dpDevNs)
		}, 3*time.Minute, pollInterval).Should(Succeed(), "pods in %s not running in time", dpDevNs)

		By("applying the ClusterEndpointType and two Endpoints serving the stable version")
		output, err = framework.KubectlApplyLiteral(kubeContext, clusterEndpointTypeYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create ClusterEndpointType: %s", output)
		for _, ep := range []string{endpointName, endpointNameAB} {
			output, err = framework.KubectlApplyLiteral(kubeContext, endpointYAML(ep))
			Expect(err).NotTo(HaveOccurred(), "failed to create Endpoint %s: %s", ep, output)
		}
		expectRoute(endpointName, map[string]int{compStable: 100})
		expectRoute(endpointNameAB, map[string]int{compStable: 100})
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}
		By("cleaning up")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")
		_, _ = framework.Kubectl(kubeContext, "delete", "clusterendpointtype", endpointTypeName, "--ignore-not-found", "--wait=false")
		if dpDevNs != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", dpDevNs, "--ignore-not-found", "--wait=false")
		}
	})

	It("refuses a plan its EndpointType can't render", func() {
		_, err := rollout("start", endpointName, "--strategy", "mirror", "--stable", "stable",
			"--candidate", "shadow="+compCanary+"/http", "--steps", "100", "--dry-run")
		Expect(err).To(MatchError(ContainSubstring("Mirror capability")))
		Expect(jsonpath(Default, "endpoints.openchoreo.dev", endpointName, "{.spec.rollout}")).To(BeEmpty())
	})

	It("starts a canary with the new version at 0%", func() {
		_, err := rollout("start", endpointName, "--strategy", "canary", "--stable", "stable",
			"--candidate", "canary="+compCanary+"/http", "--steps", "25,100", "--header", "x-canary=true")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			g.Expect(jsonpath(g, "endpoints.openchoreo.dev", endpointName, "{.spec.rollout.strategy}")).To(Equal("canary"))
		}, time.Minute, pollInterval).Should(Succeed())

		By("autoDeploy moving development to the release with both versions")
		expectRoute(endpointName, map[string]int{compStable: 100})
		expectRuleCount(endpointName, 2)
		_, err = rollout("status", endpointName, "--env", envDev, "--timeout", "2m")
		Expect(err).NotTo(HaveOccurred())
	})

	It("lets testers reach the new version at 0% by header", func() {
		expectAnswer(endpointName, compCanary, "x-canary: true")
		expectAnswer(endpointName, compStable)
	})

	It("moves 25% to the new version on the first step", func() {
		_, err := rollout("step", endpointName, "--env", envDev, "--expect-step", "0")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			g.Expect(jsonpath(g, "endpointreleasebinding", bindingName,
				`{.spec.backendOverrides[?(@.name=="canary")].weight}`)).To(Equal("25"))
		}, time.Minute, pollInterval).Should(Succeed())
		expectRoute(endpointName, map[string]int{compStable: 75, compCanary: 25})
		_, err = rollout("status", endpointName, "--env", envDev, "--timeout", "2m")
		Expect(err).NotTo(HaveOccurred())
	})

	It("refuses a step from a step the environment has left", func() {
		_, err := rollout("step", endpointName, "--env", envDev, "--expect-step", "0")
		Expect(err).To(MatchError(ContainSubstring("is at step 1")))
		Consistently(func(g Gomega) {
			g.Expect(routeWeights(g, endpointName)).To(Equal(map[string]int{compStable: 75, compCanary: 25}))
		}, 10*time.Second, pollInterval).Should(Succeed())
	})

	It("refuses to finish while the version that leaves still gets traffic", func() {
		_, err := rollout("finish", endpointName, "--keep", "canary")
		Expect(err).To(MatchError(ContainSubstring("still gets traffic")))
		Eventually(func(g Gomega) {
			g.Expect(jsonpath(g, "endpoints.openchoreo.dev", endpointName, "{.spec.rollout.strategy}")).To(Equal("canary"))
		}, time.Minute, pollInterval).Should(Succeed())
	})

	It("sends all traffic back to stable on abort", func() {
		_, err := rollout("abort", endpointName, "--env", envDev)
		Expect(err).NotTo(HaveOccurred())
		expectRoute(endpointName, map[string]int{compStable: 100})
	})

	It("ships the new version: last step, then finish", func() {
		// A step forward waits for the binding to report the abort.
		_, err := rollout("status", endpointName, "--env", envDev, "--timeout", "2m")
		Expect(err).NotTo(HaveOccurred())
		_, err = rollout("step", endpointName, "--env", envDev, "--last")
		Expect(err).NotTo(HaveOccurred())
		expectRoute(endpointName, map[string]int{compCanary: 100})
		Eventually(func(g Gomega) {
			g.Expect(servedWeight(g, endpointName, "stable")).To(Equal("0"))
		}, time.Minute, pollInterval).Should(Succeed())

		_, err = rollout("finish", endpointName, "--keep", "canary")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			g.Expect(jsonpath(g, "endpoints.openchoreo.dev", endpointName, "{.spec.rollout}")).To(BeEmpty())
			g.Expect(jsonpath(g, "endpoints.openchoreo.dev", endpointName, "{.spec.backendRefs[*].name}")).To(Equal("canary"))
		}, time.Minute, pollInterval).Should(Succeed())

		By("autoDeploy moving development to the release with the new version alone and no match")
		expectRuleCount(endpointName, 1)
		Eventually(func(g Gomega) {
			weights := routeWeights(g, endpointName)
			g.Expect(weights).To(HaveLen(1))
			g.Expect(weights).To(HaveKey(compCanary))
		}, 2*time.Minute, pollInterval).Should(Succeed())

		By("serving the new version through the gateway")
		expectAnswer(endpointName, compCanary)
	})

	It("routes an A/B test's cookie group to the new version", func() {
		_, err := rollout("start", endpointNameAB, "--strategy", "abTest", "--stable", "stable",
			"--candidate", "b="+compCanary+"/http", "--cookie", "exp=b")
		Expect(err).NotTo(HaveOccurred())
		expectRuleCount(endpointNameAB, 2)

		expectAnswer(endpointNameAB, compCanary, "Cookie: exp=b")
		expectAnswer(endpointNameAB, compCanary, "Cookie: a=1; exp=b; z=2")
		expectAnswer(endpointNameAB, compStable, "Cookie: exp=bb")
		expectAnswer(endpointNameAB, compStable)

		By("finishing the test keeping stable")
		_, err = rollout("finish", endpointNameAB, "--keep", "stable")
		Expect(err).NotTo(HaveOccurred())
		expectRuleCount(endpointNameAB, 1)
		expectAnswer(endpointNameAB, compStable, "Cookie: exp=b")
	})

	It("previews a blue/green by header, switches and switches back", func() {
		_, err := rollout("start", endpointNameAB, "--strategy", "blueGreen", "--stable", "stable",
			"--candidate", "green="+compCanary+"/http", "--header", "x-preview=green")
		Expect(err).NotTo(HaveOccurred())
		expectRuleCount(endpointNameAB, 2)
		expectAnswer(endpointNameAB, compCanary, "x-preview: green")
		expectAnswer(endpointNameAB, compStable)
		// The route is written before the binding reports green resolved.
		_, err = rollout("status", endpointNameAB, "--env", envDev, "--timeout", "2m")
		Expect(err).NotTo(HaveOccurred())

		By("switching all traffic to the new version")
		_, err = rollout("step", endpointNameAB, "--env", envDev)
		Expect(err).NotTo(HaveOccurred())
		expectRoute(endpointNameAB, map[string]int{compCanary: 100})
		expectAnswer(endpointNameAB, compCanary)

		By("switching back on abort")
		_, err = rollout("abort", endpointNameAB, "--env", envDev)
		Expect(err).NotTo(HaveOccurred())
		expectRoute(endpointNameAB, map[string]int{compStable: 100})
		expectAnswer(endpointNameAB, compStable)
		// finish reads the binding's status, written after the route.
		Eventually(func(g Gomega) {
			g.Expect(servedWeight(g, endpointNameAB, "green")).To(Equal("0"))
		}, time.Minute, pollInterval).Should(Succeed())

		_, err = rollout("finish", endpointNameAB, "--keep", "stable")
		Expect(err).NotTo(HaveOccurred())
		expectRuleCount(endpointNameAB, 1)
	})
})
