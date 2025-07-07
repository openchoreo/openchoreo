// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"path"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// HTTPRoutes renders the HTTPRoute resources for the given ServiceBinding context.
func HTTPRoutes(rCtx Context) []*choreov1.Resource {
	if rCtx.ServiceBinding.Spec.APIs == nil || len(rCtx.ServiceBinding.Spec.APIs) == 0 {
		return nil
	}

	httpRoutes := make([]*gwapiv1.HTTPRoute, 0)

	// Generate HttpRoutes per API and expose level
	for apiName, serviceAPI := range rCtx.ServiceBinding.Spec.APIs {
		if serviceAPI.Type != choreov1.EndpointTypeREST {
			continue // Skip non-REST APIs
		}

		if serviceAPI.RESTEndpoint == nil {
			rCtx.AddError(fmt.Errorf("REST endpoint specification is missing for API %s", apiName))
			continue
		}

		// Generate HttpRoute for each expose level
		if len(serviceAPI.RESTEndpoint.ExposeLevels) == 0 {
			// Default to Organization level if no expose levels specified
			httpRoute := makeHTTPRouteForServiceAPI(rCtx, apiName, serviceAPI, choreov1.ExposeLevelPublic)
			if httpRoute != nil {
				httpRoutes = append(httpRoutes, httpRoute)
			}
		} else {
			for _, exposeLevel := range serviceAPI.RESTEndpoint.ExposeLevels {
				if exposeLevel == choreov1.ExposeLevelProject {
					continue // Skip project level for now
				}
				httpRoute := makeHTTPRouteForServiceAPI(rCtx, apiName, serviceAPI, exposeLevel)
				if httpRoute != nil {
					httpRoutes = append(httpRoutes, httpRoute)
				}
			}
		}
	}

	resources := make([]*choreov1.Resource, 0, len(httpRoutes))

	for _, httpRoute := range httpRoutes {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = httpRoute

		resources = append(resources, &choreov1.Resource{
			ID:     makeHTTPRouteResourceID(httpRoute),
			Object: rawExt,
		})
	}

	return resources
}

func makeHTTPRouteForServiceAPI(rCtx Context, apiName string, serviceAPI *choreov1.ServiceAPI, exposeLevel choreov1.RESTOperationExposeLevel) *gwapiv1.HTTPRoute {
	if serviceAPI.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing for API %s", apiName))
		return nil
	}

	pathType := gwapiv1.PathMatchPathPrefix
	hostname := makeHostname(&rCtx, exposeLevel)
	name := makeHTTPRouteName(&rCtx, apiName, exposeLevel)
	port := gwapiv1.PortNumber(serviceAPI.RESTEndpoint.Backend.Port)
	basePath := serviceAPI.RESTEndpoint.Backend.BasePath

	// Use PrefixMatch pattern with basePath
	prefixPath := path.Clean(path.Join("/", rCtx.ServiceBinding.Spec.Owner.ProjectName,
		rCtx.ServiceBinding.Spec.Owner.ComponentName, basePath))

	return &gwapiv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.networking.k8s.io/v1",
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeServiceLabels(rCtx),
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Name:      gwapiv1.ObjectName(getGatewayName(exposeLevel)),
						Namespace: (*gwapiv1.Namespace)(ptr.To("choreo-system")),
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
								Value: &prefixPath,
							},
						},
					},
					Filters: []gwapiv1.HTTPRouteFilter{
						{
							Type: gwapiv1.HTTPRouteFilterURLRewrite,
							URLRewrite: &gwapiv1.HTTPURLRewriteFilter{
								Path: &gwapiv1.HTTPPathModifier{
									Type:               gwapiv1.PrefixMatchHTTPPathModifier,
									ReplacePrefixMatch: ptr.To(basePath),
								},
							},
						},
					},
					BackendRefs: []gwapiv1.HTTPBackendRef{
						{
							BackendRef: gwapiv1.BackendRef{
								BackendObjectReference: gwapiv1.BackendObjectReference{
									Name: gwapiv1.ObjectName(makeServiceName(rCtx)),
									Port: &port,
								},
							},
						},
					},
				},
			},
		},
	}
}

// makeHostname generates the hostname for a service based on environment and expose level
func makeHostname(rCtx *Context, exposeLevel choreov1.RESTOperationExposeLevel) gatewayv1.Hostname {
	envName := rCtx.ServiceBinding.Spec.Environment
	var domain string
	switch exposeLevel {
	case choreov1.ExposeLevelOrganization:
		domain = "choreoapis.internal"
	case choreov1.ExposeLevelPublic:
		domain = "choreoapis.localhost"
	default:
		domain = "choreoapis.internal"
	}
	return gatewayv1.Hostname(fmt.Sprintf("%s.%s", envName, domain))
}

func makeHTTPRouteName(rCtx *Context, apiName string, exposeLevel choreov1.RESTOperationExposeLevel) string {
	// Create a unique name for the HTTPRoute using ServiceBinding name, API name and expose level
	exposeLevelStr := strings.ToLower(string(exposeLevel))
	return dpkubernetes.GenerateK8sName(rCtx.ServiceBinding.Name, apiName, exposeLevelStr, "httproute")
}

func getGatewayName(exposeLevel choreov1.RESTOperationExposeLevel) string {
	switch exposeLevel {
	case choreov1.ExposeLevelPublic:
		return "gateway-external"
	case choreov1.ExposeLevelOrganization:
		return "gateway-internal"
	default:
		return "gateway-internal"
	}
}

func makeHTTPRouteResourceID(httpRoute *gwapiv1.HTTPRoute) string {
	return httpRoute.Name
}
