// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deployment

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("makeEndpointLabels", func() {
	var (
		deployCtx        *dataplane.DeploymentContext
		endpointTemplate *openchoreov1alpha1.EndpointTemplate
		generatedLabels  map[string]string
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = &dataplane.DeploymentContext{}
		deployCtx.Deployment = &openchoreov1alpha1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-deployment",
				Namespace: "test-organization",
				Labels: map[string]string{
					labels.LabelKeyOrganizationName:    "test-organization",
					labels.LabelKeyProjectName:         "my-project",
					labels.LabelKeyEnvironmentName:     "my-environment",
					labels.LabelKeyComponentName:       "my-component",
					labels.LabelKeyDeploymentTrackName: "my-main-track",
					labels.LabelKeyName:                "my-deployment",
				},
			},
		}
		endpointTemplate = &openchoreov1alpha1.EndpointTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-endpoint",
			},
		}
	})

	JustBeforeEach(func() {
		generatedLabels = makeEndpointLabels(deployCtx, endpointTemplate)
	})

	It("should include all the original deployment labels", func() {
		Expect(generatedLabels).To(HaveKeyWithValue("openchoreo.dev/organization", "test-organization"))
		Expect(generatedLabels).To(HaveKeyWithValue("openchoreo.dev/project", "my-project"))
		Expect(generatedLabels).To(HaveKeyWithValue("openchoreo.dev/environment", "my-environment"))
		Expect(generatedLabels).To(HaveKeyWithValue("openchoreo.dev/component", "my-component"))
		Expect(generatedLabels).To(HaveKeyWithValue("openchoreo.dev/deployment-track", "my-main-track"))
	})

	It("should include the deployment name label", func() {
		Expect(generatedLabels).To(HaveKeyWithValue("openchoreo.dev/deployment", "my-deployment"))
	})

	It("should include the endpoint name label", func() {
		Expect(generatedLabels).To(HaveKeyWithValue("openchoreo.dev/name", "my-endpoint"))
	})
})
