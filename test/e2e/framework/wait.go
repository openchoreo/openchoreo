// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"
	"strings"
	"time"

	"github.com/onsi/gomega"
)

const (
	DefaultTimeout = 3 * time.Minute
	DefaultPolling = 2 * time.Second
)

// PodStatus holds parsed pod information.
type PodStatus struct {
	Name     string
	Phase    string
	Restarts string
}

// GetPodStatuses returns the status of all non-completed pods in a namespace.
// Completed pods (Succeeded/Failed) are filtered out via field-selector so
// Job pods like the CA-extractor don't cause false failures.
func GetPodStatuses(kubeContext, namespace string) ([]PodStatus, error) {
	output, err := KubectlGet(kubeContext, namespace, "pods",
		"--field-selector=status.phase!=Succeeded,status.phase!=Failed",
		"-o", "custom-columns=NAME:.metadata.name,PHASE:.status.phase,RESTARTS:.status.containerStatuses[0].restartCount",
		"--no-headers")
	if err != nil {
		return nil, err
	}

	var pods []PodStatus
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		restarts := "<none>"
		if len(fields) >= 3 {
			restarts = fields[2]
		}
		pods = append(pods, PodStatus{
			Name:     fields[0],
			Phase:    fields[1],
			Restarts: restarts,
		})
	}
	return pods, nil
}

// AssertAllPodsRunning checks every non-completed pod in the namespace is Running.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertAllPodsRunning(g gomega.Gomega, kubeContext, namespace string) {
	pods, err := GetPodStatuses(kubeContext, namespace)
	g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get pods in %s", namespace)
	g.Expect(pods).NotTo(gomega.BeEmpty(), "no pods found in %s", namespace)

	for _, pod := range pods {
		g.Expect(pod.Phase).To(gomega.Equal("Running"),
			fmt.Sprintf("pod %s in %s is %s (restarts: %s)", pod.Name, namespace, pod.Phase, pod.Restarts))
	}
}

// GetDPNamespace discovers a data plane namespace by its control plane labels.
// DP namespace names include a hash suffix and cannot be predicted, so we
// query by label selectors instead.
func GetDPNamespace(kubeContext, cpNamespace, project, environment string) (string, error) {
	selector := fmt.Sprintf(
		"openchoreo.dev/namespace=%s,openchoreo.dev/project=%s,openchoreo.dev/environment=%s",
		cpNamespace, project, environment,
	)
	output, err := Kubectl(kubeContext,
		"get", "namespace",
		"-l", selector,
		"-o", "jsonpath={.items[0].metadata.name}",
	)
	if err != nil {
		return "", fmt.Errorf("failed to find dp namespace for cp=%s project=%s env=%s: %w", cpNamespace, project, environment, err)
	}
	if output == "" {
		return "", fmt.Errorf("no dp namespace found for cp=%s project=%s env=%s", cpNamespace, project, environment)
	}
	return output, nil
}

// AssertResourceExists checks that a named resource exists in the namespace.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertResourceExists(g gomega.Gomega, kubeContext, namespace, resource, name string) {
	_, err := KubectlGetJsonpath(kubeContext, namespace, resource, name, "{.metadata.name}")
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("%s/%s should exist in namespace %s", resource, name, namespace))
}

// AssertClusterResourceExists checks that a cluster-scoped resource exists.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertClusterResourceExists(g gomega.Gomega, kubeContext, resource, name string) {
	_, err := Kubectl(kubeContext, "get", resource, name, "-o", "jsonpath={.metadata.name}")
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("%s/%s should exist (cluster-scoped)", resource, name))
}

// AssertJsonpathEquals checks that a jsonpath value on a resource matches the expected string.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertJsonpathEquals(g gomega.Gomega, kubeContext, namespace, resource, name, jsonpath, expected string) {
	output, err := KubectlGetJsonpath(kubeContext, namespace, resource, name, jsonpath)
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("failed to get %s on %s/%s in %s", jsonpath, resource, name, namespace))
	g.Expect(output).To(gomega.Equal(expected),
		fmt.Sprintf("%s/%s jsonpath %s: got %q, want %q", resource, name, jsonpath, output, expected))
}

// AssertReleaseBindingReady checks that a ReleaseBinding has condition Ready=True.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertReleaseBindingReady(g gomega.Gomega, kubeContext, namespace, name string) {
	AssertJsonpathEquals(g, kubeContext, namespace, "releasebinding", name,
		`{.status.conditions[?(@.type=="Ready")].status}`, "True")
}

// AssertPodsRunning checks that at least one pod matches the label selector in the namespace
// and that every non-completed pod for that selector is in phase Running.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertPodsRunning(g gomega.Gomega, kubeContext, namespace, labelSelector string) {
	output, err := KubectlGet(kubeContext, namespace, "pods",
		"-l", labelSelector,
		"--field-selector=status.phase!=Succeeded,status.phase!=Failed",
		"-o", "custom-columns=NAME:.metadata.name,PHASE:.status.phase",
		"--no-headers")
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("failed to query pods in %s with selector %q", namespace, labelSelector))
	g.Expect(strings.TrimSpace(output)).NotTo(gomega.BeEmpty(),
		fmt.Sprintf("no pods in %s matching selector %q", namespace, labelSelector))
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		g.Expect(fields[1]).To(gomega.Equal("Running"),
			fmt.Sprintf("pod %s in %s is %s (selector %q)", fields[0], namespace, fields[1], labelSelector))
	}
}

// AssertRolloutComplete discovers a Deployment by label selector and waits for its rollout
// to finish. Returns immediately if the rollout is already complete.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertRolloutComplete(g gomega.Gomega, kubeContext, namespace, labelSelector, timeout string) {
	names, err := Kubectl(kubeContext,
		"get", "deployment", "-n", namespace,
		"-l", labelSelector,
		"-o", "jsonpath={.items[*].metadata.name}",
	)
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("failed to find deployment with selector %q in %s", labelSelector, namespace))
	g.Expect(names).NotTo(gomega.BeEmpty(),
		fmt.Sprintf("no deployment found with selector %q in %s", labelSelector, namespace))

	fields := strings.Fields(names)
	g.Expect(fields).To(gomega.HaveLen(1),
		fmt.Sprintf("expected exactly 1 deployment with selector %q in %s, found %d: %v",
			labelSelector, namespace, len(fields), fields))

	err = KubectlRolloutStatus(kubeContext, namespace, "deployment/"+fields[0], timeout)
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("rollout not complete for deployment/%s in %s", fields[0], namespace))
}

// AssertResourceGone checks that a named resource does not exist in the namespace.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertResourceGone(g gomega.Gomega, kubeContext, namespace, resource, name string) {
	_, err := KubectlGetJsonpath(kubeContext, namespace, resource, name, "{.metadata.name}")
	g.Expect(err).To(gomega.HaveOccurred(),
		fmt.Sprintf("%s/%s should not exist in namespace %s", resource, name, namespace))
	g.Expect(err.Error()).To(gomega.ContainSubstring("NotFound"),
		fmt.Sprintf("expected NotFound for %s/%s in %s, got: %v", resource, name, namespace, err))
}

// AssertNamespaceGone checks that a namespace does not exist.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertNamespaceGone(g gomega.Gomega, kubeContext, namespace string) {
	_, err := Kubectl(kubeContext, "get", "namespace", namespace, "-o", "jsonpath={.metadata.name}")
	g.Expect(err).To(gomega.HaveOccurred(),
		fmt.Sprintf("namespace %s should not exist", namespace))
	g.Expect(err.Error()).To(gomega.ContainSubstring("NotFound"),
		fmt.Sprintf("expected NotFound for namespace %s, got: %v", namespace, err))
}
