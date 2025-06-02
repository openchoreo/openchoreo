// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	corev1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/ptr"
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
				"/test-project/test-component/test",
				int32(8080),
				"test-env.choreoapis.localhost",
			),
			Entry("with root path",
				createTestEndpointContext("/", 9090, "api-component", "prod"),
				visibility.GatewayExternal,
				"/test-project/api-component",
				int32(9090),
				"prod.choreoapis.localhost",
			),
			Entry("with nested path",
				createTestEndpointContext("/api/v1", 8000, "service-component", "staging"),
				visibility.GatewayExternal,
				"/test-project/service-component/api/v1",
				int32(8000),
				"staging.choreoapis.localhost",
			),
		)
	})
})

// Helper function to create test endpoint context
func createTestEndpointContext(basePath string, port int32, componentName, dnsPrefix string) *dataplane.EndpointContext {
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
				Type: corev1.ComponentTypeService,
			},
		},
		Environment: &corev1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-env",
				Labels: map[string]string{
					labels.LabelKeyName: "test-env",
				},
				UID: "test-env-id",
			},
			Spec: corev1.EnvironmentSpec{
				Gateway: corev1.GatewayConfig{
					DNSPrefix: dnsPrefix,
				},
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
					PublicVirtualHost:       "choreoapis.localhost",
					OrganizationVirtualHost: "internal.choreoapis.localhost",
				},
			},
		},
	}
}
