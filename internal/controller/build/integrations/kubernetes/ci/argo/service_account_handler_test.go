// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package argo

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
)

var _ = Describe("Service Account", func() {
	var (
		buildCtx       *integrations.BuildContext
		serviceAccount *corev1.ServiceAccount
	)

	BeforeEach(func() {
		buildCtx = newTestBuildContext()
	})

	JustBeforeEach(func() {
		serviceAccount = makeServiceAccount(buildCtx)
	})

	Context("Make service account name", func() {
		It("should return the correct name", func() {
			name := makeServiceAccountName()
			Expect(name).To(Equal("workflow-sa"))
		})
	})

	namespaceLabels := map[string]string{
		"managed-by": "choreo-build-controller",
	}

	Context("Make service account", func() {
		It("should create a service account with the correct metadata", func() {
			Expect(serviceAccount).NotTo(BeNil())
			Expect(serviceAccount.Name).To(Equal("workflow-sa"))
			Expect(serviceAccount.Namespace).To(Equal("choreo-ci-test-organization"))
			Expect(serviceAccount.Labels).To(BeComparableTo(namespaceLabels))
		})
	})
})
