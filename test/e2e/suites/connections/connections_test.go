// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var (
	dpNs string // data plane namespace for cpNs/proj1/development
)

var _ = Describe("Connection Resolution", Ordered, func() {
	// assertRBConditionInNs checks a ReleaseBinding condition in a specific namespace.
	assertRBConditionInNs := func(namespace, rbName, condType, expectedStatus, expectedReason string) {
		Eventually(func(g Gomega) {
			status, err := framework.KubectlGetJsonpath(
				kubeContext, namespace, "releasebinding", rbName,
				fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].status}`, condType),
			)
			g.Expect(err).NotTo(HaveOccurred(), "failed to get condition %s on ReleaseBinding %s", condType, rbName)
			g.Expect(status).To(Equal(expectedStatus),
				"expected condition %s status=%s on ReleaseBinding %s", condType, expectedStatus, rbName)

			reason, err := framework.KubectlGetJsonpath(
				kubeContext, namespace, "releasebinding", rbName,
				fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].reason}`, condType),
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(reason).To(Equal(expectedReason),
				"expected condition %s reason=%s on ReleaseBinding %s", condType, expectedReason, rbName)
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	}

	// assertRBCondition checks a ReleaseBinding condition in cpNs via jsonpath.
	assertRBCondition := func(rbName, condType, expectedStatus, expectedReason string) {
		assertRBConditionInNs(cpNs, rbName, condType, expectedStatus, expectedReason)
	}

	// assertRBConditionStatus checks only the status of a ReleaseBinding condition (any reason).
	assertRBConditionStatus := func(rbName, condType, expectedStatus string) {
		Eventually(func(g Gomega) {
			status, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "releasebinding", rbName,
				fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].status}`, condType),
			)
			g.Expect(err).NotTo(HaveOccurred(), "failed to get condition %s on ReleaseBinding %s", condType, rbName)
			g.Expect(status).To(Equal(expectedStatus),
				"expected condition %s status=%s on ReleaseBinding %s", condType, expectedStatus, rbName)
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	}

	// assertRBEndpointServiceURL checks that a ReleaseBinding endpoint has a serviceURL.
	assertRBEndpointServiceURL := func(rbName, endpointName string, expectedPort int) {
		Eventually(func(g Gomega) {
			host, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "releasebinding", rbName,
				fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].serviceURL.host}`, endpointName),
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(host).To(ContainSubstring(".svc.cluster.local"),
				"expected serviceURL host to contain .svc.cluster.local for endpoint %s on %s", endpointName, rbName)

			port, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "releasebinding", rbName,
				fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].serviceURL.port}`, endpointName),
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(port).To(Equal(fmt.Sprintf("%d", expectedPort)),
				"expected serviceURL port=%d for endpoint %s on %s", expectedPort, endpointName, rbName)
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	}

	// getReleaseDeploymentEnv retrieves env vars from the rendered Deployment in a RenderedRelease.
	getReleaseDeploymentEnv := func(componentName string) []map[string]any {
		output, err := framework.Kubectl(
			kubeContext,
			"get", "renderedrelease",
			"-n", cpNs,
			"-l", fmt.Sprintf("openchoreo.dev/component=%s,openchoreo.dev/environment=development", componentName),
			"-o", "json",
		)
		Expect(err).NotTo(HaveOccurred(), "failed to get RenderedRelease for %s", componentName)

		var releaseList struct {
			Items []struct {
				Spec struct {
					Resources []struct {
						ID     string           `json:"id"`
						Object *json.RawMessage `json:"object"`
					} `json:"resources"`
				} `json:"spec"`
			} `json:"items"`
		}
		Expect(json.Unmarshal([]byte(output), &releaseList)).To(Succeed())
		Expect(releaseList.Items).To(HaveLen(1), "expected exactly 1 RenderedRelease for component %s, got %d", componentName, len(releaseList.Items))

		release := releaseList.Items[0]
		for _, res := range release.Spec.Resources {
			if res.Object == nil {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal(*res.Object, &obj); err != nil {
				continue
			}
			kind, _ := obj["kind"].(string)
			if kind != "Deployment" {
				continue
			}

			spec, _ := obj["spec"].(map[string]any)
			if spec == nil {
				continue
			}
			template, _ := spec["template"].(map[string]any)
			if template == nil {
				continue
			}
			podSpec, _ := template["spec"].(map[string]any)
			if podSpec == nil {
				continue
			}
			containers, _ := podSpec["containers"].([]any)
			if len(containers) == 0 {
				continue
			}
			container, _ := containers[0].(map[string]any)
			if container == nil {
				continue
			}
			envList, _ := container["env"].([]any)
			result := make([]map[string]any, 0, len(envList))
			for _, e := range envList {
				if envMap, ok := e.(map[string]any); ok {
					result = append(result, envMap)
				}
			}
			return result
		}

		Fail(fmt.Sprintf("no Deployment resource found in RenderedRelease for component %s", componentName))
		return nil
	}

	BeforeAll(func() {
		// --- First namespace (cpNs): providers + consumers ---
		By("creating control plane namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)

		By("applying platform resources")
		output, err = framework.KubectlApplyLiteral(kubeContext,
			platformResourcesYAML(cpNs, []string{"development", "staging"}, []string{"proj1"}))
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		By("deploying provider-a (HTTP:8080, project+namespace visibility)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "provider-a", "deployment/service",
			"hashicorp/http-echo",
			[]string{"-text=provider-a", "-listen=:8080"},
			map[string]endpointDef{
				"api": {epType: "HTTP", port: 8080, visibility: []string{"project", "namespace"}},
			},
			nil,
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create provider-a: %s", output)

		By("deploying provider-b (HTTP:9090, project visibility)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "provider-b", "deployment/service",
			"hashicorp/http-echo",
			[]string{"-text=provider-b", "-listen=:9090"},
			map[string]endpointDef{
				"grpc-api": {epType: "HTTP", port: 9090, visibility: []string{"project"}},
			},
			nil,
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create provider-b: %s", output)

		By("waiting for data plane namespace discovery")
		Eventually(func() error {
			var discoverErr error
			dpNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, "proj1", "development")
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "dp namespace for proj1/development not found")
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: %s\n", dpNs)

		By("waiting for provider ReleaseBindings to reach Ready=True")
		assertRBCondition("provider-a-development", "Ready", "True", "Ready")
		assertRBCondition("provider-b-development", "Ready", "True", "Ready")

		By("deploying consumer with project-visibility connections to both providers")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "consumer", "deployment/service",
			"hashicorp/http-echo",
			[]string{"-text=consumer", "-listen=:3000"},
			map[string]endpointDef{
				"web": {epType: "HTTP", port: 3000, visibility: []string{"project"}},
			},
			[]connectionDef{
				{
					component:  "provider-a",
					endpoint:   "api",
					visibility: "project",
					envURL:     "PROVIDER_A_URL",
				},
				{
					component:  "provider-b",
					endpoint:   "grpc-api",
					visibility: "project",
					envURL:     "PROVIDER_B_URL",
				},
			},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create consumer: %s", output)

	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}

		By("cleaning up data plane namespaces")
		if dpNs != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", dpNs, "--ignore-not-found", "--wait=false")
		}

		By("cleaning up control plane namespaces")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")
	})

	// --- Project/namespace visibility tests ---

	Context("project visibility", func() {
		It("resolves provider endpoints without connections", func() {
			By("provider-a ReleaseBinding should have ConnectionsResolved=True with reason NoConnections")
			assertRBCondition("provider-a-development", "ConnectionsResolved", "True", "NoConnections")
			assertRBCondition("provider-a-development", "Ready", "True", "Ready")

			By("provider-a should have serviceURL for api endpoint")
			assertRBEndpointServiceURL("provider-a-development", "api", 8080)
		})

		It("resolves consumer connections eventually", func() {
			By("consumer ReleaseBinding should reach ConnectionsResolved=True")
			assertRBCondition("consumer-development", "ConnectionsResolved", "True", "AllConnectionsResolved")
			assertRBCondition("consumer-development", "Ready", "True", "Ready")
		})

		It("sets consumer's own endpoint URLs", func() {
			By("consumer should have serviceURL for web endpoint")
			assertRBEndpointServiceURL("consumer-development", "web", 3000)
		})

		It("renders connection env vars in the RenderedRelease Deployment", func() {
			By("waiting for consumer connections to resolve first")
			assertRBCondition("consumer-development", "ConnectionsResolved", "True", "AllConnectionsResolved")

			By("checking rendered RenderedRelease for connection env vars")
			envVars := getReleaseDeploymentEnv("consumer")

			envMap := make(map[string]string, len(envVars))
			for _, ev := range envVars {
				name, _ := ev["name"].(string)
				value, _ := ev["value"].(string)
				if name != "" {
					envMap[name] = value
				}
			}

			Expect(envMap).To(HaveKey("PROVIDER_A_URL"), "PROVIDER_A_URL env var should exist in rendered Deployment")
			Expect(envMap["PROVIDER_A_URL"]).To(And(
				ContainSubstring(".svc.cluster.local"),
				ContainSubstring(":8080"),
				HavePrefix("http://"),
			), "PROVIDER_A_URL should be a valid service URL")

			Expect(envMap).To(HaveKey("PROVIDER_B_URL"), "PROVIDER_B_URL env var should exist in rendered Deployment")
			Expect(envMap["PROVIDER_B_URL"]).To(And(
				ContainSubstring(".svc.cluster.local"),
				ContainSubstring(":9090"),
				HavePrefix("http://"),
			), "PROVIDER_B_URL should be a valid service URL")
		})

		It("stores resolved connections in ReleaseBinding status", func() {
			By("verifying ReleaseBinding has 2 resolved connections and 2 connection targets")
			Eventually(func(g Gomega) {
				status := getReleaseBindingStatus(g, "consumer-development")
				g.Expect(status.ResolvedConnections).To(HaveLen(2), "expected 2 resolved connections")
				g.Expect(status.ConnectionTargets).To(HaveLen(2), "expected 2 connection targets")
			}, 3*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("keeps connections pending for non-existent endpoint", func() {
			By("deploying consumer-bad with connection to nonexistent component")
			output, err := framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
				cpNs, "proj1", "consumer-bad", "deployment/service",
				"hashicorp/http-echo",
				[]string{"-text=consumer-bad", "-listen=:3000"},
				map[string]endpointDef{
					"web": {epType: "HTTP", port: 3000, visibility: []string{"project"}},
				},
				[]connectionDef{
					{
						component:  "nonexistent",
						endpoint:   "api",
						visibility: "project",
						envURL:     "BAD_URL",
					},
				},
			))
			Expect(err).NotTo(HaveOccurred(), "failed to create consumer-bad: %s", output)

			By("consumer-bad ReleaseBinding should have ConnectionsResolved=False")
			assertRBCondition("consumer-bad-development", "ConnectionsResolved", "False", "ConnectionsPending")
			// Ready=False is expected, but the reason may vary (ConnectionsPending or ReleaseSynced)
			// depending on which sub-condition is evaluated first.
			assertRBConditionStatus("consumer-bad-development", "Ready", "False")

			By("consumer-bad's own endpoint should still have serviceURL")
			assertRBEndpointServiceURL("consumer-bad-development", "web", 3000)
		})

		It("clears connection status when connections are removed", func() {
			By("recording current ComponentRelease name")
			var originalRelease string
			Eventually(func(g Gomega) {
				var err error
				originalRelease, err = framework.KubectlGetJsonpath(
					kubeContext, cpNs, "component", "consumer",
					"{.status.latestRelease.name}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(originalRelease).NotTo(BeEmpty())
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("re-applying consumer workload without connections")
			output, err := framework.KubectlApplyLiteral(kubeContext, workloadOnlyYAML(
				cpNs, "proj1", "consumer",
				"hashicorp/http-echo",
				[]string{"-text=consumer-no-conn", "-listen=:3000"},
				map[string]endpointDef{
					"web": {epType: "HTTP", port: 3000, visibility: []string{"project"}},
				},
				nil,
			))
			Expect(err).NotTo(HaveOccurred(), "failed to update consumer workload: %s", output)

			By("waiting for a new ComponentRelease to be created")
			Eventually(func(g Gomega) {
				currentRelease, err := framework.KubectlGetJsonpath(
					kubeContext, cpNs, "component", "consumer",
					"{.status.latestRelease.name}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(currentRelease).NotTo(BeEmpty())
				g.Expect(currentRelease).NotTo(Equal(originalRelease),
					"expected new ComponentRelease after removing connections")
			}, 3*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying ConnectionsResolved=True with reason NoConnections")
			assertRBCondition("consumer-development", "ConnectionsResolved", "True", "NoConnections")
			assertRBCondition("consumer-development", "Ready", "True", "Ready")

			By("verifying connectionTargets is empty")
			Eventually(func(g Gomega) {
				status := getReleaseBindingStatus(g, "consumer-development")
				g.Expect(status.ConnectionTargets).To(BeEmpty(), "expected no connection targets after removing connections")
				g.Expect(status.ResolvedConnections).To(BeEmpty(), "expected no resolved connections after removing connections")
			}, 3*time.Minute, 2*time.Second).Should(Succeed())
		})
	})

})

// getReleaseBindingStatus fetches a ReleaseBinding as full JSON and returns its typed status.
func getReleaseBindingStatus(g Gomega, rbName string) openchoreov1alpha1.ReleaseBindingStatus {
	return getReleaseBindingStatusInNs(g, cpNs, rbName)
}

func getReleaseBindingStatusInNs(g Gomega, namespace, rbName string) openchoreov1alpha1.ReleaseBindingStatus {
	output, err := framework.Kubectl(
		kubeContext,
		"get", "releasebinding", rbName,
		"-n", namespace,
		"-o", "json",
	)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get ReleaseBinding %s in %s", rbName, namespace)

	var rb openchoreov1alpha1.ReleaseBinding
	g.Expect(json.Unmarshal([]byte(output), &rb)).To(Succeed(), "failed to unmarshal ReleaseBinding %s", rbName)
	return rb.Status
}
