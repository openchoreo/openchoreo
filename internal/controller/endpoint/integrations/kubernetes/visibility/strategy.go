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

	// Get endpoint with overridden API settings
	ep := OverrideAPISettings(epCtx, s.gatewayType)

	// Check if OAuth security scheme is configured
	return hasOAuthSecurityScheme(ep)
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

	// Get endpoint with overridden API settings
	ep := OverrideAPISettings(epCtx, s.gatewayType)

	// Check if OAuth security scheme is configured
	return hasOAuthSecurityScheme(ep)
}

// hasOAuthSecurityScheme checks if the endpoint has OAuth configured as a security scheme
func hasOAuthSecurityScheme(ep *choreov1.Endpoint) bool {
	if ep.Spec.APISettings == nil || ep.Spec.APISettings.SecuritySchemes == nil {
		return false
	}

	for _, scheme := range ep.Spec.APISettings.SecuritySchemes {
		if scheme == choreov1.Oauth {
			return true
		}
	}

	return false
}

// OverrideAPISettings applies visibility-specific API settings to the endpoint based on the gateway type.
// For web applications or endpoints without network visibilities, it returns the original endpoint unchanged.
// Otherwise, it applies the API settings from either the public or organization visibility configuration.
func OverrideAPISettings(epCtx *dataplane.EndpointContext, gwType GatewayType) *choreov1.Endpoint {
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication ||
		epCtx.Endpoint.Spec.NetworkVisibilities == nil {
		return epCtx.Endpoint
	}

	ep := epCtx.Endpoint.DeepCopy()
	visibilities := ep.Spec.NetworkVisibilities

	switch gwType {
	case GatewayExternal:
		if visibilities.Public != nil && visibilities.Public.APISettings != nil {
			ep.Spec.APISettings = visibilities.Public.APISettings
		}
	case GatewayInternal:
		if visibilities.Organization != nil && visibilities.Organization.APISettings != nil {
			ep.Spec.APISettings = visibilities.Organization.APISettings
		}
	}

	return ep
}
