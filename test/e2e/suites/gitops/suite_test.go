// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"flag"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var (
	kubeContext   string
	dpKubeContext string
	wpKubeContext string
	opKubeContext string
)

func init() {
	flag.StringVar(&kubeContext, "e2e.kubecontext", "",
		"Kubernetes context for the control plane cluster (required)")
	flag.StringVar(&dpKubeContext, "e2e.dp-kubecontext", "",
		"Kubernetes context for the data plane cluster (multi-cluster only)")
	flag.StringVar(&wpKubeContext, "e2e.wp-kubecontext", "",
		"Kubernetes context for the workflow plane cluster (multi-cluster only)")
	flag.StringVar(&opKubeContext, "e2e.op-kubecontext", "",
		"Kubernetes context for the observability plane cluster (multi-cluster only)")
}

func dpCtx() string {
	if dpKubeContext != "" {
		return dpKubeContext
	}
	return kubeContext
}

func TestE2EGitOps(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting OpenChoreo gitops e2e suite\n")
	RunSpecs(t, "OpenChoreo E2E GitOps Suite")
}

var _ = BeforeSuite(func() {
	Expect(kubeContext).NotTo(BeEmpty(), "--e2e.kubecontext is required")
	fmt.Fprintf(GinkgoWriter, "Using kube context: %s\n", kubeContext)

	By("verifying cluster is accessible")
	output, err := framework.Kubectl(kubeContext, "cluster-info")
	Expect(err).NotTo(HaveOccurred(),
		"cluster not accessible with context %s:\n%s", kubeContext, output)
})
