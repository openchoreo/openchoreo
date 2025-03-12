package visibility

import (
	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/dataplane"
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

func (s *baseVisibilityStrategy) IsSecurityPolicyRequired(epCtx *dataplane.EndpointContext) bool {
	ep := OverrideAPISettings(epCtx, s.gatewayType)
	if ep.Spec.APISettings == nil || ep.Spec.APISettings.SecuritySchemes == nil {
		return false
	}
	secSchemes := epCtx.Endpoint.Spec.APISettings.SecuritySchemes
	for _, scheme := range secSchemes {
		return scheme == choreov1.Oauth
	}
	return false
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
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication {
		return true
	}
	return epCtx.Endpoint.Spec.NetworkVisibilities.Public != nil &&
		epCtx.Endpoint.Spec.NetworkVisibilities.Public.Enable
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
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication {
		return false
	}
	return epCtx.Endpoint.Spec.NetworkVisibilities.Organization != nil &&
		epCtx.Endpoint.Spec.NetworkVisibilities.Organization.Enable
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
