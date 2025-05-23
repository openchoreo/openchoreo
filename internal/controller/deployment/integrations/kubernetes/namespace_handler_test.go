/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/openchoreo/openchoreo/internal/dataplane"
)

var _ = Describe("makeNamespace", func() {
	var (
		deployCtx *dataplane.DeploymentContext
		namespace *corev1.Namespace
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		namespace = makeNamespace(deployCtx)
	})

	Context("when the DeploymentContext has valid Project and Environment", func() {

		It("should create a Namespace with valid name", func() {
			Expect(namespace).NotTo(BeNil())
			Expect(namespace.Name).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))
		})

		expectedLabels := map[string]string{
			"organization-name": "test-organization",
			"project-name":      "my-project",
			"environment-name":  "test-environment",
			"managed-by":        "choreo-deployment-controller",
			"belong-to":         "user-workloads",
		}

		It("should create a Namespace with valid labels", func() {
			Expect(namespace.Labels).To(BeComparableTo(expectedLabels))
		})
	})
})
