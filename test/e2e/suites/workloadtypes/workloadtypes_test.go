// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

const (
	// Image references for the 3 workload types. greeter-service and
	// github-issue-reporter come from the public OpenChoreo sample images;
	// the web-app slot uses hashicorp/http-echo because the react sample
	// image is not consistently reachable from CI.
	imageService   = "ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40"
	imageWebApp    = "hashicorp/http-echo:1.0.0"
	imageScheduled = "ghcr.io/openchoreo/samples/github-issue-reporter:latest"
)

var dpNs string

var _ = Describe("Workload Type Matrix", Ordered, Label("tier1"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	BeforeAll(func() {
		By("creating control plane namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)

		By("applying platform resources (pipeline, environment, project)")
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		// The DP namespace is created by the ProjectReleaseBinding applied
		// above once the project's first release is cut, so deploy components
		// then poll for it.
		By("deploying service component (greeter)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentWithImageYAML(
			componentService, "deployment/service", imageService, servicePort,
			[]string{"--port", strconv.Itoa(servicePort)},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create service component: %s", output)

		By("deploying web-application component (http-echo stand-in)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentWithImageYAML(
			componentWebApp, "deployment/web-application", imageWebApp, webAppPort,
			[]string{"-listen=:" + strconv.Itoa(webAppPort), "-text=react-spa"},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create web-app component: %s", output)

		By("deploying scheduled-task component (issue-reporter)")
		output, err = framework.KubectlApplyLiteral(kubeContext,
			scheduledTaskComponentYAML(componentScheduled, imageScheduled))
		Expect(err).NotTo(HaveOccurred(), "failed to create scheduled-task component: %s", output)

		By("waiting for data plane namespace discovery")
		Eventually(func() error {
			var discoverErr error
			dpNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"dp namespace for %s/%s not found", projectName, envDev)
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: %s\n", dpNs)

		By("deploying tester pod for in-cluster invoke")
		output, err = framework.KubectlApplyLiteral(kubeContext, testerPodYAML(dpNs))
		Expect(err).NotTo(HaveOccurred(), "failed to create tester pod: %s", output)

		By("waiting for tester pod to be Running")
		Eventually(func(g Gomega) {
			framework.AssertPodsRunning(g, kubeContext, dpNs, testerLabel)
		}, 4*time.Minute, 3*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}
		By("cleaning up control plane namespace (cascades to DP namespace)")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs,
			"--ignore-not-found", "--wait=false")
		if dpNs != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", dpNs,
				"--ignore-not-found", "--wait=false")
		}
	})

	Context("rendering and reachability", func() {
		It("service: ReleaseBinding becomes Ready and endpoint is reachable", func() {
			By("waiting for ReleaseBinding Ready")
			Eventually(func(g Gomega) {
				framework.AssertReleaseBindingReady(g, kubeContext, cpNs, componentService+releaseBindingSuffix)
			}, 5*time.Minute, 2*time.Second).Should(Succeed())

			By("workload pod is Running")
			Eventually(func(g Gomega) {
				framework.AssertPodsRunning(g, kubeContext, dpNs,
					"openchoreo.dev/component="+componentService)
			}, 3*time.Minute, 2*time.Second).Should(Succeed())

			By("TCP port is reachable from tester pod")
			host, port := endpointHostPort(componentService, "http")
			Eventually(func() error {
				_, err := framework.CheckTCPReachableFromPodByLabel(
					kubeContext, dpNs, testerLabel, testerContainer, host, port, 5,
				)
				return err
			}, 2*time.Minute, 5*time.Second).Should(Succeed(),
				"service endpoint %s:%s should be TCP-reachable", host, port)
		})

		It("web-application: ReleaseBinding becomes Ready and endpoint is reachable", func() {
			By("waiting for ReleaseBinding Ready")
			Eventually(func(g Gomega) {
				framework.AssertReleaseBindingReady(g, kubeContext, cpNs, componentWebApp+releaseBindingSuffix)
			}, 5*time.Minute, 2*time.Second).Should(Succeed())

			By("workload pod is Running")
			Eventually(func(g Gomega) {
				framework.AssertPodsRunning(g, kubeContext, dpNs,
					"openchoreo.dev/component="+componentWebApp)
			}, 3*time.Minute, 2*time.Second).Should(Succeed())

			By("TCP port is reachable from tester pod")
			host, port := endpointHostPort(componentWebApp, "http")
			Eventually(func() error {
				_, err := framework.CheckTCPReachableFromPodByLabel(
					kubeContext, dpNs, testerLabel, testerContainer, host, port, 5,
				)
				return err
			}, 2*time.Minute, 5*time.Second).Should(Succeed(),
				"web-app endpoint %s:%s should be TCP-reachable", host, port)
		})

		It("scheduled-task: ReleaseBinding becomes Ready and CronJob gets scheduled", func() {
			By("waiting for ReleaseBinding Ready")
			Eventually(func(g Gomega) {
				framework.AssertReleaseBindingReady(g, kubeContext, cpNs, componentScheduled+releaseBindingSuffix)
			}, 5*time.Minute, 2*time.Second).Should(Succeed())

			By("CronJob exists in the DP namespace")
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext,
					"get", "cronjob",
					"-n", dpNs,
					"-l", "openchoreo.dev/component="+componentScheduled,
					"-o", "jsonpath={.items[0].metadata.name}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).NotTo(BeEmpty(), "no CronJob found for component %s in %s", componentScheduled, dpNs)
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("CronJob status.lastScheduleTime is populated (Job was scheduled)")
			// Schedule is * * * * * so the kube-controller-manager should fire
			// the first Job within ~60s. We do not assert Job completion: the
			// sample image needs unreachable MySQL/SMTP/GitHub and will exit
			// non-zero, but scheduling is the meaningful signal here.
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext,
					"get", "cronjob",
					"-n", dpNs,
					"-l", "openchoreo.dev/component="+componentScheduled,
					"-o", "jsonpath={.items[0].status.lastScheduleTime}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).NotTo(BeEmpty(),
					"CronJob for %s should have status.lastScheduleTime populated", componentScheduled)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("service: public URL is reachable through kgateway", func() {
			// visibility includes external, so the controller renders an
			// HTTPRoute on the data plane's external gateway. The service
			// ClusterComponentType matches /<componentName>-<endpoint> and
			// URL-rewrites that prefix back to "/" before forwarding to the
			// backend — so we append the greeter sample's documented REST
			// path (/greeter/greet) to hit a known-200 handler.
			base := endpointExternalHTTPURL(componentService, "http")
			url := base + "/greeter/greet"
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, dpNs, testerLabel, testerContainer, url, 10,
				)
				return err
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"service public URL %s should return 200", url)
		})

		It("web-application: public URL is reachable through kgateway", func() {
			// The web-application template forwards all paths without a
			// prefix rewrite, and http-echo replies 200 with the configured
			// text on any path — so a plain GET of the externalURL is enough.
			base := endpointExternalHTTPURL(componentWebApp, "http")
			url := base + "/"
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(
					kubeContext, dpNs, testerLabel, testerContainer, url, 10,
				)
				return err
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"web-application public URL %s should return 200", url)
		})
	})

	Context("deletion drains rendered resources", func() {
		It("Component delete drains the workload from the DP namespace", func() {
			By("deleting the web-application component")
			output, err := framework.Kubectl(kubeContext,
				"delete", "component", componentWebApp,
				"-n", cpNs, "--wait=true", "--timeout=3m")
			Expect(err).NotTo(HaveOccurred(),
				"failed to delete component %s: %s", componentWebApp, output)

			By("ReleaseBinding for the deleted component is gone")
			Eventually(func(g Gomega) {
				framework.AssertResourceGone(g, kubeContext, cpNs, "releasebinding",
					componentWebApp+releaseBindingSuffix)
			}, 3*time.Minute, 2*time.Second).Should(Succeed())

			By("no Deployment for the deleted component remains in DP namespace")
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext,
					"get", "deployment",
					"-n", dpNs,
					"-l", "openchoreo.dev/component="+componentWebApp,
					"-o", "jsonpath={.items[*].metadata.name}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(BeEmpty(),
					"Deployment for deleted component %s still present: %s", componentWebApp, out)
			}, 3*time.Minute, 2*time.Second).Should(Succeed())

			By("the service component is untouched")
			Eventually(func(g Gomega) {
				framework.AssertReleaseBindingReady(g, kubeContext, cpNs, componentService+releaseBindingSuffix)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("Project delete drains the DP namespace", func() {
			By("deleting the project")
			output, err := framework.Kubectl(kubeContext,
				"delete", "project", projectName,
				"-n", cpNs, "--wait=false")
			Expect(err).NotTo(HaveOccurred(),
				"failed to delete project %s: %s", projectName, output)

			By("all remaining ReleaseBindings under the project disappear")
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext,
					"get", "releasebinding",
					"-n", cpNs,
					"-o", "jsonpath={.items[*].metadata.name}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(BeEmpty(),
					"ReleaseBindings still present after project delete: %s", out)
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("DP namespace fully drains (no Deployment/Service/CronJob left)")
			Eventually(func(g Gomega) {
				deployments, err := framework.Kubectl(kubeContext,
					"get", "deployment", "-n", dpNs,
					"-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(deployments).To(BeEmpty(), "deployments remain in %s: %s", dpNs, deployments)

				services, err := framework.Kubectl(kubeContext,
					"get", "service", "-n", dpNs,
					"-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(services).To(BeEmpty(), "services remain in %s: %s", dpNs, services)

				cronjobs, err := framework.Kubectl(kubeContext,
					"get", "cronjob", "-n", dpNs,
					"-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cronjobs).To(BeEmpty(), "cronjobs remain in %s: %s", dpNs, cronjobs)
			}, 5*time.Minute, 5*time.Second).Should(Succeed())
		})
	})
})

// endpointExternalHTTPURL reads the rendered external HTTP gateway URL for a
// named endpoint off the ReleaseBinding status, assembled from scheme/host/
// port/path. This is the URL a real caller would hit through kgateway.
// Path is optional — the web-application template renders no path prefix,
// while the service template uses /<componentName>-<endpointKey>.
func endpointExternalHTTPURL(component, endpoint string) string {
	rbName := component + releaseBindingSuffix
	jp := func(g Gomega, field string) string {
		out, err := framework.KubectlGetJsonpath(
			kubeContext, cpNs, "releasebinding", rbName,
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
		path := jp(g, "path") // empty for web-application; "/<componentName>-<endpointKey>" for service
		g.Expect(scheme).NotTo(BeEmpty(), "externalURLs.http.scheme empty on %s endpoint %s", rbName, endpoint)
		g.Expect(host).NotTo(BeEmpty(), "externalURLs.http.host empty on %s endpoint %s", rbName, endpoint)
		g.Expect(port).NotTo(BeEmpty(), "externalURLs.http.port empty on %s endpoint %s", rbName, endpoint)
		url = fmt.Sprintf("%s://%s:%s%s", scheme, host, port, path)
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
	return url
}

// endpointHostPort reads the rendered Service URL host+port for a named endpoint
// off the ReleaseBinding status. Returns string host + string port (port is
// decimal as serialised in jsonpath).
func endpointHostPort(component, endpoint string) (host, port string) {
	rbName := component + releaseBindingSuffix
	var hostOut, portOut string
	Eventually(func(g Gomega) {
		var err error
		hostOut, err = framework.KubectlGetJsonpath(
			kubeContext, cpNs, "releasebinding", rbName,
			fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].serviceURL.host}`, endpoint),
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(hostOut).NotTo(BeEmpty(), "serviceURL.host empty on %s endpoint %s", rbName, endpoint)

		portOut, err = framework.KubectlGetJsonpath(
			kubeContext, cpNs, "releasebinding", rbName,
			fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].serviceURL.port}`, endpoint),
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(portOut).NotTo(BeEmpty(), "serviceURL.port empty on %s endpoint %s", rbName, endpoint)
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
	return hostOut, portOut
}
