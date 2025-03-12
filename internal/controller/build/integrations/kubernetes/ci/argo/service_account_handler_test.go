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

package argo

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
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
