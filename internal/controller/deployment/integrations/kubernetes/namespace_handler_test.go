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
