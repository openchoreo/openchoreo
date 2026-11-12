// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"flag"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var kubeContext string

var occ *framework.OCCRunner

const (
	apiURL   = "http://api.e2e-cp.local:28080"
	tokenURL = "http://thunder.e2e-cp.local:28080/oauth2/token"

	// The same client the occ suite uses, with a binding of its own: the
	// authz suite asserts exact grants for customer-portal-client.
	clientID     = "openchoreo-system-app"
	clientSecret = "openchoreo-system-app-secret"
)

var clusterAuthzBindingName = "endpointrollout-e2e-binding-" + runID

func init() {
	flag.StringVar(&kubeContext, "e2e.kubecontext", "",
		"Kubernetes context for e2e tests (required)")
}

func TestE2EEndpointRollout(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting OpenChoreo endpoint rollout e2e suite\n")
	RunSpecs(t, "OpenChoreo E2E Endpoint Rollout Suite")
}

var _ = BeforeSuite(func() {
	Expect(kubeContext).NotTo(BeEmpty(), "--e2e.kubecontext is required")
	fmt.Fprintf(GinkgoWriter, "Using kube context: %s\n", kubeContext)

	By("verifying cluster is accessible")
	out, err := framework.Kubectl(kubeContext, "cluster-info")
	Expect(err).NotTo(HaveOccurred(), "cluster not accessible with context %s:\n%s", kubeContext, out)

	By("granting admin to the client_credentials subject")
	out, err = framework.KubectlApplyLiteral(kubeContext, fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: %s
spec:
  roleMappings:
    - roleRef:
        name: admin
        kind: ClusterAuthzRole
  entitlement:
    claim: client_id
    value: %s
  effect: allow`, clusterAuthzBindingName, clientID))
	Expect(err).NotTo(HaveOccurred(), "failed to create ABAC binding: %s", out)

	By("waiting for the API to be reachable")
	framework.WaitForHTTP(apiURL+"/.well-known/oauth-protected-resource", 60*time.Second)

	By("obtaining an OAuth2 token from Thunder")
	token, err := framework.FetchClientCredentialsToken(tokenURL, clientID, clientSecret)
	Expect(err).NotTo(HaveOccurred(), "failed to obtain access token")
	Expect(token).NotTo(BeEmpty(), "access token is empty")

	By("building occ")
	runner, err := framework.NewOCCRunner(apiURL)
	Expect(err).NotTo(HaveOccurred(), "failed to create OCC runner")
	Expect(runner.Build()).To(Succeed(), "failed to build occ binary")
	occ = runner
	Expect(occ.SeedConfig(token)).To(Succeed(), "failed to seed occ config")
})

var _ = AfterSuite(func() {
	if occ != nil {
		occ.Cleanup()
	}
	framework.Kubectl(kubeContext, "delete", "clusterauthzrolebinding", clusterAuthzBindingName, "--ignore-not-found", "--wait=false") //nolint:errcheck
})
