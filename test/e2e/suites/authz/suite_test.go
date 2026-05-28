// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"flag"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var kubeContext string

func init() {
	flag.StringVar(&kubeContext, "e2e.kubecontext", "",
		"Kubernetes context for e2e tests (required)")
}

func TestE2EAuthz(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting OpenChoreo Authorization e2e suite\n")
	RunSpecs(t, "OpenChoreo E2E Authorization Suite")
}

// subjectClient talks to the API as customer-portal-client (no bootstrap binding).
// Tests vary its permissions by creating/deleting authz bindings.
var subjectClient *gen.ClientWithResponses

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

	By("waiting for API server to be reachable")
	framework.WaitForHTTP(apiURL+"/health", 60*time.Second)

	By("obtaining test subject token (customer-portal-client)")
	var subjectToken string
	Eventually(func() error {
		var tokenErr error
		subjectToken, tokenErr = framework.FetchClientCredentialsToken(
			tokenURL, subjectClientID, subjectClientSecret,
		)
		return tokenErr
	}, 60*time.Second, 5*time.Second).Should(Succeed(), "failed to obtain subject token")
	Expect(subjectToken).NotTo(BeEmpty())
	subjectClient = newAPIClient(subjectToken)

	By("seeding test namespace via kubectl")
	nsYAML := cpNamespaceYAML(testNs)
	output, err = framework.KubectlApplyLiteral(kubeContext, nsYAML)
	Expect(err).NotTo(HaveOccurred(), "create test namespace: %s", output)

	By("seeding platform resources in test namespace")
	output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML(testNs))
	Expect(err).NotTo(HaveOccurred(), "apply platform resources: %s", output)

	fmt.Fprintf(GinkgoWriter, "Authz e2e suite ready: ns=%s\n", testNs)
})

var _ = AfterSuite(func() {
	if kubeContext == "" {
		return
	}
	if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
		By("skipping cleanup (E2E_KEEP_RESOURCES=true)")
		return
	}

	By("cleaning up test namespace")
	if testNs != "" {
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", testNs, "--ignore-not-found", "--wait=false")
	}

	By("cleaning up cluster-scoped authz resources")
	_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrolebinding", "-l", labelSelector(), "--ignore-not-found", "--wait=false")
	_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", "-l", labelSelector(), "--ignore-not-found", "--wait=false")
})
