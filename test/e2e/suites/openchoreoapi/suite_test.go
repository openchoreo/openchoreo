// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"flag"
	"fmt"
	"net"
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var kubeContext string

func init() {
	flag.StringVar(&kubeContext, "e2e.kubecontext", "",
		"Kubernetes context for e2e tests (required)")
}

func TestE2EOpenChoreoAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting OpenChoreo API e2e suite\n")
	RunSpecs(t, "OpenChoreo E2E API Suite")
}

var _ = BeforeSuite(func() {
	Expect(kubeContext).NotTo(BeEmpty(), "--e2e.kubecontext is required")
	fmt.Fprintf(GinkgoWriter, "Using kube context: %s\n", kubeContext)

	By("verifying e2e host entries resolve to 127.0.0.1")
	for _, host := range []string{"api.e2e-cp.local", "thunder.e2e-cp.local"} {
		addrs, err := net.LookupHost(host)
		Expect(err).NotTo(HaveOccurred(),
			"%s does not resolve — add '127.0.0.1 api.e2e-cp.local thunder.e2e-cp.local' to /etc/hosts", host)
		Expect(addrs).To(ContainElement("127.0.0.1"),
			"%s resolves to %v instead of 127.0.0.1 — check /etc/hosts", host, addrs)
	}

	By("verifying cluster is accessible")
	output, err := framework.Kubectl(kubeContext, "cluster-info")
	Expect(err).NotTo(HaveOccurred(),
		"cluster not accessible with context %s:\n%s", kubeContext, output)
	fmt.Fprintf(GinkgoWriter, "Cluster accessible\n")
})
