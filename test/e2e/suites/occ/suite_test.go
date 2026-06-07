// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"flag"
	"fmt"
	"net"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var kubeContext string

// Shared state initialised in BeforeSuite and used by test cases.
var occ *framework.OCCRunner

const (
	cpNamespace = "openchoreo-control-plane"

	apiURL   = "http://api.e2e-cp.local:28080"
	tokenURL = "http://thunder.e2e-cp.local:28080/oauth2/token"

	// openchoreo-system-app, NOT customer-portal-client: the authz suite owns
	// customer-portal-client and asserts exact grants/revocations for it, and
	// suite packages run in parallel within a tier — the cluster-wide admin
	// binding this suite creates for its subject would leak into those
	// assertions (seen as a tier2 flake in CI).
	clientID     = "openchoreo-system-app"
	clientSecret = "openchoreo-system-app-secret"
)

func init() {
	flag.StringVar(&kubeContext, "e2e.kubecontext", "",
		"Kubernetes context for e2e tests (required)")
}

func TestE2EOCC(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting OpenChoreo OCC CLI e2e suite\n")
	RunSpecs(t, "OpenChoreo E2E OCC Suite")
}

var _ = BeforeSuite(func() {
	Expect(kubeContext).NotTo(BeEmpty(), "--e2e.kubecontext is required")
	fmt.Fprintf(GinkgoWriter, "Using kube context: %s\n", kubeContext)

	By("verifying e2e host entries resolve to 127.0.0.1")
	for _, host := range []string{"api.e2e-cp.local", "thunder.e2e-cp.local", "openchoreo.e2e-cp.local"} {
		addrs, err := net.LookupHost(host)
		Expect(err).NotTo(HaveOccurred(),
			"%s does not resolve — add '127.0.0.1 api.e2e-cp.local thunder.e2e-cp.local openchoreo.e2e-cp.local' to /etc/hosts", host)
		Expect(addrs).To(ContainElement("127.0.0.1"),
			"%s resolves to %v instead of 127.0.0.1 — check /etc/hosts", host, addrs)
	}

	By("verifying cluster is accessible")
	out, err := framework.Kubectl(kubeContext, "cluster-info")
	Expect(err).NotTo(HaveOccurred(),
		"cluster not accessible with context %s:\n%s", kubeContext, out)

	By("granting admin ABAC role to the client_credentials subject")
	abacBinding := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: %s
spec:
  roleMappings:
    - roleRef:
        name: admin
        kind: ClusterAuthzRole
  entitlement:
    claim: sub
    value: %s
  effect: allow`, clusterAuthzBindingName, clientID)
	out, err = framework.KubectlApplyLiteral(kubeContext, abacBinding)
	Expect(err).NotTo(HaveOccurred(), "failed to create ABAC binding: %s", out)

	By("waiting for gateway to be reachable")
	framework.WaitForHTTP(apiURL+"/.well-known/oauth-protected-resource", 60*time.Second)

	By("obtaining OAuth2 token from Thunder")
	token, err := framework.FetchClientCredentialsToken(tokenURL, clientID, clientSecret)
	Expect(err).NotTo(HaveOccurred(), "failed to obtain access token")
	Expect(token).NotTo(BeEmpty(), "access token is empty")
	fmt.Fprintf(GinkgoWriter, "Obtained access token (length=%d)\n", len(token))

	By("building occ binary")
	runner, err := framework.NewOCCRunner(apiURL)
	Expect(err).NotTo(HaveOccurred(), "failed to create OCC runner")
	Expect(runner.Build()).To(Succeed(), "failed to build occ binary")
	occ = runner

	By("seeding occ config with token")
	Expect(occ.SeedConfig(token)).To(Succeed(), "failed to seed occ config")
	fmt.Fprintf(GinkgoWriter, "OCC runner ready: binary=%s home=%s api=%s\n",
		occ.BinaryPath, occ.HomeDir, occ.APIServer)
})

var _ = AfterSuite(func() {
	if occ != nil {
		occ.Cleanup()
	}
	framework.Kubectl(kubeContext, "delete", "clusterauthzrolebinding", clusterAuthzBindingName, "--ignore-not-found", "--wait=false") //nolint:errcheck
	framework.Kubectl(kubeContext, "delete", "clusterauthzrole", clusterAuthzRoleName, "--ignore-not-found", "--wait=false")           //nolint:errcheck
})
