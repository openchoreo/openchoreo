// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var _ = Describe("Gateway Routing", Ordered, Label("tier2"), func() {
	var (
		dpDevNs string // data plane namespace for development
		dpStgNs string // data plane namespace for staging (test 6)
	)

	endpointExternalHTTPURL := func(component, endpoint, env string) string {
		rbName := component + "-" + env
		jp := func(g Gomega, field string) string {
			out, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "componentreleasebinding", rbName,
				fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].externalURLs.http.%s}`, endpoint, field),
			)
			g.Expect(err).NotTo(HaveOccurred())
			return out
		}
		var url string
		Eventually(func(g Gomega) {
			scheme := jp(g, "scheme")
			host := jp(g, "host")
			port := jp(g, "port")
			path := jp(g, "path")
			g.Expect(scheme).NotTo(BeEmpty(), "externalURLs.http.scheme empty on %s", rbName)
			g.Expect(host).NotTo(BeEmpty(), "externalURLs.http.host empty on %s", rbName)
			g.Expect(port).NotTo(BeEmpty(), "externalURLs.http.port empty on %s", rbName)
			url = fmt.Sprintf("%s://%s:%s%s", scheme, host, port, path)
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
		return url
	}

	endpointInternalHTTPURL := func(component, endpoint, env string) string {
		rbName := component + "-" + env
		jp := func(g Gomega, field string) string {
			out, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "componentreleasebinding", rbName,
				fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].internalURLs.http.%s}`, endpoint, field),
			)
			g.Expect(err).NotTo(HaveOccurred())
			return out
		}
		var url string
		Eventually(func(g Gomega) {
			scheme := jp(g, "scheme")
			host := jp(g, "host")
			port := jp(g, "port")
			path := jp(g, "path")
			g.Expect(scheme).NotTo(BeEmpty(), "internalURLs.http.scheme empty on %s", rbName)
			g.Expect(host).NotTo(BeEmpty(), "internalURLs.http.host empty on %s", rbName)
			g.Expect(port).NotTo(BeEmpty(), "internalURLs.http.port empty on %s", rbName)
			url = fmt.Sprintf("%s://%s:%s%s", scheme, host, port, path)
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
		return url
	}

	BeforeAll(func() {
		By("creating CP namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)

		By("applying platform resources")
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		By("deploying gw-ext (external visibility)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			"gw-ext", "hashicorp/http-echo", []string{"-text=gw-ext", "-listen=:8080"},
			map[string]endpointDef{
				"http": {epType: "HTTP", port: 8080, visibility: []string{"external"}},
			}, nil))
		Expect(err).NotTo(HaveOccurred(), "failed to create gw-ext: %s", output)

		By("deploying gw-proj (project-only visibility)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			"gw-proj", "hashicorp/http-echo", []string{"-text=gw-proj", "-listen=:8080"},
			map[string]endpointDef{
				"http": {epType: "HTTP", port: 8080, visibility: []string{"project"}},
			}, nil))
		Expect(err).NotTo(HaveOccurred(), "failed to create gw-proj: %s", output)

		By("deploying gw-int (internal visibility)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			"gw-int", "hashicorp/http-echo", []string{"-text=gw-int", "-listen=:8080"},
			map[string]endpointDef{
				"http": {epType: "HTTP", port: 8080, visibility: []string{"internal"}},
			}, nil))
		Expect(err).NotTo(HaveOccurred(), "failed to create gw-int: %s", output)

		By("deploying gw-multi (two external endpoints, same port)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			"gw-multi", "hashicorp/http-echo", []string{"-text=gw-multi", "-listen=:8080"},
			map[string]endpointDef{
				"api":   {epType: "HTTP", port: 8080, visibility: []string{"external"}},
				"admin": {epType: "HTTP", port: 8080, visibility: []string{"external"}},
			}, nil))
		Expect(err).NotTo(HaveOccurred(), "failed to create gw-multi: %s", output)

		By("deploying gw-base (external with basePath)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentYAML(
			"gw-base", "hashicorp/http-echo", []string{"-text=gw-base", "-listen=:8080"},
			map[string]endpointDef{
				"api": {epType: "HTTP", port: 8080, visibility: []string{"external"}, basePath: "/api/v1"},
			}, nil))
		Expect(err).NotTo(HaveOccurred(), "failed to create gw-base: %s", output)

		By("discovering development DP namespace")
		Eventually(func() error {
			var discoverErr error
			dpDevNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for development not found")
		fmt.Fprintf(GinkgoWriter, "discovered dev dp namespace: %s\n", dpDevNs)

		By("deploying tester pod in DP namespace")
		output, err = framework.KubectlApplyLiteral(kubeContext, testerPodYAML(dpDevNs))
		Expect(err).NotTo(HaveOccurred(), "failed to create tester pod: %s", output)

		By("waiting for all pods running in dev DP namespace")
		Eventually(func(g Gomega) {
			framework.AssertAllPodsRunning(g, kubeContext, dpDevNs)
		}, 3*time.Minute, 2*time.Second).Should(Succeed(),
			"pods in %s not running in time", dpDevNs)

		By("waiting for ComponentReleaseBindings to become Ready")
		for _, comp := range []string{"gw-ext", "gw-proj", "gw-int", "gw-multi", "gw-base"} {
			framework.WaitForComponentReleaseBindingReady(kubeContext, cpNs, comp+"-"+envDev)
		}

		By("promoting gw-ext to staging for environment override test")
		var compRelease string
		Eventually(func() error {
			var discoverErr error
			compRelease, discoverErr = framework.KubectlGetJsonpath(
				kubeContext, cpNs, "component", "gw-ext",
				"{.status.latestRelease.name}")
			if discoverErr != nil {
				return discoverErr
			}
			if compRelease == "" {
				return fmt.Errorf("gw-ext latestRelease.name not yet populated")
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "gw-ext ComponentRelease not created")

		output, err = framework.KubectlApplyLiteral(kubeContext, componentReleaseBindingYAML("gw-ext", compRelease, envStaging))
		Expect(err).NotTo(HaveOccurred(), "failed to create staging ComponentReleaseBinding: %s", output)

		By("discovering staging DP namespace")
		Eventually(func() error {
			var discoverErr error
			dpStgNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envStaging)
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for staging not found")
		fmt.Fprintf(GinkgoWriter, "discovered staging dp namespace: %s\n", dpStgNs)

		By("waiting for staging pods running")
		Eventually(func(g Gomega) {
			framework.AssertAllPodsRunning(g, kubeContext, dpStgNs)
		}, 3*time.Minute, 2*time.Second).Should(Succeed(),
			"pods in %s not running in time", dpStgNs)

		framework.WaitForComponentReleaseBindingReady(kubeContext, cpNs, "gw-ext-"+envStaging)
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}

		By("cleaning up CP namespace")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")

		By("cleaning up DP namespaces")
		for _, ns := range []string{dpDevNs, dpStgNs} {
			if ns != "" {
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
			}
		}
	})

	// ─── Visibility enforcement ─────────────────────────────────────────

	Context("Visibility enforcement", func() {
		It("routes external traffic through kgateway", func() {
			By("verifying external HTTPRoute exists for gw-ext")
			selector := "openchoreo.dev/component=gw-ext,openchoreo.dev/endpoint-visibility=external"
			Eventually(func(g Gomega) {
				count, err := framework.CountHTTPRoutesByLabel(kubeContext, dpDevNs, selector)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(1), "expected exactly 1 external HTTPRoute for gw-ext")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying HTTPRoute is accepted")
			names, err := framework.GetHTTPRouteNames(kubeContext, dpDevNs, selector)
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(HaveLen(1))
			Eventually(func(g Gomega) {
				framework.AssertHTTPRouteAccepted(g, kubeContext, dpDevNs, names[0])
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying traffic through external gateway")
			url := endpointExternalHTTPURL("gw-ext", "http", envDev)
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, dpDevNs, testerLabel, testerContainer, url, 10)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed(),
				"external URL %s should return 200", url)
		})

		It("does not create external HTTPRoute for project-only endpoint", func() {
			By("verifying no external HTTPRoute exists for gw-proj")
			selector := "openchoreo.dev/component=gw-proj,openchoreo.dev/endpoint-visibility=external"
			Consistently(func(g Gomega) {
				count, err := framework.CountHTTPRoutesByLabel(kubeContext, dpDevNs, selector)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(0), "project-only component should have no external HTTPRoute")
			}, 10*time.Second, 2*time.Second).Should(Succeed())

			By("verifying ComponentReleaseBinding has no externalURLs for gw-proj")
			rbName := "gw-proj-" + envDev
			externalScheme, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "componentreleasebinding", rbName,
				`{.status.endpoints[?(@.name=="http")].externalURLs.http.scheme}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(externalScheme).To(BeEmpty(), "project-only endpoint should have no externalURLs")
		})

		It("routes internal traffic through internal gateway only", func() {
			By("verifying internal HTTPRoute exists for gw-int")
			internalSelector := "openchoreo.dev/component=gw-int,openchoreo.dev/endpoint-visibility=internal"
			Eventually(func(g Gomega) {
				count, err := framework.CountHTTPRoutesByLabel(kubeContext, dpDevNs, internalSelector)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(1), "expected exactly 1 internal HTTPRoute for gw-int")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying no external HTTPRoute exists for gw-int")
			externalSelector := "openchoreo.dev/component=gw-int,openchoreo.dev/endpoint-visibility=external"
			count, err := framework.CountHTTPRoutesByLabel(kubeContext, dpDevNs, externalSelector)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0), "internal-only component should have no external HTTPRoute")

			By("verifying internal HTTPRoute is accepted")
			names, err := framework.GetHTTPRouteNames(kubeContext, dpDevNs, internalSelector)
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(HaveLen(1))
			Eventually(func(g Gomega) {
				framework.AssertHTTPRouteAccepted(g, kubeContext, dpDevNs, names[0])
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying traffic through internal gateway")
			url := endpointInternalHTTPURL("gw-int", "http", envDev)
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, dpDevNs, testerLabel, testerContainer, url, 10)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed(),
				"internal URL %s should return 200", url)

			By("verifying no externalURLs on ComponentReleaseBinding")
			rbName := "gw-int-" + envDev
			externalScheme, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "componentreleasebinding", rbName,
				`{.status.endpoints[?(@.name=="http")].externalURLs.http.scheme}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(externalScheme).To(BeEmpty(), "internal-only endpoint should have no externalURLs")
		})
	})

	// ─── Routing correctness ────────────────────────────────────────────

	Context("Routing correctness", func() {
		It("creates distinct HTTPRoutes for multiple endpoints on same component", func() {
			By("verifying two external HTTPRoutes exist for gw-multi")
			selector := "openchoreo.dev/component=gw-multi,openchoreo.dev/endpoint-visibility=external"
			Eventually(func(g Gomega) {
				count, err := framework.CountHTTPRoutesByLabel(kubeContext, dpDevNs, selector)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(2), "expected 2 external HTTPRoutes for gw-multi")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying each endpoint has its own HTTPRoute")
			for _, epName := range []string{"api", "admin"} {
				epSelector := fmt.Sprintf("openchoreo.dev/component=gw-multi,openchoreo.dev/endpoint-name=%s", epName)
				names, err := framework.GetHTTPRouteNames(kubeContext, dpDevNs, epSelector)
				Expect(err).NotTo(HaveOccurred())
				Expect(names).To(HaveLen(1), "expected 1 HTTPRoute for endpoint %s", epName)

				Eventually(func(g Gomega) {
					framework.AssertHTTPRouteAccepted(g, kubeContext, dpDevNs, names[0])
				}, 2*time.Minute, 2*time.Second).Should(Succeed(),
					"HTTPRoute for endpoint %s should be accepted", epName)
			}

			By("verifying both endpoints have externalURLs")
			for _, epName := range []string{"api", "admin"} {
				rbName := "gw-multi-" + envDev
				scheme, err := framework.KubectlGetJsonpath(
					kubeContext, cpNs, "componentreleasebinding", rbName,
					fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].externalURLs.http.scheme}`, epName))
				Expect(err).NotTo(HaveOccurred())
				Expect(scheme).NotTo(BeEmpty(), "endpoint %s should have externalURLs", epName)
			}
		})

		It("configures basePath URL rewrite on HTTPRoute", func() {
			By("finding the HTTPRoute for gw-base")
			selector := "openchoreo.dev/component=gw-base,openchoreo.dev/endpoint-name=api"
			var routeName string
			Eventually(func(g Gomega) {
				names, err := framework.GetHTTPRouteNames(kubeContext, dpDevNs, selector)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(names).To(HaveLen(1))
				routeName = names[0]
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying URLRewrite replacePrefixMatch is /api/v1")
			Eventually(func(g Gomega) {
				rewrite, err := framework.KubectlGetJsonpath(
					kubeContext, dpDevNs,
					"httproute.gateway.networking.k8s.io", routeName,
					`{.spec.rules[0].filters[0].urlRewrite.path.replacePrefixMatch}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rewrite).To(Equal("/api/v1"),
					"basePath should be configured as replacePrefixMatch")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying HTTPRoute is accepted and traffic succeeds")
			Eventually(func(g Gomega) {
				framework.AssertHTTPRouteAccepted(g, kubeContext, dpDevNs, routeName)
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			url := endpointExternalHTTPURL("gw-base", "api", envDev)
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, dpDevNs, testerLabel, testerContainer, url+"/", 10)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed(),
				"basePath URL %s should be routable", url)
		})
	})

	// ─── Gateway configuration ──────────────────────────────────────────

	Context("Gateway configuration", func() {
		It("uses Environment gateway override when specified", func() {
			By("finding the external HTTPRoute for gw-ext in staging DP namespace")
			selector := "openchoreo.dev/component=gw-ext,openchoreo.dev/endpoint-visibility=external"
			var routeName string
			Eventually(func(g Gomega) {
				names, err := framework.GetHTTPRouteNames(kubeContext, dpStgNs, selector)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(names).To(HaveLen(1))
				routeName = names[0]
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying parentRef points to the override gateway (gateway-internal)")
			Eventually(func(g Gomega) {
				parentName, err := framework.KubectlGetJsonpath(
					kubeContext, dpStgNs,
					"httproute.gateway.networking.k8s.io", routeName,
					`{.spec.parentRefs[0].name}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(parentName).To(Equal(gwInternalName),
					"staging HTTPRoute should reference the environment override gateway, not gateway-default")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying hostname uses the override host")
			Eventually(func(g Gomega) {
				hostname, err := framework.KubectlGetJsonpath(
					kubeContext, dpStgNs,
					"httproute.gateway.networking.k8s.io", routeName,
					`{.spec.hostnames[0]}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(hostname).To(ContainSubstring(stagingGWHost),
					"staging HTTPRoute hostname should use the environment override host")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})
	})

})
