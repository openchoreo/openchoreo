// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"

	"github.com/onsi/gomega"
)

// AssertDedicatedServiceAccount checks that the first object of the given kind matching the
// label selector has a pod spec (at podSpecPath) running as its own ServiceAccount instead of
// the namespace default, with no API token mounted, and that the ServiceAccount exists with
// automountServiceAccountToken disabled. It returns the ServiceAccount name.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertDedicatedServiceAccount(g gomega.Gomega, kubeContext, namespace, kind, labelSelector, podSpecPath string) string {
	jp := func(field string) string {
		out, err := Kubectl(kubeContext,
			"get", kind,
			"-n", namespace,
			"-l", labelSelector,
			"-o", "jsonpath={.items[0]"+podSpecPath+field+"}",
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(),
			fmt.Sprintf("failed to get %s in %s with selector %q", kind, namespace, labelSelector))
		return out
	}

	saName := jp(".serviceAccountName")
	g.Expect(saName).NotTo(gomega.BeEmpty(),
		fmt.Sprintf("%s with selector %q has no serviceAccountName", kind, labelSelector))
	g.Expect(saName).NotTo(gomega.Equal("default"),
		fmt.Sprintf("%s with selector %q runs as the default ServiceAccount", kind, labelSelector))
	g.Expect(jp(".automountServiceAccountToken")).To(gomega.Equal("false"),
		fmt.Sprintf("%s with selector %q should set automountServiceAccountToken: false", kind, labelSelector))
	g.Expect(jp(".volumes[*].projected.sources[*].serviceAccountToken.path")).To(gomega.BeEmpty(),
		fmt.Sprintf("%s with selector %q should not mount a ServiceAccount token", kind, labelSelector))

	saAutomount, err := KubectlGetJsonpath(kubeContext, namespace, "serviceaccount", saName,
		"{.automountServiceAccountToken}")
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("ServiceAccount %s not found in %s", saName, namespace))
	g.Expect(saAutomount).To(gomega.Equal("false"),
		fmt.Sprintf("ServiceAccount %s should set automountServiceAccountToken: false", saName))
	return saName
}
