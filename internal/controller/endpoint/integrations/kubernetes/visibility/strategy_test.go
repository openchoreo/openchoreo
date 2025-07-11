// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package visibility

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
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
				Component: &choreov1.Component{
					Spec: choreov1.ComponentSpec{
						Type: choreov1.ComponentTypeWebApplication,
					},
				},
				Endpoint: &choreov1.Endpoint{},
			}
			Expect(publicStrategy.IsHTTPRouteRequired(epCtx)).To(BeTrue())
		})

		It("should require HTTP route when public visibility is enabled with ComponentTypeService", func() {
			epCtx := &dataplane.EndpointContext{
				Component: &choreov1.Component{
					Spec: choreov1.ComponentSpec{
						Type: choreov1.ComponentTypeService,
					},
				},
				Endpoint: &choreov1.Endpoint{
					Spec: choreov1.EndpointSpec{
						NetworkVisibilities: &choreov1.NetworkVisibility{
							Public: &choreov1.VisibilityConfig{
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
				Component: &choreov1.Component{
					Spec: choreov1.ComponentSpec{
						Type: choreov1.ComponentTypeService,
					},
				},
				Endpoint: &choreov1.Endpoint{
					Spec: choreov1.EndpointSpec{
						NetworkVisibilities: &choreov1.NetworkVisibility{
							Public: &choreov1.VisibilityConfig{
								Enable: true,
								Policies: []choreov1.Policy{
									{
										Name: "oauth2-policy",
										Type: choreov1.Oauth2PolicyType,
										PolicySpec: &choreov1.PolicySpec{
											OAuth2: &choreov1.OAuth2PolicySpec{
												JWT: choreov1.JWT{
													Authorization: choreov1.AuthzSpec{
														APIType: choreov1.APITypeREST,
														Rest: &choreov1.REST{
															Operations: &[]choreov1.RESTOperation{
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
				Component: &choreov1.Component{
					Spec: choreov1.ComponentSpec{
						Type: choreov1.ComponentTypeWebApplication,
					},
				},
				Endpoint: &choreov1.Endpoint{},
			}
			Expect(organizationStrategy.IsHTTPRouteRequired(epCtx)).To(Not(BeTrue()))
		})

		It("should require HTTP route when organization visibility is enabled with ComponentTypeService", func() {
			epCtx := &dataplane.EndpointContext{
				Component: &choreov1.Component{
					Spec: choreov1.ComponentSpec{
						Type: choreov1.ComponentTypeService,
					},
				},
				Endpoint: &choreov1.Endpoint{
					Spec: choreov1.EndpointSpec{
						NetworkVisibilities: &choreov1.NetworkVisibility{
							Organization: &choreov1.VisibilityConfig{
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
