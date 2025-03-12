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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	corev1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/choreo-idp/choreo/internal/dataplane"
	"github.com/choreo-idp/choreo/internal/labels"
	"github.com/choreo-idp/choreo/internal/ptr"
)

func TestKubernetes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTPRoute Handler Suite")
}

var _ = Describe("HTTPRoute Handler", func() {
	Context("When generating HTTPRoute from Endpoint", func() {
		DescribeTable("should generate correct HTTPRoute specifications for different scenarios",
			func(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType, expectedPath string, expectedPort int32, expectedHostname string) {
				httpRoute := MakeHTTPRoute(epCtx, gwType)

				// Name
				Expect(httpRoute).NotTo(BeNil())
				Expect(httpRoute.ObjectMeta.Name).To(Equal(makeHTTPRouteName(epCtx, gwType)))

				// Verify spec details
				Expect(httpRoute.Spec.Rules).To(HaveLen(1))
				rule := httpRoute.Spec.Rules[0]

				// Verify path matching
				Expect(*rule.Matches[0].Path.Type).To(Equal(gatewayv1.PathMatchPathPrefix))
				Expect(*rule.Matches[0].Path.Value).To(Equal(expectedPath))

				// Verify backend reference
				backendRef := rule.BackendRefs[0]
				Expect(backendRef.BackendRef.BackendObjectReference.Name).To(Equal(gatewayv1.ObjectName(makeServiceName(epCtx))))
				Expect(backendRef.BackendRef.BackendObjectReference.Port).To(Equal((*gatewayv1.PortNumber)(ptr.Int32(expectedPort))))

				// Verify hostname
				Expect(string(httpRoute.Spec.Hostnames[0])).To(Equal(expectedHostname))
			},
			Entry("with standard path and port",
				createTestEndpointContext("/test", 8080, "test-component", "test-env"),
				visibility.GatewayExternal,
				"/test",
				int32(8080),
				"test-component-test-env.choreo.localhost",
			),
			Entry("with root path",
				createTestEndpointContext("/", 9090, "api-component", "prod"),
				visibility.GatewayExternal,
				"/",
				int32(9090),
				"api-component-prod.choreo.localhost",
			),
			Entry("with nested path",
				createTestEndpointContext("/api/v1", 8000, "service-component", "staging"),
				visibility.GatewayExternal,
				"/api/v1",
				int32(8000),
				"service-component-staging.choreo.localhost",
			),
		)
	})
})

// Helper function to create test endpoint context
func createTestEndpointContext(basePath string, port int32, componentName, envName string) *dataplane.EndpointContext {
	return &dataplane.EndpointContext{
		Endpoint: &corev1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-endpoint",
				Labels: map[string]string{
					labels.LabelKeyName: "test-endpoint",
				},
			},
			Spec: corev1.EndpointSpec{
				Type: "HTTP",
				Service: corev1.EndpointServiceSpec{
					BasePath: basePath,
					Port:     port,
				},
			},
		},
		Component: &corev1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentName,
				Labels: map[string]string{
					labels.LabelKeyName: componentName,
				},
			},
			Spec: corev1.ComponentSpec{
				Type: corev1.ComponentTypeWebApplication,
			},
		},
		Environment: &corev1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name: envName,
				Labels: map[string]string{
					labels.LabelKeyName: envName,
				},
				UID: "test-env-id",
			},
		},
		Project: &corev1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-project",
				Labels: map[string]string{
					labels.LabelKeyName: "test-project",
				},
			},
		},
		DeploymentTrack: &corev1.DeploymentTrack{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-track",
				Labels: map[string]string{
					labels.LabelKeyName: "test-track",
				},
			},
		},
		Deployment: &corev1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-deployment",
				Labels: map[string]string{
					labels.LabelKeyName: "test-deployment",
				},
			},
		},
		DataPlane: &corev1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-dataplane",
				Labels: map[string]string{
					labels.LabelKeyName: "test-dataplane",
				},
			},
			Spec: corev1.DataPlaneSpec{
				Gateway: corev1.GatewaySpec{
					PublicVirtualHost:       "choreo.localhost",
					OrganizationVirtualHost: "internal.choreo.localhost",
				},
			},
		},
	}
}
