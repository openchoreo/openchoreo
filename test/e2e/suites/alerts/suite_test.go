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

func wpCtx() string {
	if wpKubeContext != "" {
		return wpKubeContext
	}
	return kubeContext
}

func opCtx() string {
	if opKubeContext != "" {
		return opKubeContext
	}
	return kubeContext
}

func TestE2EAlerts(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting OpenChoreo alerts e2e suite\n")
	RunSpecs(t, "OpenChoreo E2E Alerts Suite")
}

var _ = BeforeSuite(func() {
	Expect(kubeContext).NotTo(BeEmpty(), "--e2e.kubecontext is required")
	fmt.Fprintf(GinkgoWriter, "Using kube context: %s\n", kubeContext)

	By("verifying cluster is accessible")
	output, err := framework.Kubectl(kubeContext, "cluster-info")
	Expect(err).NotTo(HaveOccurred(),
		"cluster not accessible with context %s:\n%s", kubeContext, output)

	By("verifying the observer service is present")
	_, err = framework.Kubectl(opCtx(),
		"get", "svc", framework.ObserverService, "-n", framework.ObserverNamespace)
	Expect(err).NotTo(HaveOccurred(),
		"observability plane is not installed; run make e2e.setup with E2E_WITH_OBSERVABILITY=true or make e2e.multi")
})
