/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	"fmt"
	"path"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

// makeHostname generates the hostname for an endpoint based on gateway type and component type
func makeHostname(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) gatewayv1.Hostname {
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication {
		return gatewayv1.Hostname(fmt.Sprintf("%s-%s.%s", epCtx.Component.Name, epCtx.Environment.Name, "choreoapps.localhost"))
	}
	var domain string
	switch gwType {
	case visibility.GatewayInternal:
		domain = epCtx.DataPlane.Spec.Gateway.OrganizationVirtualHost
	default:
		domain = epCtx.DataPlane.Spec.Gateway.PublicVirtualHost
	}
	return gatewayv1.Hostname(fmt.Sprintf("%s.%s", epCtx.Environment.Spec.Gateway.DNSPrefix, domain))
}

// makePathPrefix returns the URL path prefix based on component type
func makePathPrefix(epCtx *dataplane.EndpointContext) string {
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication {
		return "/"
	}
	return path.Clean(path.Join("/", epCtx.Project.Name, epCtx.Component.Name))
}

// MakeAddress constructs the full HTTPS URL for an endpoint
func MakeAddress(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) string {
	host := makeHostname(epCtx, gwType)
	pathPrefix := makePathPrefix(epCtx)

	return fmt.Sprintf("https://%s%s", host, pathPrefix)
}
