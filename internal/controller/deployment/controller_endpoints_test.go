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

package deployment

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/dataplane"
	"github.com/choreo-idp/choreo/internal/labels"
)

var _ = Describe("makeEndpointLabels", func() {
	var (
		deployCtx        *dataplane.DeploymentContext
		endpointTemplate *choreov1.EndpointTemplate
		generatedLabels  map[string]string
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = &dataplane.DeploymentContext{}
		deployCtx.Deployment = &choreov1.Deployment{
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
		endpointTemplate = &choreov1.EndpointTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-endpoint",
			},
		}
	})

	JustBeforeEach(func() {
		generatedLabels = makeEndpointLabels(deployCtx, endpointTemplate)
	})

	It("should include all the original deployment labels", func() {
		Expect(generatedLabels).To(HaveKeyWithValue("core.choreo.dev/organization", "test-organization"))
		Expect(generatedLabels).To(HaveKeyWithValue("core.choreo.dev/project", "my-project"))
		Expect(generatedLabels).To(HaveKeyWithValue("core.choreo.dev/environment", "my-environment"))
		Expect(generatedLabels).To(HaveKeyWithValue("core.choreo.dev/component", "my-component"))
		Expect(generatedLabels).To(HaveKeyWithValue("core.choreo.dev/deployment-track", "my-main-track"))
	})

	It("should include the deployment name label", func() {
		Expect(generatedLabels).To(HaveKeyWithValue("core.choreo.dev/deployment", "my-deployment"))
	})

	It("should include the endpoint name label", func() {
		Expect(generatedLabels).To(HaveKeyWithValue("core.choreo.dev/name", "my-endpoint"))
	})
})
