/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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
