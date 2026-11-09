// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

// The Endpoint family end to end: a hand-authored Endpoint splitting traffic
// across two components renders a weighted HTTPRoute that the gateway serves,
// its binding reports per-backend resolution, and a backend that is not
// deployed in an environment blocks rendering there unless it is staged at
// weight 0.
var _ = Describe("Endpoint Routing", Ordered, Label("tier2"), func() {
	const (
		routeSelector = "openchoreo.dev/endpoint=" + endpointName
		pollInterval  = 2 * time.Second
	)

	var (
		dpDevNs string
		dpStgNs string
	)

	bindingName := func(env string) string { return endpointName + "-" + env }

	jsonpath := func(g Gomega, resource, name, path string) string {
		out, err := framework.KubectlGetJsonpath(kubeContext, cpNs, resource, name, path)
		g.Expect(err).NotTo(HaveOccurred(), "reading %s %s %s", resource, name, path)
		return out
	}

	// latestEndpointRelease waits for the Endpoint to cut a release whose name
	// differs from previous, and returns it.
	latestEndpointRelease := func(previous string) string {
		var name string
		Eventually(func(g Gomega) {
			name = jsonpath(g, "endpoints.openchoreo.dev", endpointName, "{.status.latestRelease.name}")
			g.Expect(name).NotTo(BeEmpty(), "Endpoint has not cut a release yet")
			g.Expect(name).NotTo(Equal(previous), "Endpoint has not cut a new release yet")
		}, 2*time.Minute, pollInterval).Should(Succeed())
		return name
	}

	componentRelease := func(component string) string {
		var name string
		Eventually(func(g Gomega) {
			name = jsonpath(g, "component", component, "{.status.latestRelease.name}")
			g.Expect(name).NotTo(BeEmpty(), "%s has not cut a ComponentRelease yet", component)
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		return name
	}

	condition := func(g Gomega, binding, condType, field string) string {
		return jsonpath(g, "endpointreleasebinding", binding,
			fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].%s}`, condType, field))
	}

	waitBindingReady := func(env string) {
		Eventually(func(g Gomega) {
			g.Expect(condition(g, bindingName(env), "Ready", "status")).To(Equal("True"),
				"binding %s not Ready: %s", bindingName(env), condition(g, bindingName(env), "Ready", "message"))
		}, 3*time.Minute, pollInterval).Should(Succeed())
	}

	// routeBackendRefs returns the rendered route's backendRefs as
	// "service:weight" pairs.
	routeBackendRefs := func(g Gomega, dpNs string) []string {
		names, err := framework.GetHTTPRouteNames(kubeContext, dpNs, routeSelector)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(HaveLen(1), "expected exactly one HTTPRoute for the Endpoint in %s", dpNs)
		out, err := framework.KubectlGetJsonpath(kubeContext, dpNs, "httproute.gateway.networking.k8s.io", names[0],
			`{range .spec.rules[0].backendRefs[*]}{.name}:{.weight}{" "}{end}`)
		g.Expect(err).NotTo(HaveOccurred())
		return strings.Fields(out)
	}

	BeforeAll(func() {
		By("creating the control-plane namespace and platform resources")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		By("deploying the two backend components to development")
		for _, comp := range []string{compStable, compCanary} {
			output, err = framework.KubectlApplyLiteral(kubeContext, backendComponentYAML(comp))
			Expect(err).NotTo(HaveOccurred(), "failed to create %s: %s", comp, output)
		}

		By("discovering the data-plane namespaces")
		Eventually(func() error {
			var discoverErr error
			dpDevNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for development not found")
		Eventually(func() error {
			var discoverErr error
			dpStgNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envStaging)
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for staging not found")

		By("deploying a tester pod in the development data-plane namespace")
		output, err = framework.KubectlApplyLiteral(kubeContext, testerPodYAML(dpDevNs))
		Expect(err).NotTo(HaveOccurred(), "failed to create tester pod: %s", output)

		By("waiting for both backends to be deployed in development")
		for _, comp := range []string{compStable, compCanary} {
			framework.WaitForReleaseBindingReady(kubeContext, cpNs, comp+"-"+envDev)
		}
		Eventually(func(g Gomega) {
			framework.AssertAllPodsRunning(g, kubeContext, dpDevNs)
		}, 3*time.Minute, pollInterval).Should(Succeed(), "pods in %s not running in time", dpDevNs)

		By("promoting only the stable backend to staging")
		output, err = framework.KubectlApplyLiteral(kubeContext,
			releaseBindingYAML(compStable, componentRelease(compStable), envStaging))
		Expect(err).NotTo(HaveOccurred(), "failed to promote %s: %s", compStable, output)
		framework.WaitForReleaseBindingReady(kubeContext, cpNs, compStable+"-"+envStaging)

		By("applying the ClusterEndpointType and an Endpoint splitting 50/50")
		output, err = framework.KubectlApplyLiteral(kubeContext, clusterEndpointTypeYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create ClusterEndpointType: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext, endpointYAML(50, 50))
		Expect(err).NotTo(HaveOccurred(), "failed to create Endpoint: %s", output)
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}
		By("cleaning up")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")
		_, _ = framework.Kubectl(kubeContext, "delete", "clusterendpointtype", endpointTypeName, "--ignore-not-found", "--wait=false")
		for _, ns := range []string{dpDevNs, dpStgNs} {
			if ns != "" {
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
			}
		}
	})

	It("renders a weighted HTTPRoute the gateway serves when every backend is deployed", func() {
		release := latestEndpointRelease("")

		By("binding the release to development")
		output, err := framework.KubectlApplyLiteral(kubeContext, endpointReleaseBindingYAML(envDev, release))
		Expect(err).NotTo(HaveOccurred(), "failed to create binding: %s", output)
		waitBindingReady(envDev)

		By("reporting both backends resolved")
		Eventually(func(g Gomega) {
			for _, backend := range []string{"stable", "canary"} {
				g.Expect(jsonpath(g, "endpointreleasebinding", bindingName(envDev),
					fmt.Sprintf(`{.status.backends[?(@.name=="%s")].resolved}`, backend))).To(Equal("true"),
					"backend %s should be resolved", backend)
			}
		}, time.Minute, pollInterval).Should(Succeed())

		By("rendering one HTTPRoute with both backends at weight 50")
		var routeName string
		Eventually(func(g Gomega) {
			refs := routeBackendRefs(g, dpDevNs)
			g.Expect(refs).To(HaveLen(2))
			for _, ref := range refs {
				g.Expect(ref).To(HaveSuffix(":50"), "backendRef %s should carry weight 50", ref)
			}
			names, err := framework.GetHTTPRouteNames(kubeContext, dpDevNs, routeSelector)
			g.Expect(err).NotTo(HaveOccurred())
			routeName = names[0]
		}, 2*time.Minute, pollInterval).Should(Succeed())
		Eventually(func(g Gomega) {
			framework.AssertHTTPRouteAccepted(g, kubeContext, dpDevNs, routeName)
		}, 2*time.Minute, pollInterval).Should(Succeed())

		By("serving traffic from both backends through the gateway")
		url := routeURL(envDev)
		seen := map[string]bool{}
		Eventually(func(g Gomega) {
			out, err := framework.KubectlExecByLabel(kubeContext, dpDevNs, testerLabel, testerContainer,
				"wget", "-q", "-O", "-", "-T", "5", url)
			g.Expect(err).NotTo(HaveOccurred(), "request to %s failed", url)
			seen[strings.TrimSpace(out)] = true
			g.Expect(seen).To(HaveKey(compStable), "no response from %s yet (seen %v)", compStable, seen)
			g.Expect(seen).To(HaveKey(compCanary), "no response from %s yet (seen %v)", compCanary, seen)
		}, 2*time.Minute, 500*time.Millisecond).Should(Succeed())
	})

	It("blocks rendering with a readable reason when a weighted backend is not deployed", func() {
		release := latestEndpointRelease("")

		By("binding the same release to staging, where the canary is not deployed")
		output, err := framework.KubectlApplyLiteral(kubeContext, endpointReleaseBindingYAML(envStaging, release))
		Expect(err).NotTo(HaveOccurred(), "failed to create binding: %s", output)

		Eventually(func(g Gomega) {
			g.Expect(condition(g, bindingName(envStaging), "BackendsResolved", "status")).To(Equal("False"))
			g.Expect(condition(g, bindingName(envStaging), "Ready", "reason")).To(Equal("BackendsUnresolved"))
			g.Expect(condition(g, bindingName(envStaging), "Ready", "message")).To(ContainSubstring(
				fmt.Sprintf(`canary (component %q is not deployed in environment %q)`, compCanary, envStaging)))
			g.Expect(jsonpath(g, "endpointreleasebinding", bindingName(envStaging),
				`{.status.backends[?(@.name=="canary")].reason}`)).To(Equal("NotDeployed"))
			g.Expect(jsonpath(g, "endpointreleasebinding", bindingName(envStaging),
				`{.status.backends[?(@.name=="stable")].resolved}`)).To(Equal("true"))
		}, 2*time.Minute, pollInterval).Should(Succeed())

		By("rendering no route in staging")
		Consistently(func(g Gomega) {
			names, err := framework.GetHTTPRouteNames(kubeContext, dpStgNs, routeSelector)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(names).To(BeEmpty(), "no HTTPRoute should be rendered while a weighted backend is missing")
		}, 15*time.Second, pollInterval).Should(Succeed())
	})

	It("renders once the missing backend is staged at weight 0", func() {
		previous := latestEndpointRelease("")

		By("setting the canary's weight to 0, which cuts a new release")
		output, err := framework.KubectlApplyLiteral(kubeContext, endpointYAML(100, 0))
		Expect(err).NotTo(HaveOccurred(), "failed to update Endpoint: %s", output)
		release := latestEndpointRelease(previous)

		By("pinning the staging binding to the new release")
		output, err = framework.KubectlApplyLiteral(kubeContext, endpointReleaseBindingYAML(envStaging, release))
		Expect(err).NotTo(HaveOccurred(), "failed to update binding: %s", output)
		waitBindingReady(envStaging)

		By("still reporting the staged canary as not deployed")
		Eventually(func(g Gomega) {
			g.Expect(jsonpath(g, "endpointreleasebinding", bindingName(envStaging),
				`{.status.backends[?(@.name=="canary")].reason}`)).To(Equal("NotDeployed"))
		}, time.Minute, pollInterval).Should(Succeed())

		By("rendering a route in staging that sends traffic to the stable backend only")
		Eventually(func(g Gomega) {
			refs := routeBackendRefs(g, dpStgNs)
			g.Expect(refs).To(HaveLen(1))
			g.Expect(refs[0]).To(HaveSuffix(":100"))
		}, 2*time.Minute, pollInterval).Should(Succeed())
	})
})
