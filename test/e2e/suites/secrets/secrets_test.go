// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var dpNs string

var _ = Describe("Secrets and External Secrets", Ordered, Label("tier2"), func() {
	var (
		envESName     string
		fileESName    string
		envSecretK8s  string
		fileSecretK8s string
	)

	componentSelector := fmt.Sprintf(
		"openchoreo.dev/component=%s,openchoreo.dev/environment=%s",
		componentName, environmentName,
	)

	rbName := componentName + "-" + environmentName

	// --- Helpers ---

	assertRBCondition := func(condType, expectedStatus, expectedReason string) {
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
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
	}

	// listExternalSecrets returns ExternalSecret items as raw JSON maps in the data-plane namespace,
	// filtered by component labels.
	listExternalSecrets := func(ns string) ([]map[string]any, error) {
		output, err := framework.KubectlGet(
			kubeContext, ns, "externalsecret",
			"-l", componentSelector,
			"-o", "json",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list externalsecrets in %s: %w", ns, err)
		}

		var list struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal([]byte(output), &list); err != nil {
			return nil, fmt.Errorf("failed to unmarshal externalsecret list: %w", err)
		}
		return list.Items, nil
	}

	// findESBySecretKey finds the ExternalSecret whose spec.data contains the given secretKey.
	findESBySecretKey := func(items []map[string]any, secretKey string) map[string]any {
		for _, item := range items {
			spec, _ := item["spec"].(map[string]any)
			if spec == nil {
				continue
			}
			dataList, _ := spec["data"].([]any)
			for _, d := range dataList {
				entry, _ := d.(map[string]any)
				if entry == nil {
					continue
				}
				if sk, _ := entry["secretKey"].(string); sk == secretKey {
					return item
				}
			}
		}
		return nil
	}

	// esName extracts .metadata.name from a raw ExternalSecret map.
	esName := func(es map[string]any) string {
		md, _ := es["metadata"].(map[string]any)
		if md == nil {
			return ""
		}
		name, _ := md["name"].(string)
		return name
	}

	// esTargetName extracts .spec.target.name from a raw ExternalSecret map.
	esTargetName := func(es map[string]any) string {
		spec, _ := es["spec"].(map[string]any)
		if spec == nil {
			return ""
		}
		target, _ := spec["target"].(map[string]any)
		if target == nil {
			return ""
		}
		name, _ := target["name"].(string)
		return name
	}

	// esRemoteKey extracts remoteRef.key for a given secretKey from a raw ExternalSecret map.
	esRemoteKey := func(es map[string]any, secretKey string) string {
		spec, _ := es["spec"].(map[string]any)
		if spec == nil {
			return ""
		}
		dataList, _ := spec["data"].([]any)
		for _, d := range dataList {
			entry, _ := d.(map[string]any)
			if entry == nil {
				continue
			}
			if sk, _ := entry["secretKey"].(string); sk == secretKey {
				ref, _ := entry["remoteRef"].(map[string]any)
				if ref == nil {
					return ""
				}
				key, _ := ref["key"].(string)
				return key
			}
		}
		return ""
	}

	// assertExternalSecretReady polls until the ExternalSecret has Ready=True.
	assertExternalSecretReady := func(ns, name string) {
		Eventually(func(g Gomega) {
			framework.AssertJsonpathEquals(g, kubeContext, ns, "externalsecret", name,
				`{.status.conditions[?(@.type=="Ready")].status}`, "True")
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed(),
			"ExternalSecret %s/%s did not become Ready", ns, name)
	}

	// getK8sSecretValue reads a decoded value from a Kubernetes Secret.
	getK8sSecretValue := func(ns, secretName, key string) (string, error) {
		output, err := framework.Kubectl(kubeContext,
			"get", "secret", secretName, "-n", ns, "-o", "json",
		)
		if err != nil {
			return "", err
		}
		var secret struct {
			Data map[string]string `json:"data"`
		}
		if err := json.Unmarshal([]byte(output), &secret); err != nil {
			return "", fmt.Errorf("failed to unmarshal secret %s/%s: %w", ns, secretName, err)
		}
		encoded, ok := secret.Data[key]
		if !ok {
			return "", fmt.Errorf("key %q not found in secret %s/%s; available: %v",
				key, ns, secretName, mapKeys(secret.Data))
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 for key %q in secret %s/%s: %w", key, ns, secretName, err)
		}
		return string(decoded), nil
	}

	assertK8sSecretValue := func(ns, secretName, key, expected string) {
		Eventually(func(g Gomega) {
			val, err := getK8sSecretValue(ns, secretName, key)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(val).To(Equal(expected),
				"secret %s/%s key %q: got %q, want %q", ns, secretName, key, val, expected)
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
	}

	assertPodReadsEnv := func(ns, envVar, expected string) {
		Eventually(func(g Gomega) {
			output, err := framework.KubectlExecByLabel(kubeContext, ns, componentSelector, "main", "printenv", envVar)
			g.Expect(err).NotTo(HaveOccurred(), "failed to exec printenv %s in pod", envVar)
			g.Expect(strings.TrimSpace(output)).To(Equal(expected))
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
	}

	assertPodReadsFile := func(ns, path, expected string) {
		Eventually(func(g Gomega) {
			output, err := framework.KubectlExecByLabel(kubeContext, ns, componentSelector, "main", "cat", path)
			g.Expect(err).NotTo(HaveOccurred(), "failed to exec cat %s in pod", path)
			g.Expect(strings.TrimSpace(output)).To(Equal(expected))
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
	}

	getPodNames := func(ns string) ([]string, error) {
		output, err := framework.Kubectl(kubeContext,
			"get", "pod", "-n", ns,
			"-l", componentSelector,
			"-o", "jsonpath={.items[*].metadata.name}",
		)
		if err != nil {
			return nil, err
		}
		if output == "" {
			return nil, nil
		}
		return strings.Fields(output), nil
	}

	assertResourceGone := func(ns, resource, name string, timeout time.Duration) {
		Eventually(func(g Gomega) {
			output, err := framework.Kubectl(kubeContext,
				"get", resource, name, "-n", ns, "--ignore-not-found", "-o", "name")
			g.Expect(err).NotTo(HaveOccurred(), "kubectl get failed unexpectedly for %s/%s in %s", resource, name, ns)
			g.Expect(output).To(BeEmpty(), "%s/%s still exists in %s", resource, name, ns)
		}, timeout, framework.DefaultPolling).Should(Succeed(),
			"%s/%s should be deleted from %s", resource, name, ns)
	}

	assertNoExternalSecretsForComponent := func(ns string) {
		Eventually(func(g Gomega) {
			items, err := listExternalSecrets(ns)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(items).To(BeEmpty(), "expected no externalsecrets for component in %s, found %d", ns, len(items))
		}, 5*time.Minute, framework.DefaultPolling).Should(Succeed())
	}

	// --- Setup ---

	BeforeAll(func() {
		By("creating control plane namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)

		By("applying platform resources (DeploymentPipeline, Environment, Project)")
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		By("creating SecretReference for env secret (APP_USERNAME -> fake store key 'username')")
		output, err = framework.KubectlApplyLiteral(kubeContext,
			secretReferenceYAML("env-secret", cpNs, "APP_USERNAME", "username"))
		Expect(err).NotTo(HaveOccurred(), "failed to create env SecretReference: %s", output)

		By("creating SecretReference for file secret (password.txt -> fake store key 'password')")
		output, err = framework.KubectlApplyLiteral(kubeContext,
			secretReferenceYAML("file-secret", cpNs, "password.txt", "password"))
		Expect(err).NotTo(HaveOccurred(), "failed to create file SecretReference: %s", output)

		By("deploying Component and Workload with secretKeyRef env and file mounts")
		output, err = framework.KubectlApplyLiteral(kubeContext, componentAndWorkloadYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create Component and Workload: %s", output)

		By("waiting for data plane namespace discovery")
		Eventually(func() error {
			var discoverErr error
			dpNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, environmentName)
			return discoverErr
		}, framework.DefaultTimeout, 5*time.Second).Should(Succeed(), "dp namespace not found")
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: %s\n", dpNs)

		By("waiting for ReleaseBinding Ready=True")
		assertRBCondition("Ready", "True", "Ready")
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
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")
	})

	// --- Test Case 1: Full chain — env + file secrets synced via ESO into a running pod ---

	It("syncs env and file secrets through ESO into a running workload", func() {
		By("discovering ExternalSecret resources in data plane namespace")
		var esItems []map[string]any
		Eventually(func(g Gomega) {
			var err error
			esItems, err = listExternalSecrets(dpNs)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(len(esItems)).To(BeNumerically(">=", 2),
				"expected at least 2 ExternalSecrets (env + file), found %d", len(esItems))
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

		By("identifying env and file ExternalSecrets by secretKey")
		envES := findESBySecretKey(esItems, "APP_USERNAME")
		Expect(envES).NotTo(BeNil(), "env ExternalSecret with secretKey APP_USERNAME not found")
		fileES := findESBySecretKey(esItems, "password.txt")
		Expect(fileES).NotTo(BeNil(), "file ExternalSecret with secretKey password.txt not found")

		envESName = esName(envES)
		fileESName = esName(fileES)
		envSecretK8s = esTargetName(envES)
		fileSecretK8s = esTargetName(fileES)
		fmt.Fprintf(GinkgoWriter, "env ES: %s (target: %s)\n", envESName, envSecretK8s)
		fmt.Fprintf(GinkgoWriter, "file ES: %s (target: %s)\n", fileESName, fileSecretK8s)

		By("verifying ExternalSecret spec (secretStoreRef, remoteRef)")
		Expect(esRemoteKey(envES, "APP_USERNAME")).To(Equal("username"),
			"env ExternalSecret should reference fake store key 'username'")
		Expect(esRemoteKey(fileES, "password.txt")).To(Equal("password"),
			"file ExternalSecret should reference fake store key 'password'")

		By("waiting for both ExternalSecrets to become Ready")
		assertExternalSecretReady(dpNs, envESName)
		assertExternalSecretReady(dpNs, fileESName)

		By("verifying K8s Secret values match fake store values")
		assertK8sSecretValue(dpNs, envSecretK8s, "APP_USERNAME", "e2e-user")
		assertK8sSecretValue(dpNs, fileSecretK8s, "password.txt", "e2e-password")

		By("verifying pod reads env var APP_USERNAME = e2e-user")
		assertPodReadsEnv(dpNs, "APP_USERNAME", "e2e-user")

		By("verifying pod reads file /etc/secrets/password.txt = e2e-password")
		assertPodReadsFile(dpNs, "/etc/secrets/password.txt", "e2e-password")
	})

	// --- Test Case 2: SecretReference update triggers rollout gated on ESO sync ---

	It("updates ExternalSecret in-place and triggers rollout via dp-resource-hash", func() {
		By("updating SecretReference/env-secret to point at fake store key 'npm-token'")
		output, err := framework.KubectlApplyLiteral(kubeContext,
			secretReferenceYAML("env-secret", cpNs, "APP_USERNAME", "npm-token"))
		Expect(err).NotTo(HaveOccurred(), "failed to update env SecretReference: %s", output)

		By("waiting for ReleaseBinding to re-reconcile and reach Ready=True")
		assertRBCondition("Ready", "True", "Ready")

		By("waiting for ExternalSecret to be updated in-place with new remoteRef")
		// Current behavior: ExternalSecret name is derived from component+environment (not
		// content), so it's updated in-place. dp-resource-hash triggers a rolling update.
		//
		// Race condition: the rollout starts before ESO syncs the new value, so new pods
		// may read stale secrets. This passes here because the fake ESO provider syncs
		// near-instantly, but with a real backend (Vault, AWS SM) the sync delay would
		// cause pods to start with old values.
		//
		// Fix: include a content hash (remoteRef keys) in the ExternalSecret resource name.
		// A new name means a new K8s Secret, and pods block until ESO syncs it — naturally
		// gating the rollout on ESO readiness.
		Eventually(func(g Gomega) {
			esItems, err := listExternalSecrets(dpNs)
			g.Expect(err).NotTo(HaveOccurred())
			envES := findESBySecretKey(esItems, "APP_USERNAME")
			g.Expect(envES).NotTo(BeNil(), "env ExternalSecret not found after update")
			g.Expect(esRemoteKey(envES, "APP_USERNAME")).To(Equal("npm-token"),
				"env ExternalSecret should reference 'npm-token' after update")
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

		By("waiting for ExternalSecret to be Ready after in-place update")
		assertExternalSecretReady(dpNs, envESName)

		By("waiting for K8s Secret to have the updated value")
		assertK8sSecretValue(dpNs, envSecretK8s, "APP_USERNAME", "fake-npm-token")

		By("waiting for Deployment rollout to complete")
		Eventually(func(g Gomega) {
			framework.AssertRolloutComplete(g, kubeContext, dpNs, componentSelector, "5m")
		}, 5*time.Minute, framework.DefaultPolling).Should(Succeed())

		By("verifying new pod reads updated env var")
		assertPodReadsEnv(dpNs, "APP_USERNAME", "fake-npm-token")

		By("verifying file secret is unaffected")
		assertPodReadsFile(dpNs, "/etc/secrets/password.txt", "e2e-password")
	})

	// --- Test Case 3: Component deletion cascades cleanup to data plane ---

	It("cascades ExternalSecret and Kubernetes Secret cleanup when the Component is deleted", func() {
		By("recording resource names before deletion")
		Expect(envESName).NotTo(BeEmpty(), "env ExternalSecret name must be known from prior test")
		Expect(fileESName).NotTo(BeEmpty(), "file ExternalSecret name must be known from prior test")
		fmt.Fprintf(GinkgoWriter, "will verify cleanup of: envES=%s, fileES=%s, envK8s=%s, fileK8s=%s\n",
			envESName, fileESName, envSecretK8s, fileSecretK8s)

		By("deleting Component from control plane")
		output, err := framework.Kubectl(kubeContext,
			"delete", "component", componentName, "-n", cpNs, "--wait=false")
		Expect(err).NotTo(HaveOccurred(), "failed to delete component: %s", output)

		By("waiting for control plane cleanup")
		assertResourceGone(cpNs, "component", componentName, 5*time.Minute)
		assertResourceGone(cpNs, "releasebinding", rbName, 5*time.Minute)

		By("waiting for data plane ExternalSecret cleanup")
		assertNoExternalSecretsForComponent(dpNs)

		By("waiting for data plane K8s Secret cleanup (ESO owner deletion)")
		if envSecretK8s != "" {
			assertResourceGone(dpNs, "secret", envSecretK8s, 5*time.Minute)
		}
		if fileSecretK8s != "" {
			assertResourceGone(dpNs, "secret", fileSecretK8s, 5*time.Minute)
		}

		By("verifying no component pods remain")
		Eventually(func(g Gomega) {
			pods, err := getPodNames(dpNs)
			g.Expect(err).NotTo(HaveOccurred(), "failed to query pods")
			g.Expect(pods).To(BeEmpty(), "expected no pods for deleted component, found %v", pods)
		}, 5*time.Minute, framework.DefaultPolling).Should(Succeed())
	})
})

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
