package visibility

import (
	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

// GatewayType represents the type of gateway used to expose endpoints
type GatewayType string

const (
	// GatewayExternal is the gateway used to expose endpoints that are publicly accessible from outside the cluster
	GatewayExternal GatewayType = "gateway-external"

	// GatewayInternal is the gateway used to expose endpoints that are only accessible within the organization
	GatewayInternal GatewayType = "gateway-internal"
)

// Visibility represents the accessibility level of an endpoint
type Visibility string

const (
	// VisibilityPublic indicates that an endpoint should be accessible from outside the cluster
	// through the external gateway
	VisibilityPublic Visibility = "Public"

	// VisibilityPrivate indicates that an endpoint should only be accessible within the
	// organization through the internal gateway
	VisibilityPrivate Visibility = "Organization"
)

type VisibilityStrategy interface {
	IsHTTPRouteRequired(epCtx *dataplane.EndpointContext) bool
	IsSecurityPolicyRequired(epCtx *dataplane.EndpointContext) bool
	GetGatewayType() GatewayType
}

// baseVisibilityStrategy contains common functionality for visibility strategies
type baseVisibilityStrategy struct {
	gatewayType GatewayType
}

func (s *baseVisibilityStrategy) GetGatewayType() GatewayType {
	return s.gatewayType
}

type PublicVisibilityStrategy struct {
	baseVisibilityStrategy
}

func NewPublicVisibilityStrategy() *PublicVisibilityStrategy {
	return &PublicVisibilityStrategy{
		baseVisibilityStrategy{gatewayType: GatewayExternal},
	}
}

func (s *PublicVisibilityStrategy) IsHTTPRouteRequired(epCtx *dataplane.EndpointContext) bool {
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication || epCtx.Endpoint.Spec.NetworkVisibilities == nil {
		return true
	}
	return epCtx.Endpoint.Spec.NetworkVisibilities.Public != nil &&
		epCtx.Endpoint.Spec.NetworkVisibilities.Public.Enable
}

func (s *PublicVisibilityStrategy) IsSecurityPolicyRequired(epCtx *dataplane.EndpointContext) bool {
	// Check if public visibility is enabled
	if epCtx.Endpoint.Spec.NetworkVisibilities == nil ||
		epCtx.Endpoint.Spec.NetworkVisibilities.Public == nil ||
		!epCtx.Endpoint.Spec.NetworkVisibilities.Public.Enable {
		return false
	}

	// Check if OAuth security scheme is configured
	return hasOAuthSecurityScheme(epCtx, s.gatewayType)
}

type OrganizationVisibilityStrategy struct {
	baseVisibilityStrategy
}

func NewOrganizationVisibilityStrategy() *OrganizationVisibilityStrategy {
	return &OrganizationVisibilityStrategy{
		baseVisibilityStrategy{gatewayType: GatewayInternal},
	}
}

func (s *OrganizationVisibilityStrategy) IsHTTPRouteRequired(epCtx *dataplane.EndpointContext) bool {
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication || epCtx.Endpoint.Spec.NetworkVisibilities == nil {
		return false // Disable organization visibility for webapp
	}
	return epCtx.Endpoint.Spec.NetworkVisibilities.Organization != nil &&
		epCtx.Endpoint.Spec.NetworkVisibilities.Organization.Enable
}

func (s *OrganizationVisibilityStrategy) IsSecurityPolicyRequired(epCtx *dataplane.EndpointContext) bool {
	// Check if organization visibility is enabled
	if epCtx.Endpoint.Spec.NetworkVisibilities == nil ||
		epCtx.Endpoint.Spec.NetworkVisibilities.Organization == nil ||
		!epCtx.Endpoint.Spec.NetworkVisibilities.Organization.Enable {
		return false
	}

	// Check if OAuth security scheme is configured
	return hasOAuthSecurityScheme(epCtx, s.gatewayType)
}

// hasOAuthSecurityScheme checks if the endpoint has OAuth configured as a security scheme
func hasOAuthSecurityScheme(epCtx *dataplane.EndpointContext, gwType GatewayType) bool {
	ep := epCtx.Endpoint
	if ep.Spec.NetworkVisibilities == nil {
		return false
	}

	switch gwType {
	case GatewayExternal:
		if ep.Spec.NetworkVisibilities.Public == nil ||
			!ep.Spec.NetworkVisibilities.Public.Enable ||
			ep.Spec.NetworkVisibilities.Public.Policies == nil ||
			len(ep.Spec.NetworkVisibilities.Public.Policies) == 0 {
			return false
		}
		for _, policy := range ep.Spec.NetworkVisibilities.Public.Policies {
			if policy.PolicySpec != nil && policy.Type == "oauth2" {
				if policy.PolicySpec.OAuth2 != nil &&
					policy.PolicySpec.OAuth2.JWT.Authorization.Rest != nil &&
					policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations != nil &&
					len(*policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations) > 0 {
					return true
				}
			}
		}
		return false
	case GatewayInternal:
		if ep.Spec.NetworkVisibilities.Organization == nil ||
			!ep.Spec.NetworkVisibilities.Organization.Enable ||
			ep.Spec.NetworkVisibilities.Organization.Policies == nil ||
			len(ep.Spec.NetworkVisibilities.Organization.Policies) == 0 {
			return false
		}
		for _, policy := range ep.Spec.NetworkVisibilities.Public.Policies {
			if policy.PolicySpec != nil && policy.Type == "oauth2" {
				if policy.PolicySpec.OAuth2 != nil &&
					policy.PolicySpec.OAuth2.JWT.Authorization.Rest != nil &&
					policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations != nil &&
					len(*policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations) > 0 {
					return true
				}
			}
		}
		return false
	}

	return false
}
