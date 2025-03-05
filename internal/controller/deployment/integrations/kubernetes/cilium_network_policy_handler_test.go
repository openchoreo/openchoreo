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

	"github.com/choreo-idp/choreo/internal/dataplane"
	ciliumv2 "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/cilium.io/v2"
)

var _ = Describe("makeCiliumNetworkPolicy", func() {
	var (
		deployCtx *dataplane.DeploymentContext
		cnp       *ciliumv2.CiliumNetworkPolicy
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		cnp = makeCiliumNetworkPolicy(deployCtx)
	})

	Context("when the DeploymentContext has valid Project and Environment", func() {

		It("should create a CiliumNetworkPolicy with correct name and namespace", func() {
			Expect(cnp).NotTo(BeNil())
			Expect(cnp.Name).To(Equal("default-policy"))
			Expect(cnp.Namespace).To(Equal("dp-test-organiza-my-project-development-314a8e4f"))
		})

		expectedLabels := map[string]string{
			"organization-name": "test-organization",
			"project-name":      "my-project",
			"environment-name":  "development",
			"managed-by":        "choreo-deployment-controller",
			"belong-to":         "user-workloads",
		}

		It("should create a CiliumNetworkPolicy with valid labels", func() {
			Expect(cnp.Labels).To(BeComparableTo(expectedLabels))
		})

		It("should create a CiliumNetworkPolicy that allows communication between all pods within the namespace", func() {
			allEndpointsSelector := ciliumv2.EndpointSelector{}
			allowAllEgressRule := ciliumv2.EgressRule{ToEndpoints: []ciliumv2.EndpointSelector{allEndpointsSelector}}
			allowAllIngressRule := ciliumv2.IngressRule{FromEndpoints: []ciliumv2.EndpointSelector{allEndpointsSelector}}

			Expect(cnp.Spec.EndpointSelector).To(Equal(&allEndpointsSelector))
			Expect(cnp.Spec.Egress).To(HaveLen(1))
			Expect(cnp.Spec.Egress[0]).To(Equal(allowAllEgressRule))
			Expect(cnp.Spec.Ingress).To(HaveLen(1))
			Expect(cnp.Spec.Ingress[0]).To(Equal(allowAllIngressRule))
		})
	})
})
