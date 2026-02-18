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

// AssertResourceExists checks that a named resource exists in the namespace.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertResourceExists(g gomega.Gomega, kubeContext, namespace, resource, name string) {
	_, err := KubectlGetJsonpath(kubeContext, namespace, resource, name, "{.metadata.name}")
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("%s/%s should exist in namespace %s", resource, name, namespace))
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
