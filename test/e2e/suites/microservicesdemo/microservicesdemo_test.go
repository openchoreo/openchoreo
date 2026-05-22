// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

const (
	// The demo's project + components target the `default` namespace and rely
	// on the getting-started default DeploymentPipeline + Environments which
	// `e2e.setup-configure` already installs.
	demoCPNamespace = "default"
	demoProject     = "gcp-microservice-demo"
	demoEnvironment = "development"

	demoSampleSubpath = "samples/gcp-microservices-demo"

	demoFrontend = "frontend"
	demoCatalog  = "productcatalog"

	// Stable product ID from the demo's productcatalogservice/products.json.
	demoProductID = "OLJCESPC7Z"

	demoTesterLabel     = "app=msd-tester"
	demoTesterContainer = "tester"
)

// All components shipped by the demo. Used to wait for each RB to reach Ready.
var demoComponents = []string{
	"ad", "cart", "checkout", "currency", "email",
	"frontend", "payment", "productcatalog",
	"recommendation", "redis", "shipping",
}

var demoDPNs string

var _ = Describe("GCP Microservices Demo", Ordered, func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	BeforeAll(func() {
		By("resolving repo root for sample manifests")
		repoRoot, err := framework.RepoRoot()
		Expect(err).NotTo(HaveOccurred(), "failed to locate repo root")
		sampleDir := filepath.Join(repoRoot, demoSampleSubpath)
		_, err = os.Stat(sampleDir)
		Expect(err).NotTo(HaveOccurred(), "sample directory missing: %s", sampleDir)

		By("applying gcp-microservices-demo sample manifests")
		output, err := framework.Kubectl(kubeContext, "apply", "-f", sampleDir, "-R")
		Expect(err).NotTo(HaveOccurred(), "kubectl apply failed: %s", output)

		By("waiting for project DP namespace discovery")
		Eventually(func() error {
			var discoverErr error
			demoDPNs, discoverErr = framework.GetDPNamespace(
				kubeContext, demoCPNamespace, demoProject, demoEnvironment,
			)
			return discoverErr
		}, 5*time.Minute, 5*time.Second).Should(Succeed(),
			"dp namespace for %s/%s not found", demoProject, demoEnvironment)
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: %s\n", demoDPNs)

		By("deploying tester pod in the project DP namespace")
		output, err = framework.KubectlApplyLiteral(kubeContext, testerPodYAML(demoDPNs))
		Expect(err).NotTo(HaveOccurred(), "failed to create tester pod: %s", output)

		By("waiting for tester pod to be Running")
		Eventually(func(g Gomega) {
			framework.AssertPodsRunning(g, kubeContext, demoDPNs, demoTesterLabel)
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}

		By("deleting tester pod")
		if demoDPNs != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "pod", "msd-tester",
				"-n", demoDPNs, "--ignore-not-found", "--wait=false")
		}

		By("deleting demo project (cascades to all components + RBs + DP resources)")
		_, _ = framework.Kubectl(kubeContext, "delete", "project", demoProject,
			"-n", demoCPNamespace, "--ignore-not-found", "--wait=false")
	})

	Context("multi-service deployment", func() {
		It("all ReleaseBindings reach Ready", func() {
			for _, comp := range demoComponents {
				rbName := comp + "-" + demoEnvironment
				By("waiting on ReleaseBinding " + rbName)
				Eventually(func(g Gomega) {
					framework.AssertReleaseBindingReady(g, kubeContext, demoCPNamespace, rbName)
				}, 8*time.Minute, 5*time.Second).Should(Succeed(),
					"ReleaseBinding %s should be Ready", rbName)
			}
		})

		It("all demo pods are Running in the project DP namespace", func() {
			Eventually(func(g Gomega) {
				framework.AssertAllPodsRunning(g, kubeContext, demoDPNs)
			}, 8*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("frontend home page is reachable over the public URL", func() {
			// Probe goes through the kgateway external listener — this is the
			// URL a real user would hit. A 200 on `/` transitively proves the
			// frontend → productcatalog gRPC hop, because the home template
			// renders the product list and the handler 5xx's if that gRPC fails.
			base := frontendExternalHTTPURL()
			homeURL := base + "/"
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, demoDPNs, demoTesterLabel, demoTesterContainer,
					homeURL, 10,
				)
				return err
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"frontend home %s should return 200 (proves frontend → %s connectivity)",
				homeURL, demoCatalog)
		})

		It("frontend product detail page returns 200 for a known product", func() {
			// /product/<id> drives a GetProduct gRPC to productcatalog plus a
			// currency conversion via the currency service. 200 means both
			// downstream RPCs succeeded.
			productURL := frontendExternalHTTPURL() + "/product/" + demoProductID
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, demoDPNs, demoTesterLabel, demoTesterContainer,
					productURL, 10,
				)
				return err
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"frontend product page %s should return 200 (proves %s GetProduct + currency)",
				productURL, demoCatalog)
		})

		It("frontend cart page returns 200", func() {
			// /cart drives a GetCart RPC to the cart service (empty cart on a
			// fresh session is still a successful 200). This covers the cart
			// service which is otherwise only verified at the pod-running level.
			cartURL := frontendExternalHTTPURL() + "/cart"
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, demoDPNs, demoTesterLabel, demoTesterContainer,
					cartURL, 10,
				)
				return err
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"frontend cart page %s should return 200 (proves cart GetCart)",
				cartURL)
		})
	})
})

// frontendExternalHTTPURL polls the frontend ReleaseBinding until its `http`
// endpoint has an externally-resolved HTTP gateway URL, then returns the
// assembled base URL (scheme://host:port, no trailing slash). This is what a
// caller outside the cluster — or here, the tester pod going through CoreDNS
// rewrite + k3d port-forward — would actually hit on kgateway.
func frontendExternalHTTPURL() string {
	rbName := demoFrontend + "-" + demoEnvironment
	jp := func(expr string) (string, error) {
		return framework.KubectlGetJsonpath(
			kubeContext, demoCPNamespace, "releasebinding", rbName,
			fmt.Sprintf(`{.status.endpoints[?(@.name=="http")].externalURLs.http.%s}`, expr),
		)
	}
	var url string
	Eventually(func(g Gomega) {
		scheme, err := jp("scheme")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(scheme).NotTo(BeEmpty(), "externalURLs.http.scheme not populated on %s", rbName)

		host, err := jp("host")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(host).NotTo(BeEmpty(), "externalURLs.http.host not populated on %s", rbName)

		port, err := jp("port")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(port).NotTo(BeEmpty(), "externalURLs.http.port not populated on %s", rbName)

		url = fmt.Sprintf("%s://%s:%s", scheme, host, port)
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
	return url
}
