// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openchoreo/openchoreo/internal/dataplane"
	ciliumv2 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/cilium.io/v2"
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
			Expect(cnp.Namespace).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))
		})

		expectedLabels := map[string]string{
			"organization-name": "test-organization",
			"project-name":      "my-project",
			"environment-name":  "test-environment",
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
