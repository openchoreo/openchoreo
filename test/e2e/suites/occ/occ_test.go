// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

// occApply writes YAML to a temp file and runs occ apply -f <file>.
func occApply(yamlContent string) (string, string, error) {
	path, err := occ.WriteFixtureFile(yamlContent)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "failed to write fixture file")
	return occ.Run("apply", "-f", path)
}

// expectApplySucceeded asserts the apply output indicates a resource was created or configured.
func expectApplySucceeded(stdout string) {
	ExpectWithOffset(1, stdout).To(Or(
		ContainSubstring("created"),
		ContainSubstring("configured"),
	))
}

var _ = Describe("OCC CLI", Ordered, Label("tier2"), func() {
	describeSmoke()
	describeApply()
	describeResourceCommands()
	describeConfigCommands()
	describeSecretCommands()
	describeLoginCommands()
	describeNegative()
	describeDeleteCommands()

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}

		By("cleaning up data-plane namespaces created by the controller")
		dpPrefix := fmt.Sprintf("dp-%s", cpNs[:20])
		out, _ := framework.Kubectl(kubeContext, "get", "ns", "-o", "name")
		for _, line := range strings.Split(out, "\n") {
			ns := strings.TrimPrefix(line, "namespace/")
			if strings.HasPrefix(ns, dpPrefix) {
				framework.Kubectl(kubeContext, "delete", "namespace", ns, "--ignore-not-found", "--wait=false") //nolint:errcheck
			}
		}

		By(fmt.Sprintf("cleaning up namespace %s", cpNs))
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")
	})
})
