// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"regexp"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func TestHTTPRoutesHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTPRoute Handler Suite")
}

var _ = Describe("HTTPRoute Handler", func() {
	Context("When generating HTTPRoutes from Endpoint", func() {
		DescribeTable("should generate correct HTTPRoute specifications for different scenarios",
			func(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType, expectedHTTPRoutes []*gatewayv1.HTTPRoute) {
				httpRoutes := MakeHTTPRoutes(epCtx, gwType)

				// Should return at least one HTTPRoute
				Expect(httpRoutes).NotTo(BeEmpty())

				// verify whether the generated HTTPRoutes matches the expected ones
				// put the expected HTTPRoutes to a map
				expectedRoutesMap := make(map[string]*gatewayv1.HTTPRoute)
				for _, expectedRoute := range expectedHTTPRoutes {
					expectedRoutesMap[expectedRoute.ObjectMeta.Name] = expectedRoute
				}

				for _, route := range httpRoutes {
					expectedRoute, exists := expectedRoutesMap[route.ObjectMeta.Name]
					Expect(exists).To(BeTrue(), "HTTPRoute %s not found in expected routes", route.ObjectMeta.Name)

					// compare the name and namespace of the generated route with the expected one
					Expect(route.ObjectMeta.Name).To(Equal(expectedRoute.ObjectMeta.Name), "HTTPRoute name mismatch")
					Expect(route.ObjectMeta.Namespace).To(Equal(expectedRoute.ObjectMeta.Namespace), "HTTPRoute namespace mismatch")
					// Compare the spec of the generated route with the expected one
					Expect(route.Spec).To(Equal(expectedRoute.Spec), "HTTPRoute spec mismatch for %s", route.ObjectMeta.Name)
				}
			},

			Entry("for basic component with public visibility",
				createTestEndpointContext(
					&openchoreov1alpha1.Endpoint{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-endpoint",
							Labels: map[string]string{
								labels.LabelKeyName: "test-endpoint",
							},
						},
						Spec: openchoreov1alpha1.EndpointSpec{
							Type: "HTTP",
							BackendRef: openchoreov1alpha1.BackendRef{
								Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
								BasePath: "/",
								ComponentRef: &openchoreov1alpha1.ComponentRef{
									Port: 8080,
								},
							},
							NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
								Public: &openchoreov1alpha1.VisibilityConfig{
									Enable:   true,
									Policies: nil,
								},
							},
						},
					},
					"service-component-basic",
					"prod",
					openchoreov1alpha1.ComponentTypeService,
				),
				visibility.GatewayExternal,
				[]*gatewayv1.HTTPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gateway-external-test-endpoint-2870b170",
							Namespace: "dp-default-org-test-project-test-env-a545d497",
						},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{
									{
										Namespace: (*gatewayv1.Namespace)(ptr.To("choreo-system")),
										Name:      "gateway-external",
									},
								},
							},
							Hostnames: []gatewayv1.Hostname{
								"prod.choreoapis.localhost",
							},
							Rules: []gatewayv1.HTTPRouteRule{
								{
									Matches: []gatewayv1.HTTPRouteMatch{
										{
											Path: &gatewayv1.HTTPPathMatch{
												Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
												Value: ptr.To("/test-project/service-component-basic"),
											},
										},
									},
									Filters: []gatewayv1.HTTPRouteFilter{
										{
											Type: gatewayv1.HTTPRouteFilterURLRewrite,
											URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
												Path: &gatewayv1.HTTPPathModifier{
													Type:               gatewayv1.PrefixMatchHTTPPathModifier,
													ReplacePrefixMatch: ptr.To("/"),
												},
											},
										},
									},
									BackendRefs: []gatewayv1.HTTPBackendRef{
										{
											BackendRef: gatewayv1.BackendRef{
												BackendObjectReference: gatewayv1.BackendObjectReference{
													Name: "service-component-basic-test-track-1b4959e7",
													Port: (*gatewayv1.PortNumber)(ptr.To(int32(8080))),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			),

			Entry("for basic component with organization visibility",
				createTestEndpointContext(
					&openchoreov1alpha1.Endpoint{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-endpoint",
							Labels: map[string]string{
								labels.LabelKeyName: "test-endpoint",
							},
						},
						Spec: openchoreov1alpha1.EndpointSpec{
							Type: "HTTP",
							BackendRef: openchoreov1alpha1.BackendRef{
								Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
								BasePath: "/",
								ComponentRef: &openchoreov1alpha1.ComponentRef{
									Port: 8080,
								},
							},
							NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
								Organization: &openchoreov1alpha1.VisibilityConfig{
									Enable:   true,
									Policies: nil,
								},
							},
						},
					},
					"service-component-basic",
					"prod",
					openchoreov1alpha1.ComponentTypeService,
				),
				visibility.GatewayInternal,
				[]*gatewayv1.HTTPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gateway-internal-test-endpoint-5efa322c",
							Namespace: "dp-default-org-test-project-test-env-a545d497",
						},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{
									{
										Namespace: (*gatewayv1.Namespace)(ptr.To("choreo-system")),
										Name:      "gateway-internal",
									},
								},
							},
							Hostnames: []gatewayv1.Hostname{
								"prod.internal.choreoapis.localhost",
							},
							Rules: []gatewayv1.HTTPRouteRule{
								{
									Matches: []gatewayv1.HTTPRouteMatch{
										{
											Path: &gatewayv1.HTTPPathMatch{
												Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
												Value: ptr.To("/test-project/service-component-basic"),
											},
										},
									},
									Filters: []gatewayv1.HTTPRouteFilter{
										{
											Type: gatewayv1.HTTPRouteFilterURLRewrite,
											URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
												Path: &gatewayv1.HTTPPathModifier{
													Type:               gatewayv1.PrefixMatchHTTPPathModifier,
													ReplacePrefixMatch: ptr.To("/"),
												},
											},
										},
									},
									BackendRefs: []gatewayv1.HTTPBackendRef{
										{
											BackendRef: gatewayv1.BackendRef{
												BackendObjectReference: gatewayv1.BackendObjectReference{
													Name: "service-component-basic-test-track-1b4959e7",
													Port: (*gatewayv1.PortNumber)(ptr.To(int32(8080))),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			),

			Entry("for web application component with public visibility",
				createTestEndpointContext(
					&openchoreov1alpha1.Endpoint{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-endpoint",
							Labels: map[string]string{
								labels.LabelKeyName: "test-endpoint",
							},
						},
						Spec: openchoreov1alpha1.EndpointSpec{
							Type: "HTTP",
							BackendRef: openchoreov1alpha1.BackendRef{
								Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
								BasePath: "/",
								ComponentRef: &openchoreov1alpha1.ComponentRef{
									Port: 8080,
								},
							},
							NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
								Organization: &openchoreov1alpha1.VisibilityConfig{
									Enable:   true,
									Policies: nil,
								},
							},
						},
					},
					"webapp-component-basic",
					"prod",
					openchoreov1alpha1.ComponentTypeWebApplication,
				),
				visibility.GatewayExternal,
				[]*gatewayv1.HTTPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gateway-external-test-endpoint-2870b170",
							Namespace: "dp-default-org-test-project-test-env-a545d497",
						},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{
									{
										Namespace: (*gatewayv1.Namespace)(ptr.To("choreo-system")),
										Name:      "gateway-external",
									},
								},
							},
							Hostnames: []gatewayv1.Hostname{
								"webapp-component-basic-test-env.choreoapps.localhost",
							},
							Rules: []gatewayv1.HTTPRouteRule{
								{
									Matches: []gatewayv1.HTTPRouteMatch{
										{
											Path: &gatewayv1.HTTPPathMatch{
												Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
												Value: ptr.To("/"),
											},
										},
									},
									Filters: []gatewayv1.HTTPRouteFilter{
										{
											Type: gatewayv1.HTTPRouteFilterURLRewrite,
											URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
												Path: &gatewayv1.HTTPPathModifier{
													Type:               gatewayv1.PrefixMatchHTTPPathModifier,
													ReplacePrefixMatch: ptr.To("/"),
												},
											},
										},
									},
									BackendRefs: []gatewayv1.HTTPBackendRef{
										{
											BackendRef: gatewayv1.BackendRef{
												BackendObjectReference: gatewayv1.BackendObjectReference{
													Name: "webapp-component-basic-test-track-ba810c70",
													Port: (*gatewayv1.PortNumber)(ptr.To(int32(8080))),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			),

			Entry("for service component with public visibility and has oauth2 scopes",
				createTestEndpointContext(
					&openchoreov1alpha1.Endpoint{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-endpoint",
							Labels: map[string]string{
								labels.LabelKeyName: "test-endpoint",
							},
						},
						Spec: openchoreov1alpha1.EndpointSpec{
							Type: "REST",
							BackendRef: openchoreov1alpha1.BackendRef{
								Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
								BasePath: "/api/v1/reading-list",
								ComponentRef: &openchoreov1alpha1.ComponentRef{
									Port: 8080,
								},
							},
							NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
								Public: &openchoreov1alpha1.VisibilityConfig{
									Enable: true,
									Policies: []openchoreov1alpha1.Policy{
										{
											Name: "oauth2-scope-policy",
											Type: openchoreov1alpha1.Oauth2PolicyType,
											PolicySpec: &openchoreov1alpha1.PolicySpec{
												OAuth2: &openchoreov1alpha1.OAuth2PolicySpec{
													JWT: openchoreov1alpha1.JWT{
														Claims: &[]openchoreov1alpha1.JWTClaim{
															{
																Key: "aud",
																Values: []string{
																	"choreoapis.localhost",
																	"internal.choreoapis.localhost",
																},
															},
														},
														Authorization: openchoreov1alpha1.AuthzSpec{
															APIType: openchoreov1alpha1.APITypeREST,
															Rest: &openchoreov1alpha1.REST{
																Operations: &[]openchoreov1alpha1.RESTOperation{
																	{
																		Target: "/books",
																		Method: openchoreov1alpha1.HTTPMethodGet,
																		Scopes: []string{"read:books:all"},
																	},
																	{
																		Target: "/books",
																		Method: openchoreov1alpha1.HTTPMethodPost,
																		Scopes: []string{"write:books"},
																	},
																	{
																		Target: "/books/{id}",
																		Method: openchoreov1alpha1.HTTPMethodGet,
																		Scopes: []string{"read:books"},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					"reading-list-service",
					"dev",
					openchoreov1alpha1.ComponentTypeService,
				),
				visibility.GatewayExternal,
				[]*gatewayv1.HTTPRoute{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gateway-external-test-endpoint-2870b170",
							Namespace: "dp-default-org-test-project-test-env-a545d497",
						},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{
									{
										Namespace: (*gatewayv1.Namespace)(ptr.To("choreo-system")),
										Name:      "gateway-external",
									},
								},
							},
							Hostnames: []gatewayv1.Hostname{
								"dev.choreoapis.localhost",
							},
							Rules: []gatewayv1.HTTPRouteRule{
								{
									Matches: []gatewayv1.HTTPRouteMatch{
										{
											Path: &gatewayv1.HTTPPathMatch{
												Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
												Value: ptr.To("/test-project/reading-list-service/api/v1/reading-list"),
											},
										},
									},
									Filters: []gatewayv1.HTTPRouteFilter{
										{
											Type: gatewayv1.HTTPRouteFilterURLRewrite,
											URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
												Path: &gatewayv1.HTTPPathModifier{
													Type:               gatewayv1.PrefixMatchHTTPPathModifier,
													ReplacePrefixMatch: ptr.To("/api/v1/reading-list"),
												},
											},
										},
									},
									BackendRefs: []gatewayv1.HTTPBackendRef{
										{
											BackendRef: gatewayv1.BackendRef{
												BackendObjectReference: gatewayv1.BackendObjectReference{
													Name: "reading-list-service-test-track-2f72bb50",
													Port: (*gatewayv1.PortNumber)(ptr.To(int32(8080))),
												},
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gateway-external-test-endpoint-get-books-3dd70940",
							Namespace: "dp-default-org-test-project-test-env-a545d497",
						},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{
									{
										Namespace: (*gatewayv1.Namespace)(ptr.To("choreo-system")),
										Name:      "gateway-external",
									},
								},
							},
							Hostnames: []gatewayv1.Hostname{
								"dev.choreoapis.localhost",
							},
							Rules: []gatewayv1.HTTPRouteRule{
								{
									Matches: []gatewayv1.HTTPRouteMatch{
										{
											Path: &gatewayv1.HTTPPathMatch{
												Type:  ptr.To(gatewayv1.PathMatchRegularExpression),
												Value: ptr.To("^/test-project/reading-list-service(/api/v1/reading-list/books)$"),
											},
											Method: ptr.To(gatewayv1.HTTPMethodGet),
										},
									},
									Filters: []gatewayv1.HTTPRouteFilter{
										{
											Type: gatewayv1.HTTPRouteFilterExtensionRef,
											ExtensionRef: &gatewayv1.LocalObjectReference{
												Group: "gateway.envoyproxy.io",
												Kind:  "HTTPRouteFilter",
												Name:  "gateway-external-test-endpoint-get-books-3dd70940",
											},
										},
									},
									BackendRefs: []gatewayv1.HTTPBackendRef{
										{
											BackendRef: gatewayv1.BackendRef{
												BackendObjectReference: gatewayv1.BackendObjectReference{
													Name: "reading-list-service-test-track-2f72bb50",
													Port: (*gatewayv1.PortNumber)(ptr.To(int32(8080))),
												},
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gateway-external-test-endpoint-post-books-5a07271d",
							Namespace: "dp-default-org-test-project-test-env-a545d497",
						},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{
									{
										Namespace: (*gatewayv1.Namespace)(ptr.To("choreo-system")),
										Name:      "gateway-external",
									},
								},
							},
							Hostnames: []gatewayv1.Hostname{
								"dev.choreoapis.localhost",
							},
							Rules: []gatewayv1.HTTPRouteRule{
								{
									Matches: []gatewayv1.HTTPRouteMatch{
										{
											Path: &gatewayv1.HTTPPathMatch{
												Type:  ptr.To(gatewayv1.PathMatchRegularExpression),
												Value: ptr.To("^/test-project/reading-list-service(/api/v1/reading-list/books)$"),
											},
											Method: ptr.To(gatewayv1.HTTPMethodPost),
										},
									},
									Filters: []gatewayv1.HTTPRouteFilter{
										{
											Type: gatewayv1.HTTPRouteFilterExtensionRef,
											ExtensionRef: &gatewayv1.LocalObjectReference{
												Group: "gateway.envoyproxy.io",
												Kind:  "HTTPRouteFilter",
												Name:  "gateway-external-test-endpoint-post-books-5a07271d",
											},
										},
									},
									BackendRefs: []gatewayv1.HTTPBackendRef{
										{
											BackendRef: gatewayv1.BackendRef{
												BackendObjectReference: gatewayv1.BackendObjectReference{
													Name: "reading-list-service-test-track-2f72bb50",
													Port: (*gatewayv1.PortNumber)(ptr.To(int32(8080))),
												},
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gateway-external-test-endpoint-get-books--id-946b0081",
							Namespace: "dp-default-org-test-project-test-env-a545d497",
						},
						Spec: gatewayv1.HTTPRouteSpec{
							CommonRouteSpec: gatewayv1.CommonRouteSpec{
								ParentRefs: []gatewayv1.ParentReference{
									{
										Namespace: (*gatewayv1.Namespace)(ptr.To("choreo-system")),
										Name:      "gateway-external",
									},
								},
							},
							Hostnames: []gatewayv1.Hostname{
								"dev.choreoapis.localhost",
							},
							Rules: []gatewayv1.HTTPRouteRule{
								{
									Matches: []gatewayv1.HTTPRouteMatch{
										{
											Path: &gatewayv1.HTTPPathMatch{
												Type:  ptr.To(gatewayv1.PathMatchRegularExpression),
												Value: ptr.To("^/test-project/reading-list-service(/api/v1/reading-list/books/[^/]+)$"),
											},
											Method: ptr.To(gatewayv1.HTTPMethodGet),
										},
									},
									Filters: []gatewayv1.HTTPRouteFilter{
										{
											Type: gatewayv1.HTTPRouteFilterExtensionRef,
											ExtensionRef: &gatewayv1.LocalObjectReference{
												Group: "gateway.envoyproxy.io",
												Kind:  "HTTPRouteFilter",
												Name:  "gateway-external-test-endpoint-get-books--id-946b0081",
											},
										},
									},
									BackendRefs: []gatewayv1.HTTPBackendRef{
										{
											BackendRef: gatewayv1.BackendRef{
												BackendObjectReference: gatewayv1.BackendObjectReference{
													Name: "reading-list-service-test-track-2f72bb50",
													Port: (*gatewayv1.PortNumber)(ptr.To(int32(8080))),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			),
		)
	})
})

// Helper function to create test endpoint context
func createTestEndpointContext(endpoint *openchoreov1alpha1.Endpoint, componentName, envDNSPrefix string,
	componentType openchoreov1alpha1.ComponentType) *dataplane.EndpointContext {
	return &dataplane.EndpointContext{
		Endpoint: endpoint,
		Component: &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentName,
				Labels: map[string]string{
					labels.LabelKeyName: componentName,
				},
			},
			Spec: openchoreov1alpha1.ComponentSpec{
				Type: componentType,
			},
		},
		Environment: &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-env",
				Labels: map[string]string{
					labels.LabelKeyName: "test-env",
				},
				UID: "test-env-id",
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				Gateway: openchoreov1alpha1.GatewayConfig{
					DNSPrefix: envDNSPrefix,
				},
			},
		},
		Project: &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-project",
				Labels: map[string]string{
					labels.LabelKeyOrganizationName: "default-org",
					labels.LabelKeyName:             "test-project",
				},
			},
		},
		DeploymentTrack: &openchoreov1alpha1.DeploymentTrack{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-track",
				Labels: map[string]string{
					labels.LabelKeyName: "test-track",
				},
			},
		},
		Deployment: &openchoreov1alpha1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-deployment",
				Labels: map[string]string{
					labels.LabelKeyName: "test-deployment",
				},
			},
		},
		DataPlane: &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-dataplane",
				Labels: map[string]string{
					labels.LabelKeyName: "test-dataplane",
				},
			},
			Spec: openchoreov1alpha1.DataPlaneSpec{
				Gateway: openchoreov1alpha1.GatewaySpec{
					PublicVirtualHost:       "choreoapis.localhost",
					OrganizationVirtualHost: "internal.choreoapis.localhost",
				},
			},
		},
	}
}

var _ = Describe("Test GenerateRegexWithCaptureGroup", func() {
	Context("when generating regex with capture groups", func() {
		It("should handle the basic example with single parameter", func() {
			basePath := "/api/v1/reading-list"
			operation := "/books/{id}"
			pathMatch := "/default-project/reading-list-service/api/v1/reading-list/books/{id}"
			expectedRegex := "^/default-project/reading-list-service(/api/v1/reading-list/books/[^/]+)$"

			result := GenerateRegexWithCaptureGroup(basePath, operation, pathMatch)
			Expect(result).To(Equal(expectedRegex))

			re := regexp.MustCompile(result)
			Expect(re.MatchString("/default-project/reading-list-service/api/v1/reading-list/books/123")).To(BeTrue())
			Expect(re.MatchString("/default-project/reading-list-service/api/v1/reading-list/books/abc-def")).To(BeTrue())
			Expect(re.MatchString("/different-project/reading-list-service/api/v1/reading-list/books/123")).To(BeFalse())
			Expect(re.MatchString("/default-project/reading-list-service/api/v1/reading-list/books/")).To(BeFalse())
			Expect(re.MatchString("/default-project/reading-list-service/api/v1/reading-list/books/123/extra")).To(BeFalse())

			matches := re.FindStringSubmatch("/default-project/reading-list-service/api/v1/reading-list/books/123")
			Expect(matches).To(HaveLen(2))
			Expect(matches[1]).To(Equal("/api/v1/reading-list/books/123"))
		})

		It("should handle multiple parameters", func() {
			basePath := "/api/v2/users"
			operation := "/{userId}/posts/{postId}"
			pathMatch := "/service/api/v2/users/{userId}/posts/{postId}"
			expectedRegex := `^/service(/api/v2/users/[^/]+/posts/[^/]+)$`

			result := GenerateRegexWithCaptureGroup(basePath, operation, pathMatch)
			Expect(result).To(Equal(expectedRegex))

			re := regexp.MustCompile(result)
			Expect(re.MatchString("/service/api/v2/users/123/posts/456")).To(BeTrue())
			Expect(re.MatchString("/service/api/v2/users/user_abc/posts/post_def")).To(BeTrue())
			Expect(re.MatchString("/service/api/v2/users/123/posts/")).To(BeFalse())
			Expect(re.MatchString("/service/api/v2/users//posts/456")).To(BeFalse())

			matches := re.FindStringSubmatch("/service/api/v2/users/123/posts/456")
			Expect(matches).To(HaveLen(2))
			Expect(matches[1]).To(Equal("/api/v2/users/123/posts/456"))
		})

		It("should handle paths with no parameters", func() {
			basePath := "/health"
			operation := ""
			pathMatch := "/app/health"
			expectedRegex := `^/app(/health)$`

			result := GenerateRegexWithCaptureGroup(basePath, operation, pathMatch)
			Expect(result).To(Equal(expectedRegex))

			re := regexp.MustCompile(result)
			Expect(re.MatchString("/app/health")).To(BeTrue())
			Expect(re.MatchString("/app/health/check")).To(BeFalse())
			Expect(re.MatchString("/different/health")).To(BeFalse())

			matches := re.FindStringSubmatch("/app/health")
			Expect(matches).To(HaveLen(2))
			Expect(matches[1]).To(Equal("/health"))
		})

		It("should handle operation without parameters", func() {
			basePath := "/api/v1"
			operation := "/status"
			pathMatch := "/service/api/v1/status"
			expectedRegex := `^/service(/api/v1/status)$`

			result := GenerateRegexWithCaptureGroup(basePath, operation, pathMatch)
			Expect(result).To(Equal(expectedRegex))

			re := regexp.MustCompile(result)
			Expect(re.MatchString("/service/api/v1/status")).To(BeTrue())
			Expect(re.MatchString("/service/api/v1/status/details")).To(BeFalse())
			Expect(re.MatchString("/other/api/v1/status")).To(BeFalse())

			matches := re.FindStringSubmatch("/service/api/v1/status")
			Expect(matches).To(HaveLen(2))
			Expect(matches[1]).To(Equal("/api/v1/status"))
		})

		It("should handle basePath with only slash", func() {
			basePath := "/"
			operation := "/{id1}"
			pathMatch := "/service/api/{id1}"
			expectedRegex := `^/service/api(/[^/]+)$`

			result := GenerateRegexWithCaptureGroup(basePath, operation, pathMatch)
			Expect(result).To(Equal(expectedRegex))

			re := regexp.MustCompile(result)
			Expect(re.MatchString("/service/api/123")).To(BeTrue())
			Expect(re.MatchString("/service/api/item-name")).To(BeTrue())
			Expect(re.MatchString("/service/api/")).To(BeFalse())
			Expect(re.MatchString("/service/api/123/extra")).To(BeFalse())

			matches := re.FindStringSubmatch("/service/api/123")
			Expect(matches).To(HaveLen(2))
			Expect(matches[1]).To(Equal("/123"))
		})

		It("should handle special characters in paths", func() {
			basePath := "/api/v1.0/items"
			operation := "/{id}"
			pathMatch := "/service-2.0/api/v1.0/items/{id}"
			expectedRegex := "^/service-2\\.0(/api/v1\\.0/items/[^/]+)$"

			result := GenerateRegexWithCaptureGroup(basePath, operation, pathMatch)
			Expect(result).To(Equal(expectedRegex))

			re := regexp.MustCompile(result)
			Expect(re.MatchString("/service-2.0/api/v1.0/items/123")).To(BeTrue())
			Expect(re.MatchString("/service-2.0/api/v1.0/items/item-name")).To(BeTrue())
			Expect(re.MatchString("/service-2X0/api/v1X0/items/123")).To(BeFalse()) // dots should be literal
			Expect(re.MatchString("/service-2.0/api/v1.0/items/")).To(BeFalse())

			matches := re.FindStringSubmatch("/service-2.0/api/v1.0/items/123")
			Expect(matches).To(HaveLen(2))
			Expect(matches[1]).To(Equal("/api/v1.0/items/123"))
		})

		It("should handle complex nested parameter names", func() {
			basePath := "/api/v1/projects"
			operation := "/{projectId}/services/{serviceId}"
			pathMatch := "/default-project/api/v1/projects/{projectId}/services/{serviceId}"
			expectedRegex := "^/default-project(/api/v1/projects/[^/]+/services/[^/]+)$"

			result := GenerateRegexWithCaptureGroup(basePath, operation, pathMatch)
			Expect(result).To(Equal(expectedRegex))

			re := regexp.MustCompile(result)
			Expect(re.MatchString("/default-project/api/v1/projects/proj123/services/svc456")).To(BeTrue())
			Expect(re.MatchString("/default-project/api/v1/projects/my-project/services/my-service")).To(BeTrue())
			Expect(re.MatchString("/default-project/api/v1/projects/proj123/services/")).To(BeFalse())
			Expect(re.MatchString("/default-project/api/v1/projects//services/svc456")).To(BeFalse())

			matches := re.FindStringSubmatch("/default-project/api/v1/projects/proj123/services/svc456")
			Expect(matches).To(HaveLen(2))
			Expect(matches[1]).To(Equal("/api/v1/projects/proj123/services/svc456"))
		})

		Context("when basePath is not found in pathMatch", func() {
			It("should return a simple escaped regex without capture group", func() {
				basePath := "/not/found"
				operation := "/{id}"
				pathMatch := "/different/path/{id}"
				expectedRegex := "^/different/path/[^/]+$"

				result := GenerateRegexWithCaptureGroup(basePath, operation, pathMatch)
				Expect(result).To(Equal(expectedRegex))

				re := regexp.MustCompile(result)
				Expect(re.MatchString("/different/path/123")).To(BeTrue())
				Expect(re.MatchString("/not/found/123")).To(BeFalse())

				// Should not have capture groups
				matches := re.FindStringSubmatch("/different/path/{id}")
				Expect(matches).To(HaveLen(1)) // Only the full match, no capture groups
			})
		})

		Context("when validating generated regex", func() {
			It("should always generate valid regex patterns", func() {
				testCases := []struct {
					basePath  string
					operation string
					pathMatch string
				}{
					{"/api/v1", "/{id}", "/service/api/v1/{id}"},
					{"/health", "", "/app/health"},
					{"/api/v1.0", "/{id}", "/service-2.0/api/v1.0/{id}"},
					{"/not/found", "/{id}", "/different/path/{id}"},
				}

				for _, tc := range testCases {
					result := GenerateRegexWithCaptureGroup(tc.basePath, tc.operation, tc.pathMatch)
					_, err := regexp.Compile(result)
					Expect(err).ToNot(HaveOccurred(), "Generated regex should be valid: %s", result)
				}
			})
		})
	})
})
