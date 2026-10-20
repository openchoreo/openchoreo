// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var dpDevNs, dpStagingNs string

var _ = Describe("Release Lines", Ordered, Label("tier1"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	BeforeAll(func() {
		By("checking the release line component type is installed")
		_, err := framework.Kubectl(kubeContext, "get", "clustercomponenttype", "release-line-service")
		Expect(err).NotTo(HaveOccurred(),
			"ClusterComponentType release-line-service is missing; the e2e setup installs it from samples/release-lines")

		By("creating control plane namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)

		By("applying platform resources (pipeline, environments, project)")
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		By("creating the greeter component with the stable and canary lines")
		output, err = framework.KubectlApplyLiteral(kubeContext, greeterYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply greeter component: %s", output)

		By("waiting for the data plane namespaces")
		Eventually(func() error {
			var discoverErr error
			dpDevNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		Eventually(func() error {
			var discoverErr error
			dpStagingNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envStaging)
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("deploying tester pod for in-cluster invoke")
		output, err = framework.KubectlApplyLiteral(kubeContext, testerPodYAML(dpDevNs))
		Expect(err).NotTo(HaveOccurred(), "failed to create tester pod: %s", output)
		Eventually(func(g Gomega) {
			framework.AssertPodsRunning(g, kubeContext, dpDevNs, testerLabel)
		}, 4*time.Minute, 3*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}
		By("cleaning up control plane namespace (cascades to DP namespaces)")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")
		for _, ns := range []string{dpDevNs, dpStagingNs} {
			if ns != "" {
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
			}
		}
	})

	Context("deploying lines side by side", func() {
		It("deploys each line to development through its own release binding", func() {
			for _, line := range []string{lineStable, lineCanary} {
				By("waiting for the " + line + " binding to be Ready")
				Eventually(func(g Gomega) {
					framework.AssertReleaseBindingReady(g, kubeContext, cpNs, scopedName(componentGreeter, line, envDev))
				}, 5*time.Minute, 2*time.Second).Should(Succeed())

				By("checking the " + line + " line reports development as ready")
				Eventually(func(g Gomega) {
					g.Expect(lineEnvironment(g, line, envDev)).To(Equal("true"))
					g.Expect(jsonpath(g, "releaseline", line, "{.status.latestRelease.name}")).
						To(HavePrefix(componentGreeter + "-" + line + "-"))
				}, 2*time.Minute, 2*time.Second).Should(Succeed())
			}

			By("checking the component deploys through its lines")
			Eventually(func(g Gomega) {
				g.Expect(jsonpath(g, "component", componentGreeter,
					`{.status.conditions[?(@.type=="Ready")].reason}`)).To(Equal("ReleaseLinesReconciled"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("renders each line's objects under their own names in the same namespace", func() {
			stable := lineDeployment(lineStable)
			canary := lineDeployment(lineCanary)
			Expect(stable.Name).NotTo(Equal(canary.Name))

			for _, line := range []string{lineStable, lineCanary} {
				By("checking the " + line + " Service selects only its own line's pods")
				service := componentGreeter + "-" + line
				selector, err := framework.KubectlGetJsonpath(kubeContext, dpDevNs, "service", service,
					`{.spec.selector.openchoreo\.dev/release-line}`)
				Expect(err).NotTo(HaveOccurred(), "Service %s not found", service)
				Expect(selector).To(Equal(line))

				By("checking the " + line + " pods are Running")
				Eventually(func(g Gomega) {
					framework.AssertPodsRunning(g, kubeContext, dpDevNs,
						labelComponent+"="+componentGreeter+","+labelReleaseLine+"="+line)
				}, 3*time.Minute, 2*time.Second).Should(Succeed())
			}
		})

		It("serves each line on its own service", func() {
			for _, line := range []string{lineStable, lineCanary} {
				By("calling the " + line + " service from inside the namespace")
				url := serviceURL(scopedName(componentGreeter, line, envDev)) + greeterPath
				Eventually(func() error {
					_, err := framework.InvokeFromPodByLabel(kubeContext, dpDevNs, testerLabel, testerContain, url, 10)
					return err
				}, 3*time.Minute, 5*time.Second).Should(Succeed(), "%s should return 200", url)
			}
		})

		It("exposes each line on its own public route", func() {
			for _, line := range []string{lineStable, lineCanary} {
				By("calling the " + line + " public URL through the gateway")
				url := externalURL(scopedName(componentGreeter, line, envDev)) + greeterPath
				Expect(url).To(ContainSubstring("/" + componentGreeter + "-" + line + "-http"))
				Eventually(func() error {
					_, err := framework.InvokeFromPodByLabel(kubeContext, dpDevNs, testerLabel, testerContain, url, 10)
					return err
				}, 3*time.Minute, 5*time.Second).Should(Succeed(), "%s should return 200", url)
			}
		})
	})

	Context("line overrides", func() {
		It("applies a line's parameter override to that line only", func() {
			Expect(containerEnv(lineDeployment(lineStable), "LOG_LEVEL")).To(Equal("info"))
			Expect(containerEnv(lineDeployment(lineCanary), "LOG_LEVEL")).To(Equal("debug"))
		})

		It("merges a line's trait parameters onto the component's trait", func() {
			canary := mainContainer(lineDeployment(lineCanary))
			Expect(canary.ReadinessProbe).NotTo(BeNil(), "canary keeps the component's readiness probe")
			Expect(canary.LivenessProbe).NotTo(BeNil(), "canary adds a liveness probe to the component's trait")

			stable := mainContainer(lineDeployment(lineStable))
			Expect(stable.LivenessProbe).To(BeNil(), "stable has no liveness probe")
		})

		It("adds a line's own trait to that line only", func() {
			Eventually(func(g Gomega) {
				g.Expect(lineObjects(g, "hpa", lineCanary)).To(HaveLen(1))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
			Expect(lineObjects(Default, "hpa", lineStable)).To(BeEmpty())
		})

		It("keeps the component's traits on a line, which can't remove them", func() {
			By("listing only the autoscaler on the stable line")
			patch := `{"spec":{"traits":[{"kind":"ClusterTrait","name":"horizontal-pod-autoscaler",` +
				`"instanceName":"` + scalerTrait + `","parameters":{"replicas":{"min":1,"max":2}}}]}}`
			output, err := framework.Kubectl(kubeContext, "patch", "releaseline", lineStable,
				"-n", cpNs, "--type=merge", "-p", patch)
			Expect(err).NotTo(HaveOccurred(), "failed to patch line %s: %s", lineStable, output)

			By("waiting for the stable line to render its autoscaler")
			Eventually(func(g Gomega) {
				g.Expect(lineObjects(g, "hpa", lineStable)).To(HaveLen(1))
			}, 3*time.Minute, 2*time.Second).Should(Succeed())

			By("checking stable still has the component's readiness probe")
			Expect(mainContainer(lineDeployment(lineStable)).ReadinessProbe).NotTo(BeNil())
		})
	})

	Context("promotion", func() {
		It("promotes one line to staging and leaves the other line in development", func() {
			release, err := framework.KubectlGetJsonpath(kubeContext, cpNs, "releaseline", lineCanary,
				"{.status.latestRelease.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeEmpty())

			By("binding the canary release to staging")
			output, err := framework.KubectlApplyLiteral(kubeContext, promotionYAML(componentGreeter, lineCanary, release))
			Expect(err).NotTo(HaveOccurred(), "failed to promote canary: %s", output)

			Eventually(func(g Gomega) {
				framework.AssertReleaseBindingReady(g, kubeContext, cpNs, scopedName(componentGreeter, lineCanary, envStaging))
			}, 5*time.Minute, 2*time.Second).Should(Succeed())

			By("checking the canary line reports staging")
			Eventually(func(g Gomega) {
				g.Expect(lineEnvironment(g, lineCanary, envStaging)).To(Equal("true"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("checking only canary runs in staging")
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext, "get", "deployment", "-n", dpStagingNs,
					"-l", labelComponent+"="+componentGreeter, "-o", `jsonpath={.items[*].metadata.labels.openchoreo\.dev/release-line}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(Equal(lineCanary))
			}, 3*time.Minute, 2*time.Second).Should(Succeed())
			Expect(lineEnvironment(Default, lineStable, envStaging)).To(BeEmpty())
		})
	})

	Context("validation", func() {
		It("keeps line names unique within the namespace", func() {
			By("creating a second component with release lines")
			output, err := framework.KubectlApplyLiteral(kubeContext, mustYAMLDocs(
				lineComponent(componentEcho, lineComponentType, nil),
				releaseLine(lineEchoMain, componentEcho),
			))
			Expect(err).NotTo(HaveOccurred(), "failed to create echo component: %s", output)

			By("creating a line of echo with a name greeter already uses")
			output, err = kubectlCreate(mustYAMLDocs(releaseLine(lineStable, componentEcho)))
			Expect(err).To(HaveOccurred(), "a second %q line in the namespace must be rejected", lineStable)
			Expect(output).To(ContainSubstring("already exists"))

			By("checking greeter's stable line is untouched")
			Expect(jsonpath(Default, "releaseline", lineStable, "{.spec.owner.componentName}")).To(Equal(componentGreeter))
		})

		It("rejects release lines on a component type that doesn't allow them", func() {
			output, err := framework.KubectlApplyLiteral(kubeContext,
				mustYAMLDocs(lineComponent(componentPlain, plainComponentType, nil)))
			Expect(err).NotTo(HaveOccurred(), "failed to create plain component: %s", output)

			Eventually(func(g Gomega) {
				g.Expect(jsonpath(g, "component", componentPlain,
					`{.status.conditions[?(@.type=="Ready")].reason}`)).To(Equal("ReleaseLinesNotAllowed"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("flags a Workload without a line on a component with release lines", func() {
			unlined := mustYAMLDocs(lineWorkload(componentGreeter, ""))
			output, err := framework.KubectlApplyLiteral(kubeContext, unlined)
			Expect(err).NotTo(HaveOccurred(), "failed to create unlined workload: %s", output)

			Eventually(func(g Gomega) {
				g.Expect(jsonpath(g, "component", componentGreeter,
					`{.status.conditions[?(@.type=="Ready")].reason}`)).To(Equal("InvalidConfiguration"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("removing the unlined workload")
			output, err = framework.KubectlDeleteLiteral(kubeContext, unlined)
			Expect(err).NotTo(HaveOccurred(), "failed to delete unlined workload: %s", output)
			Eventually(func(g Gomega) {
				g.Expect(jsonpath(g, "component", componentGreeter,
					`{.status.conditions[?(@.type=="Ready")].reason}`)).To(Equal("ReleaseLinesReconciled"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})
	})

	Context("deleting a line", func() {
		It("removes the line's objects and leaves the other line running", func() {
			output, err := framework.Kubectl(kubeContext, "delete", "releaseline", lineCanary,
				"-n", cpNs, "--wait=true", "--timeout=3m")
			Expect(err).NotTo(HaveOccurred(), "failed to delete line %s: %s", lineCanary, output)

			By("checking the canary bindings, releases and workload are gone")
			Eventually(func(g Gomega) {
				for _, env := range []string{envDev, envStaging} {
					framework.AssertResourceGone(g, kubeContext, cpNs, "releasebinding", scopedName(componentGreeter, lineCanary, env))
				}
				framework.AssertResourceGone(g, kubeContext, cpNs, "workload", scopedName(componentGreeter, lineCanary, "workload"))
				releases, err := framework.Kubectl(kubeContext, "get", "componentrelease", "-n", cpNs,
					"-o", fmt.Sprintf(`jsonpath={.items[?(@.spec.owner.releaseLine=="%s")].metadata.name}`, lineCanary))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(releases).To(BeEmpty())
			}, 3*time.Minute, 2*time.Second).Should(Succeed())

			By("checking the canary Deployments are gone from both data plane namespaces")
			Eventually(func(g Gomega) {
				g.Expect(lineObjects(g, "deployment", lineCanary)).To(BeEmpty())
				out, err := framework.Kubectl(kubeContext, "get", "deployment", "-n", dpStagingNs,
					"-l", labelComponent+"="+componentGreeter, "-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(BeEmpty())
			}, 3*time.Minute, 2*time.Second).Should(Succeed())

			By("checking stable is still Ready and serving")
			framework.AssertReleaseBindingReady(Default, kubeContext, cpNs, scopedName(componentGreeter, lineStable, envDev))
			url := serviceURL(scopedName(componentGreeter, lineStable, envDev)) + greeterPath
			Eventually(func() error {
				_, err := framework.InvokeFromPodByLabel(kubeContext, dpDevNs, testerLabel, testerContain, url, 10)
				return err
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})
	})
})

// jsonpath reads a field of a control plane object in the suite's namespace.
func jsonpath(g Gomega, resource, name, path string) string {
	out, err := framework.KubectlGetJsonpath(kubeContext, cpNs, resource, name, path)
	g.Expect(err).NotTo(HaveOccurred(), "failed to read %s %s", resource, name)
	return out
}

// lineEnvironment returns the readiness a line reports for an environment, or "" when the line
// isn't deployed there.
func lineEnvironment(g Gomega, line, env string) string {
	return jsonpath(g, "releaseline", line, fmt.Sprintf(`{.status.environments[?(@.environment=="%s")].ready}`, env))
}

// lineObjects lists the names of a kind of object rendered for a greeter line in development.
func lineObjects(g Gomega, resource, line string) []string {
	out, err := framework.Kubectl(kubeContext, "get", resource, "-n", dpDevNs,
		"-l", labelComponent+"="+componentGreeter+","+labelReleaseLine+"="+line, "-o", "name")
	g.Expect(err).NotTo(HaveOccurred())
	var names []string
	for _, n := range strings.Fields(out) {
		names = append(names, filepath.Base(n))
	}
	return names
}

// lineDeployment returns the single Deployment rendered for a greeter line in development.
func lineDeployment(line string) appsv1.Deployment {
	var list appsv1.DeploymentList
	Eventually(func(g Gomega) {
		out, err := framework.Kubectl(kubeContext, "get", "deployment", "-n", dpDevNs,
			"-l", labelComponent+"="+componentGreeter+","+labelReleaseLine+"="+line, "-o", "json")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(json.Unmarshal([]byte(out), &list)).To(Succeed())
		g.Expect(list.Items).To(HaveLen(1), "expected one Deployment for line %s", line)
	}, 2*time.Minute, 2*time.Second).Should(Succeed())
	return list.Items[0]
}

func mainContainer(d appsv1.Deployment) *corev1.Container {
	for i := range d.Spec.Template.Spec.Containers {
		if d.Spec.Template.Spec.Containers[i].Name == "main" {
			return &d.Spec.Template.Spec.Containers[i]
		}
	}
	Fail(fmt.Sprintf("Deployment %s has no main container", d.Name))
	return nil
}

func containerEnv(d appsv1.Deployment, name string) string {
	for _, env := range mainContainer(d).Env {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

// serviceURL is the in-cluster URL of a binding's http endpoint, from the binding status.
func serviceURL(binding string) string {
	var host, port string
	Eventually(func(g Gomega) {
		host = jsonpath(g, "releasebinding", binding, `{.status.endpoints[?(@.name=="http")].serviceURL.host}`)
		port = jsonpath(g, "releasebinding", binding, `{.status.endpoints[?(@.name=="http")].serviceURL.port}`)
		g.Expect(host).NotTo(BeEmpty())
		g.Expect(port).NotTo(BeEmpty())
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
	return fmt.Sprintf("http://%s:%s", host, port)
}

// externalURL is the public URL of a binding's http endpoint, from the binding status.
func externalURL(binding string) string {
	var url string
	Eventually(func(g Gomega) {
		field := func(f string) string {
			return jsonpath(g, "releasebinding", binding,
				fmt.Sprintf(`{.status.endpoints[?(@.name=="http")].externalURLs.http.%s}`, f))
		}
		scheme, host, port, path := field("scheme"), field("host"), field("port"), field("path")
		g.Expect(scheme).NotTo(BeEmpty())
		g.Expect(host).NotTo(BeEmpty())
		g.Expect(port).NotTo(BeEmpty())
		url = fmt.Sprintf("%s://%s:%s%s", scheme, host, port, path)
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
	return url
}

// kubectlCreate runs kubectl create on literal YAML, which fails when the object already exists.
func kubectlCreate(yamlContent string) (string, error) {
	f, err := os.CreateTemp("", "rl-create-*.yaml")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(yamlContent); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return framework.Kubectl(kubeContext, "create", "-f", f.Name())
}
