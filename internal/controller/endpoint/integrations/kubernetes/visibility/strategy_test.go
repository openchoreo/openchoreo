package visibility

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/dataplane"
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

		Context("IsHTTPRouteRequired", func() {
			DescribeTable("HTTP Route Requirements",
				func(strategy VisibilityStrategy, component *choreov1.Component, visibilityConfig *choreov1.NetworkVisibility, expected bool) {
					epCtx := &dataplane.EndpointContext{
						Component: component,
						Endpoint: &choreov1.Endpoint{
							Spec: choreov1.EndpointSpec{
								NetworkVisibilities: visibilityConfig,
							},
						},
					}
					Expect(strategy.IsHTTPRouteRequired(epCtx)).To(Equal(expected))
				},
				Entry("public strategy - web application",
					publicStrategy,
					&choreov1.Component{Spec: choreov1.ComponentSpec{Type: choreov1.ComponentTypeWebApplication}},
					nil,
					true),
				Entry("public strategy - enabled visibility",
					publicStrategy,
					&choreov1.Component{Spec: choreov1.ComponentSpec{Type: choreov1.ComponentTypeService}},
					&choreov1.NetworkVisibility{Public: &choreov1.VisibilityConfig{Enable: true}},
					true),
				Entry("public strategy - disabled visibility",
					publicStrategy,
					&choreov1.Component{Spec: choreov1.ComponentSpec{Type: choreov1.ComponentTypeService}},
					&choreov1.NetworkVisibility{Public: &choreov1.VisibilityConfig{Enable: false}},
					false),
				Entry("organization strategy - web application",
					organizationStrategy,
					&choreov1.Component{Spec: choreov1.ComponentSpec{Type: choreov1.ComponentTypeWebApplication}},
					nil,
					false),
				Entry("organization strategy - enabled visibility",
					organizationStrategy,
					&choreov1.Component{},
					&choreov1.NetworkVisibility{Organization: &choreov1.VisibilityConfig{Enable: true}},
					true),
				Entry("organization strategy - disabled visibility",
					organizationStrategy,
					&choreov1.Component{},
					&choreov1.NetworkVisibility{Organization: &choreov1.VisibilityConfig{Enable: false}},
					false),
			)
		})

		Context("IsSecurityPolicyRequired", func() {
			It("should return true when OAuth security scheme is configured", func() {
				epCtx := &dataplane.EndpointContext{
					Component: &choreov1.Component{
						Spec: choreov1.ComponentSpec{
							Type: choreov1.ComponentTypeService,
						},
					},
					Endpoint: &choreov1.Endpoint{
						Spec: choreov1.EndpointSpec{
							APISettings: &choreov1.EndpointAPISettingsSpec{
								SecuritySchemes: []choreov1.SecurityScheme{choreov1.Oauth},
							},
						},
					},
				}
				Expect(publicStrategy.IsSecurityPolicyRequired(epCtx)).To(BeTrue())
			})

			It("should return false when no security scheme is configured", func() {
				epCtx := &dataplane.EndpointContext{
					Endpoint: &choreov1.Endpoint{
						Spec: choreov1.EndpointSpec{},
					},
					Component: &choreov1.Component{
						Spec: choreov1.ComponentSpec{
							Type: choreov1.ComponentTypeService,
						},
					},
				}
				Expect(publicStrategy.IsSecurityPolicyRequired(epCtx)).To(BeFalse())
			})
		})
	})

	Context("Organization Visibility Strategy", func() {
		It("should return correct gateway type", func() {
			Expect(organizationStrategy.GetGatewayType()).To(Equal(GatewayInternal))
		})

		Context("IsHTTPRouteRequired", func() {
			It("should return false for web applications", func() {
				epCtx := &dataplane.EndpointContext{
					Component: &choreov1.Component{
						Spec: choreov1.ComponentSpec{
							Type: choreov1.ComponentTypeWebApplication,
						},
					},
					Endpoint: &choreov1.Endpoint{
						Spec: choreov1.EndpointSpec{},
					},
				}
				Expect(organizationStrategy.IsHTTPRouteRequired(epCtx)).To(BeFalse())
			})

			It("should return true when organization visibility is enabled", func() {
				epCtx := &dataplane.EndpointContext{
					Component: &choreov1.Component{},
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

			It("should return false when organization visibility is disabled", func() {
				epCtx := &dataplane.EndpointContext{
					Component: &choreov1.Component{},
					Endpoint: &choreov1.Endpoint{
						Spec: choreov1.EndpointSpec{
							NetworkVisibilities: &choreov1.NetworkVisibility{
								Organization: &choreov1.VisibilityConfig{
									Enable: false,
								},
							},
						},
					},
				}
				Expect(organizationStrategy.IsHTTPRouteRequired(epCtx)).To(BeFalse())
			})
		})
	})

	DescribeTable("API Settings Override",
		func(gatewayType GatewayType, visibilityConfig *choreov1.NetworkVisibility) {
			epCtx := &dataplane.EndpointContext{
				Component: &choreov1.Component{},
				Endpoint: &choreov1.Endpoint{
					Spec: choreov1.EndpointSpec{
						APISettings:         &choreov1.EndpointAPISettingsSpec{},
						NetworkVisibilities: visibilityConfig,
					},
				},
			}
			result := OverrideAPISettings(epCtx, gatewayType)
			Expect(result.Spec.APISettings.SecuritySchemes).To(ContainElement(choreov1.Oauth))
		},
		Entry("public visibility",
			GatewayExternal,
			&choreov1.NetworkVisibility{
				Public: &choreov1.VisibilityConfig{
					APISettings: &choreov1.EndpointAPISettingsSpec{
						SecuritySchemes: []choreov1.SecurityScheme{choreov1.Oauth},
					},
				},
			}),
		Entry("organization visibility",
			GatewayInternal,
			&choreov1.NetworkVisibility{
				Organization: &choreov1.VisibilityConfig{
					APISettings: &choreov1.EndpointAPISettingsSpec{
						SecuritySchemes: []choreov1.SecurityScheme{choreov1.Oauth},
					},
				},
			}),
	)
})
