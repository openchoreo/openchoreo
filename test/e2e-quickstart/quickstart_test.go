// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package quickstart

import (
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

var _ = Describe("Quick Start Guide", Ordered, func() {
	const (
		installTimeout = 10 * time.Minute
		deployTimeout  = 5 * time.Minute
		httpTimeout    = 2 * time.Minute
	)

	BeforeAll(func() {
		By("starting the quick-start container")
		Expect(startContainer(image)).To(Succeed())

		By("verifying Docker is accessible inside the container")
		_, err := dockerExec("docker info >/dev/null 2>&1")
		Expect(err).NotTo(HaveOccurred(), "docker should be accessible inside the container")

		By(fmt.Sprintf("running install.sh --version %s", version))
		output, err := dockerExec(fmt.Sprintf("./install.sh --version %s --skip-resource-check", version))
		fmt.Fprintf(GinkgoWriter, "install output (last 500 chars):\n%s\n", lastN(output, 500))
		Expect(err).NotTo(HaveOccurred(), "install.sh should succeed")
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}

		By("running uninstall.sh")
		dockerExec("./uninstall.sh --force") //nolint:errcheck

		By("removing container")
		cleanupContainer()
	})

	// ─── Installation validation ───────────────────────────────────────

	Context("Installation validation", func() {
		It("should pass check-status.sh with all core components READY", func() {
			Eventually(func(g Gomega) {
				output, err := dockerExec("./check-status.sh")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(ContainSubstring("[NOT STARTED]"))
				g.Expect(output).NotTo(ContainSubstring("[PENDING]"))
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("should pass validate-installation.sh", func() {
			_, err := dockerExec("./validate-installation.sh")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ─── CRDs ──────────────────────────────────────────────────────────

	Context("CRDs", func() {
		crds := []string{
			"projects.openchoreo.dev",
			"components.openchoreo.dev",
			"componenttypes.openchoreo.dev",
			"environments.openchoreo.dev",
			"releasebindings.openchoreo.dev",
			"workloads.openchoreo.dev",
		}

		for _, crd := range crds {
			It(fmt.Sprintf("should have CRD %s", crd), func() {
				_, err := dockerExec(fmt.Sprintf("kubectl get crd %s", crd))
				Expect(err).NotTo(HaveOccurred())
			})
		}
	})

	// ─── Default resources ─────────────────────────────────────────────

	Context("Default resources", func() {
		It("should have Project 'default'", func() {
			_, err := dockerExec("kubectl get project default -o name")
			Expect(err).NotTo(HaveOccurred())
		})

		environments := []string{"development", "staging", "production"}
		for _, env := range environments {
			It(fmt.Sprintf("should have Environment '%s'", env), func() {
				_, err := dockerExec(fmt.Sprintf("kubectl get environment %s -o name", env))
				Expect(err).NotTo(HaveOccurred())
			})
		}

		clusterComponentTypes := []string{"worker", "service", "web-application", "scheduled-task"}
		for _, cct := range clusterComponentTypes {
			It(fmt.Sprintf("should have ClusterComponentType '%s'", cct), func() {
				_, err := dockerExec(fmt.Sprintf("kubectl get clustercomponenttype %s -o name", cct))
				Expect(err).NotTo(HaveOccurred())
			})
		}

		It("should have ClusterDataPlane 'default'", func() {
			_, err := dockerExec("kubectl get clusterdataplane default -o name")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ─── Pod health ────────────────────────────────────────────────────

	Context("Pod health", func() {
		namespaces := []string{"openchoreo-control-plane", "openchoreo-data-plane"}
		for _, ns := range namespaces {
			It(fmt.Sprintf("should have all pods running in %s", ns), func() {
				Eventually(func(g Gomega) {
					output, err := dockerExec(fmt.Sprintf(
						"kubectl get pods -n %s --no-headers --field-selector=status.phase!=Succeeded,status.phase!=Failed | grep -cv Running || true", ns))
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(strings.TrimSpace(output)).To(Equal("0"),
						"all non-completed pods in %s should be Running", ns)
				}, 2*time.Minute, 5*time.Second).Should(Succeed())
			})
		}
	})

	// ─── Deploy react-starter ──────────────────────────────────────────

	Context("React starter deployment", Ordered, func() {
		It("should deploy successfully via deploy-react-starter.sh", func() {
			output, err := dockerExec("./deploy-react-starter.sh")
			fmt.Fprintf(GinkgoWriter, "deploy output:\n%s\n", output)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should have Component 'react-starter'", func() {
			_, err := dockerExec("kubectl get component react-starter -o name")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should have Workload 'react-starter'", func() {
			_, err := dockerExec("kubectl get workload react-starter -o name")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should have ReleaseBinding 'react-starter-development' Ready", func() {
			Eventually(func(g Gomega) {
				output, err := dockerExec(
					`kubectl get releasebinding react-starter-development -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}, deployTimeout, 5*time.Second).Should(Succeed())
		})

		It("should list react-starter in 'kubectl get components'", func() {
			output, err := dockerExec("kubectl get components")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("react-starter"))
		})
	})

	// ─── HTTP access ───────────────────────────────────────────────────

	Context("HTTP access", func() {
		It("should serve the react-starter app via external gateway", func() {
			By("discovering the external URL from ReleaseBinding status")
			var reactURL string
			Eventually(func(g Gomega) {
				host, err := dockerExec(
					`kubectl get releasebinding react-starter-development -o jsonpath='{.status.endpoints[0].externalURLs.http.host}'`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(host).NotTo(BeEmpty())

				port, err := dockerExec(
					`kubectl get releasebinding react-starter-development -o jsonpath='{.status.endpoints[0].externalURLs.http.port}'`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(port).NotTo(BeEmpty())

				reactURL = fmt.Sprintf("http://%s:%s", host, port)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By(fmt.Sprintf("polling %s for HTTP 200", reactURL))
			Eventually(func(g Gomega) {
				output, err := dockerExec(fmt.Sprintf(
					"curl -s -o /dev/null -w '%%{http_code}' --connect-timeout 5 -4 '%s'", reactURL))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("200"), "react-starter should return HTTP 200")
			}, httpTimeout, 5*time.Second).Should(Succeed())
		})

		It("should serve the Backstage UI", func() {
			Eventually(func(g Gomega) {
				output, err := dockerExec(
					"curl -s -o /dev/null -w '%{http_code}' --connect-timeout 5 -4 http://openchoreo.localhost:8080/")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("200"), "Backstage UI should return HTTP 200")
			}, httpTimeout, 5*time.Second).Should(Succeed())
		})

		It("should have the OpenChoreo API reachable", func() {
			Eventually(func(g Gomega) {
				output, err := dockerExec(
					"curl -s -o /dev/null -w '%{http_code}' --connect-timeout 5 -4 http://api.openchoreo.localhost:8080/")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(Equal("000"), "API should be reachable (any HTTP status)")
			}, httpTimeout, 5*time.Second).Should(Succeed())
		})
	})

	// ─── Guide kubectl commands ────────────────────────────────────────

	Context("Guide kubectl commands", func() {
		It("should list control plane namespaces", func() {
			output, err := dockerExec("kubectl get namespaces -l openchoreo.dev/control-plane=true --no-headers")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty())
		})

		It("should show environments", func() {
			output, err := dockerExec("kubectl get environments")
			Expect(err).NotTo(HaveOccurred())
			for _, env := range []string{"development", "staging", "production"} {
				Expect(output).To(ContainSubstring(env))
			}
		})

		It("should show clustercomponenttypes", func() {
			output, err := dockerExec("kubectl get clustercomponenttypes")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty())
		})
	})

	// ─── Cleanup ───────────────────────────────────────────────────────

	Context("Sample app cleanup", Ordered, func() {
		It("should clean up react-starter via --clean flag", func() {
			_, err := dockerExec("./deploy-react-starter.sh --clean")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete the component", func() {
			Eventually(func(g Gomega) {
				output, err := dockerExec("kubectl get component react-starter -o name 2>&1")
				g.Expect(err).To(HaveOccurred())
				g.Expect(output).To(ContainSubstring("NotFound"))
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		})
	})
})

// lastN returns the last n characters of s.
func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

