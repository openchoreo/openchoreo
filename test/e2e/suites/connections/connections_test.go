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

var dpNs string // data plane namespace for proj1/development

var _ = Describe("Connection URL Resolution", Ordered, func() {
	// assertRBCondition checks a ReleaseBinding condition via jsonpath.
	assertRBCondition := func(rbName, condType, expectedStatus, expectedReason string) {
		Eventually(func(g Gomega) {
			status, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "releasebinding", rbName,
				fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].status}`, condType),
			)
			g.Expect(err).NotTo(HaveOccurred(), "failed to get condition %s on ReleaseBinding %s", condType, rbName)
			g.Expect(status).To(Equal(expectedStatus),
				"expected condition %s status=%s on ReleaseBinding %s", condType, expectedStatus, rbName)

			reason, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "releasebinding", rbName,
				fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].reason}`, condType),
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(reason).To(Equal(expectedReason),
				"expected condition %s reason=%s on ReleaseBinding %s", condType, expectedReason, rbName)
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
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

	// getReleaseDeploymentEnv retrieves env vars from the rendered Deployment in a Release.
	getReleaseDeploymentEnv := func(componentName string) []map[string]any {
		output, err := framework.Kubectl(
			kubeContext,
			"get", "release",
			"-n", cpNs,
			"-l", fmt.Sprintf("openchoreo.dev/component=%s,openchoreo.dev/environment=development", componentName),
			"-o", "json",
		)
		Expect(err).NotTo(HaveOccurred(), "failed to get Release for %s", componentName)

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
		Expect(releaseList.Items).To(HaveLen(1), "expected exactly 1 Release for component %s, got %d", componentName, len(releaseList.Items))

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

		Fail(fmt.Sprintf("no Deployment resource found in Release for component %s", componentName))
		return nil
	}

	BeforeAll(func() {
		By("reading clientCA from existing DataPlane 'default'")
		clientCA, err := framework.KubectlGetJsonpath(
			kubeContext, "default", "dataplane", "default",
			"{.spec.clusterAgent.clientCA.value}",
		)
		Expect(err).NotTo(HaveOccurred(), "failed to read DataPlane default")
		Expect(clientCA).NotTo(BeEmpty(), "DataPlane default clientCA is empty")

		By("creating control plane namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)

		By("creating DataPlane")
		output, err = framework.KubectlApplyLiteral(kubeContext, dataPlaneYAML(cpNs, clientCA))
		Expect(err).NotTo(HaveOccurred(), "failed to create DataPlane in %s: %s", cpNs, output)

		By("applying platform resources")
		output, err = framework.KubectlApplyLiteral(kubeContext,
			platformResourcesYAML(cpNs, []string{"development", "staging"}, []string{"proj1"}))
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		By("applying ComponentType e2e-conn-service")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentTypeYAML(cpNs))
		Expect(err).NotTo(HaveOccurred(), "failed to apply ComponentType: %s", output)

		By("deploying provider-a (HTTP:8080, project+namespace visibility)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "provider-a", "deployment/e2e-conn-service",
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
			cpNs, "proj1", "provider-b", "deployment/e2e-conn-service",
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

		By("deploying consumer with connections to both providers")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "consumer", "deployment/e2e-conn-service",
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

		By("cleaning up data plane namespace")
		if dpNs != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", dpNs, "--ignore-not-found", "--wait=false")
		}

		By("cleaning up control plane namespace")
		_, _ = framework.Kubectl(kubeContext, "delete", "dataplane", dataPlane, "-n", cpNs, "--ignore-not-found")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")
	})

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

	It("renders connection env vars in the Release Deployment", func() {
		By("waiting for consumer connections to resolve first")
		assertRBCondition("consumer-development", "ConnectionsResolved", "True", "AllConnectionsResolved")

		By("checking rendered Release for connection env vars")
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
			cpNs, "proj1", "consumer-bad", "deployment/e2e-conn-service",
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
			targetsJSON, err := framework.KubectlGetJsonpath(
				kubeContext, cpNs, "releasebinding", "consumer-development",
				"{.status.connectionTargets}",
			)
			g.Expect(err).NotTo(HaveOccurred())
			// Empty jsonpath returns empty string, nil slice returns empty string
			g.Expect(targetsJSON).To(BeEmpty(), "expected no connection targets after removing connections")
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	})
})

// getReleaseBindingStatus fetches a ReleaseBinding as full JSON and returns its typed status.
// This avoids jsonpath-based unmarshalling which can produce non-JSON output for array fields.
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

var cpNs2 = fmt.Sprintf("e2e-conn2-%s", connRunID)
var dpNs2 string

var _ = Describe("Internal Visibility Connection Resolution", Ordered, func() {
	// assertRBConditionInNs checks a ReleaseBinding condition in a specific namespace.
	assertRBConditionInNs := func(namespace, rbName, condType, expectedStatus, expectedReason string) {
		Eventually(func(g Gomega) {
			status, err := framework.KubectlGetJsonpath(
				kubeContext, namespace, "releasebinding", rbName,
				fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].status}`, condType),
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status).To(Equal(expectedStatus))

			reason, err := framework.KubectlGetJsonpath(
				kubeContext, namespace, "releasebinding", rbName,
				fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].reason}`, condType),
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(reason).To(Equal(expectedReason))
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	}

	BeforeAll(func() {
		By("reading clientCA from existing DataPlane 'default'")
		clientCA, err := framework.KubectlGetJsonpath(
			kubeContext, "default", "dataplane", "default",
			"{.spec.clusterAgent.clientCA.value}",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(clientCA).NotTo(BeEmpty())

		By("creating second control plane namespace for cross-namespace tests")
		ns2 := fmt.Sprintf(
			`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    openchoreo.dev/controlplane-namespace: "true"`, cpNs2)
		output, err := framework.KubectlApplyLiteral(kubeContext, ns2)
		Expect(err).NotTo(HaveOccurred(), "failed to create second CP namespace: %s", output)

		By("creating DataPlane in second namespace")
		output, err = framework.KubectlApplyLiteral(kubeContext, dataPlaneYAML(cpNs2, clientCA))
		Expect(err).NotTo(HaveOccurred(), "failed to create DataPlane in %s: %s", cpNs2, output)

		By("applying platform resources in second namespace")
		output, err = framework.KubectlApplyLiteral(kubeContext,
			platformResourcesYAML(cpNs2, []string{"development"}, []string{"proj2"}))
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources in %s: %s", cpNs2, output)

		By("applying ComponentType in second namespace")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentTypeYAML(cpNs2))
		Expect(err).NotTo(HaveOccurred(), "failed to apply ComponentType in %s: %s", cpNs2, output)

		By("deploying cross-ns-provider in second namespace (HTTP:7070, internal visibility)")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs2, "proj2", "cross-ns-provider", "deployment/e2e-conn-service",
			"hashicorp/http-echo",
			[]string{"-text=cross-ns-provider", "-listen=:7070"},
			map[string]endpointDef{
				"api": {epType: "HTTP", port: 7070, visibility: []string{"project", "internal"}},
			},
			nil,
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create cross-ns-provider: %s", output)

		By("waiting for dp namespace for proj2/development")
		Eventually(func() error {
			var discoverErr error
			dpNs2, discoverErr = framework.GetDPNamespace(kubeContext, cpNs2, "proj2", "development")
			return discoverErr
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace for ns2: %s\n", dpNs2)

		By("waiting for cross-ns-provider to be Ready")
		assertRBConditionInNs(cpNs2, "cross-ns-provider-development", "Ready", "True", "Ready")
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			return
		}
		if dpNs2 != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", dpNs2, "--ignore-not-found", "--wait=false")
		}
		_, _ = framework.Kubectl(kubeContext, "delete", "dataplane", dataPlane, "-n", cpNs2, "--ignore-not-found")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs2, "--ignore-not-found", "--wait=false")
	})

	It("resolves internal visibility connection within same namespace", func() {
		By("deploying internal-consumer in first namespace with internal connection to provider-a")
		output, err := framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "internal-consumer", "deployment/e2e-conn-service",
			"hashicorp/http-echo",
			[]string{"-text=internal-consumer", "-listen=:3001"},
			map[string]endpointDef{
				"web": {epType: "HTTP", port: 3001, visibility: []string{"project"}},
			},
			[]connectionDef{
				{
					component:  "provider-a",
					endpoint:   "api",
					visibility: "internal",
					envURL:     "INTERNAL_PROVIDER_A_URL",
				},
			},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create internal-consumer: %s", output)

		By("internal-consumer should resolve the internal connection")
		assertRBConditionInNs(cpNs, "internal-consumer-development", "ConnectionsResolved", "True", "AllConnectionsResolved")
	})

	It("resolves cross-namespace internal visibility connection", func() {
		By("deploying cross-ns-consumer in first namespace with connection to second namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "cross-ns-consumer", "deployment/e2e-conn-service",
			"hashicorp/http-echo",
			[]string{"-text=cross-ns-consumer", "-listen=:3002"},
			map[string]endpointDef{
				"web": {epType: "HTTP", port: 3002, visibility: []string{"project"}},
			},
			[]connectionDef{
				{
					namespace:  cpNs2,
					project:    "proj2",
					component:  "cross-ns-provider",
					endpoint:   "api",
					visibility: "internal",
					envURL:     "CROSS_NS_PROVIDER_URL",
				},
			},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create cross-ns-consumer: %s", output)

		By("cross-ns-consumer should resolve the cross-namespace connection")
		assertRBConditionInNs(cpNs, "cross-ns-consumer-development", "ConnectionsResolved", "True", "AllConnectionsResolved")

		By("verifying connection target has correct namespace and environment")
		Eventually(func(g Gomega) {
			status := getReleaseBindingStatusInNs(g, cpNs, "cross-ns-consumer-development")
			g.Expect(status.ConnectionTargets).To(HaveLen(1))
			g.Expect(status.ConnectionTargets[0].Namespace).To(Equal(cpNs2))
			g.Expect(status.ConnectionTargets[0].Project).To(Equal("proj2"))
			g.Expect(status.ConnectionTargets[0].Component).To(Equal("cross-ns-provider"))
			g.Expect(status.ConnectionTargets[0].Environment).To(Equal("development"))
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	})

	It("resolves cross-namespace connection with environment mapping", func() {
		By("deploying mapped-consumer with environment mapping")
		output, err := framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "mapped-consumer", "deployment/e2e-conn-service",
			"hashicorp/http-echo",
			[]string{"-text=mapped-consumer", "-listen=:3003"},
			map[string]endpointDef{
				"web": {epType: "HTTP", port: 3003, visibility: []string{"project"}},
			},
			[]connectionDef{
				{
					namespace:  cpNs2,
					project:    "proj2",
					component:  "cross-ns-provider",
					endpoint:   "api",
					visibility: "internal",
					envURL:     "MAPPED_PROVIDER_URL",
					environmentMapping: map[string]string{
						"development": "development",
						"staging":     "development",
					},
				},
			},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create mapped-consumer: %s", output)

		By("mapped-consumer should resolve the connection using environment mapping")
		assertRBConditionInNs(cpNs, "mapped-consumer-development", "ConnectionsResolved", "True", "AllConnectionsResolved")

		By("verifying connection target has mapped environment")
		Eventually(func(g Gomega) {
			status := getReleaseBindingStatusInNs(g, cpNs, "mapped-consumer-development")
			g.Expect(status.ConnectionTargets).To(HaveLen(1))
			g.Expect(status.ConnectionTargets[0].Environment).To(Equal("development"))
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	})

	It("keeps cross-namespace connection pending when target does not exist", func() {
		By("deploying consumer with connection to nonexistent namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, componentWithConnectionsYAML(
			cpNs, "proj1", "bad-cross-ns", "deployment/e2e-conn-service",
			"hashicorp/http-echo",
			[]string{"-text=bad-cross-ns", "-listen=:3004"},
			map[string]endpointDef{
				"web": {epType: "HTTP", port: 3004, visibility: []string{"project"}},
			},
			[]connectionDef{
				{
					namespace:  "nonexistent-ns",
					project:    "nonexistent-proj",
					component:  "nonexistent-comp",
					endpoint:   "api",
					visibility: "internal",
					envURL:     "BAD_CROSS_NS_URL",
				},
			},
		))
		Expect(err).NotTo(HaveOccurred(), "failed to create bad-cross-ns: %s", output)

		By("bad-cross-ns should have pending connections")
		assertRBConditionInNs(cpNs, "bad-cross-ns-development", "ConnectionsResolved", "False", "ConnectionsPending")
	})
})
