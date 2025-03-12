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
	"path"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/dataplane"
	"github.com/choreo-idp/choreo/internal/ptr"
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

func MakeHTTPRoute(epCtx *dataplane.EndpointContext, gwType GatewayType) *gwapiv1.HTTPRoute {
	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeHTTPRouteName(epCtx, gwType),
			Namespace: MakeNamespaceName(epCtx),
			Labels:    MakeWorkloadLabels(epCtx),
		},
		Spec: makeHTTPRouteSpec(epCtx, gwType),
	}
}

func makeHTTPRouteSpec(epCtx *dataplane.EndpointContext, gwType GatewayType) gwapiv1.HTTPRouteSpec {
	updatedEp := mergeAPISettings(epCtx, gwType)
	pathType := gwapiv1.PathMatchPathPrefix
	hostname := makeHostname(epCtx, gwType)
	port := gwapiv1.PortNumber(updatedEp.Spec.Service.Port)
	prefix := makePathPrefix(epCtx)
	basePath := epCtx.Endpoint.Spec.Service.BasePath
	endpointPath := basePath
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeService {
		// Prefix basepath with project and component names TODO: add org if necessary
		endpointPath = path.Clean(path.Join(prefix, basePath))
	}
	return gwapiv1.HTTPRouteSpec{
		CommonRouteSpec: gwapiv1.CommonRouteSpec{
			ParentRefs: []gwapiv1.ParentReference{
				{
					Name:      gwapiv1.ObjectName(gwType),
					Namespace: (*gwapiv1.Namespace)(ptr.String("choreo-system")), // Change NS based on where envoy gateway is deployed
				},
			},
		},
		Hostnames: []gwapiv1.Hostname{hostname},
		Rules: []gwapiv1.HTTPRouteRule{
			{
				Matches: []gwapiv1.HTTPRouteMatch{
					{
						Path: &gwapiv1.HTTPPathMatch{
							Type:  &pathType,
							Value: ptr.String(endpointPath),
						},
					},
				},
				Filters: []gwapiv1.HTTPRouteFilter{
					{
						Type: gwapiv1.HTTPRouteFilterURLRewrite,
						URLRewrite: &gwapiv1.HTTPURLRewriteFilter{
							Path: &gwapiv1.HTTPPathModifier{
								Type:               gwapiv1.PrefixMatchHTTPPathModifier,
								ReplacePrefixMatch: ptr.String(basePath),
							},
						},
					},
				},
				BackendRefs: []gwapiv1.HTTPBackendRef{
					{
						BackendRef: gwapiv1.BackendRef{
							BackendObjectReference: gwapiv1.BackendObjectReference{
								Name: gwapiv1.ObjectName(MakeServiceName(epCtx)),
								Port: &port,
							},
						},
					},
				},
			},
		},
	}
}

func mergeAPISettings(epCtx *dataplane.EndpointContext, gwType GatewayType) *choreov1.Endpoint {
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication {
		return epCtx.Endpoint
	}
	ep := epCtx.Endpoint.DeepCopy()
	if gwType == GatewayExternal {
		if ep.Spec.NetworkVisibilities.Public != nil &&
			ep.Spec.NetworkVisibilities.Public.APISettings != nil {
			ep.Spec.APISettings = ep.Spec.NetworkVisibilities.Public.APISettings
		}
	} else if gwType == GatewayInternal {
		if ep.Spec.NetworkVisibilities.Organization != nil &&
			ep.Spec.NetworkVisibilities.Organization.APISettings != nil {
			ep.Spec.APISettings = ep.Spec.NetworkVisibilities.Organization.APISettings
		}
	}
	return ep
}
