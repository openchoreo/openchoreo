// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package visibility

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

func TestVisibility(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Visibility Suite")
}

var _ = Describe("Visibility Strategy", func() {
	var (
		publicStrategy       VisibilityStrategy
		organizationStrategy VisibilityStrategy
	)

	BeforeEach(func() {
		publicStrategy = NewPublicVisibilityStrategy()
		organizationStrategy = NewOrganizationVisibilityStrategy()
	})

	Context("Public Visibility Strategy", func() {
		It("should return correct gateway type", func() {
			Expect(publicStrategy.GetGatewayType()).To(Equal(GatewayExternal))
		})

		It("should require HTTP route for web applications with ComponentTypeWebApplication", func() {
			epCtx := &dataplane.EndpointContext{
				Component: &openchoreov1alpha1.Component{
					Spec: openchoreov1alpha1.ComponentSpec{
						Type: openchoreov1alpha1.ComponentTypeWebApplication,
					},
				},
				Endpoint: &openchoreov1alpha1.Endpoint{},
			}
			Expect(publicStrategy.IsHTTPRouteRequired(epCtx)).To(BeTrue())
		})

		It("should require HTTP route when public visibility is enabled with ComponentTypeService", func() {
			epCtx := &dataplane.EndpointContext{
				Component: &openchoreov1alpha1.Component{
					Spec: openchoreov1alpha1.ComponentSpec{
						Type: openchoreov1alpha1.ComponentTypeService,
					},
				},
				Endpoint: &openchoreov1alpha1.Endpoint{
					Spec: openchoreov1alpha1.EndpointSpec{
						NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
							Public: &openchoreov1alpha1.VisibilityConfig{
								Enable: true,
							},
						},
					},
				},
			}
			Expect(publicStrategy.IsHTTPRouteRequired(epCtx)).To(BeTrue())
		})

		It("should require security policy when OAuth is configured with ComponentTypeService", func() {
			epCtx := &dataplane.EndpointContext{
				Component: &openchoreov1alpha1.Component{
					Spec: openchoreov1alpha1.ComponentSpec{
						Type: openchoreov1alpha1.ComponentTypeService,
					},
				},
				Endpoint: &openchoreov1alpha1.Endpoint{
					Spec: openchoreov1alpha1.EndpointSpec{
						NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
							Public: &openchoreov1alpha1.VisibilityConfig{
								Enable: true,
								Policies: []openchoreov1alpha1.Policy{
									{
										Name: "oauth2-policy",
										Type: openchoreov1alpha1.Oauth2PolicyType,
										PolicySpec: &openchoreov1alpha1.PolicySpec{
											OAuth2: &openchoreov1alpha1.OAuth2PolicySpec{
												JWT: openchoreov1alpha1.JWT{
													Authorization: openchoreov1alpha1.AuthzSpec{
														APIType: openchoreov1alpha1.APITypeREST,
														Rest: &openchoreov1alpha1.REST{
															Operations: &[]openchoreov1alpha1.RESTOperation{
																{
																	Method: "GET",
																	Target: "/api/v1/users",
																	Scopes: []string{"read:users"},
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
			}
			Expect(publicStrategy.IsSecurityPolicyRequired(epCtx)).To(BeTrue())
		})
	})

	Context("Organization Visibility Strategy", func() {
		It("should return correct gateway type", func() {
			Expect(organizationStrategy.GetGatewayType()).To(Equal(GatewayInternal))
		})

		It("should not require HTTP route for web applications with ComponentTypeWebApplication", func() {
			epCtx := &dataplane.EndpointContext{
				Component: &openchoreov1alpha1.Component{
					Spec: openchoreov1alpha1.ComponentSpec{
						Type: openchoreov1alpha1.ComponentTypeWebApplication,
					},
				},
				Endpoint: &openchoreov1alpha1.Endpoint{},
			}
			Expect(organizationStrategy.IsHTTPRouteRequired(epCtx)).To(Not(BeTrue()))
		})

		It("should require HTTP route when organization visibility is enabled with ComponentTypeService", func() {
			epCtx := &dataplane.EndpointContext{
				Component: &openchoreov1alpha1.Component{
					Spec: openchoreov1alpha1.ComponentSpec{
						Type: openchoreov1alpha1.ComponentTypeService,
					},
				},
				Endpoint: &openchoreov1alpha1.Endpoint{
					Spec: openchoreov1alpha1.EndpointSpec{
						NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
							Organization: &openchoreov1alpha1.VisibilityConfig{
								Enable: true,
							},
						},
					},
				},
			}
			Expect(organizationStrategy.IsHTTPRouteRequired(epCtx)).To(BeTrue())
		})
	})
})
