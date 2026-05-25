// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"
	"strings"

	"github.com/onsi/gomega"
)

// AssertHTTPRouteAccepted asserts that the named HTTPRoute has at least one
// parent reporting Accepted=True in status.parents[].conditions.
func AssertHTTPRouteAccepted(g gomega.Gomega, kubeContext, namespace, name string) {
	out, err := KubectlGetJsonpath(
		kubeContext, namespace,
		"httproute.gateway.networking.k8s.io", name,
		`{.status.parents[0].conditions[?(@.type=="Accepted")].status}`,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		"failed to read HTTPRoute %s/%s accepted condition", namespace, name)
	g.Expect(out).To(gomega.Equal("True"),
		"HTTPRoute %s/%s should be Accepted", namespace, name)
}

// CountHTTPRoutesByLabel returns the number of HTTPRoutes matching a label selector.
func CountHTTPRoutesByLabel(kubeContext, namespace, labelSelector string) (int, error) {
	out, err := Kubectl(kubeContext,
		"get", "httproute.gateway.networking.k8s.io",
		"-n", namespace,
		"-l", labelSelector,
		"-o", "name",
	)
	if err != nil {
		return 0, fmt.Errorf("failed to list httproutes in %s with selector %s: %w", namespace, labelSelector, err)
	}
	if strings.TrimSpace(out) == "" {
		return 0, nil
	}
	return len(strings.Split(strings.TrimSpace(out), "\n")), nil
}

// GetHTTPRouteNames returns the names of HTTPRoutes matching a label selector.
func GetHTTPRouteNames(kubeContext, namespace, labelSelector string) ([]string, error) {
	out, err := Kubectl(kubeContext,
		"get", "httproute.gateway.networking.k8s.io",
		"-n", namespace,
		"-l", labelSelector,
		"-o", "jsonpath={.items[*].metadata.name}",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list httproutes in %s with selector %s: %w", namespace, labelSelector, err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	return strings.Fields(trimmed), nil
}
