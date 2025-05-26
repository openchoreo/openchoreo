// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
)

var _ = Describe("Build Namespace", func() {
	var (
		buildCtx  *integrations.BuildContext
		namespace *corev1.Namespace
	)

	BeforeEach(func() {
		buildCtx = newTestBuildContext()
	})

	JustBeforeEach(func() {
		namespace = makeNamespace(buildCtx)
	})

	Context("Make name", func() {
		It("should create correct namespace name", func() {
			expectedName := MakeNamespaceName(buildCtx)
			Expect(expectedName).NotTo(BeNil())
			Expect(expectedName).To(Equal("choreo-ci-test-organization"))
		})
	})

	Context("Make namespace kind", func() {

		It("should create a Namespace with the correct name", func() {
			Expect(namespace).NotTo(BeNil())
			Expect(namespace.Name).To(Equal("choreo-ci-test-organization"))
		})

		namespaceLabels := map[string]string{
			"managed-by": "choreo-build-controller",
		}

		It("should create a Namespace with the correct labels", func() {
			Expect(namespace.Labels).To(BeComparableTo(namespaceLabels))
		})
	})
})
