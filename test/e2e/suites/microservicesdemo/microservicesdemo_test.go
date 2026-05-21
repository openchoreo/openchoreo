// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	demoFrontend  = "frontend"
	demoCatalog   = "productcatalog"

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

		It("frontend serves traffic and reaches productcatalog over gRPC", func() {
			// External invoke goes through kgateway; for a deterministic
			// cross-service signal we hit the rendered ClusterIP Service for
			// frontend from inside the DP namespace. If frontend's `/` returns
			// 200, the rendered page included the product list, which means
			// the in-pod gRPC client successfully reached productcatalog.
			host, port := serviceForComponent(demoDPNs, demoFrontend)
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, demoDPNs, demoTesterLabel, demoTesterContainer,
					fmt.Sprintf("http://%s:%s/", host, port), 10,
				)
				return err
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"frontend at %s:%s should return success (proves frontend → %s connectivity)",
				host, port, demoCatalog)
		})
	})
})

// serviceForComponent returns the name + first port of the rendered Service
// for an OpenChoreo component in the given DP namespace. Looked up by label.
func serviceForComponent(dpNamespace, component string) (name, port string) {
	var svcName, svcPort string
	Eventually(func(g Gomega) {
		out, err := framework.Kubectl(kubeContext,
			"get", "service",
			"-n", dpNamespace,
			"-l", "openchoreo.dev/component="+component,
			"-o", "jsonpath={.items[0].metadata.name}|{.items[0].spec.ports[0].port}",
		)
		g.Expect(err).NotTo(HaveOccurred())
		parts := strings.SplitN(out, "|", 2)
		g.Expect(parts).To(HaveLen(2), "unexpected output from service lookup: %q", out)
		svcName, svcPort = parts[0], parts[1]
		g.Expect(svcName).NotTo(BeEmpty(), "no Service found for component %s in %s", component, dpNamespace)
		g.Expect(svcPort).NotTo(BeEmpty(), "Service for component %s has no port", component)
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
	return svcName, svcPort
}
