// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// HTTPRoutes renders the HTTPRoute resources for exposing the webapplication endpoints.
func HTTPRoutes(rCtx Context) []*choreov1.Resource {
	if rCtx.WebApplicationBinding.Spec.WorkloadSpec.Endpoints == nil ||
		len(rCtx.WebApplicationBinding.Spec.WorkloadSpec.Endpoints) == 0 {
		return nil
	}

	httpRoutes := make([]*gwapiv1.HTTPRoute, 0)

	// Generate HttpRoute for each workload endpoint (typically one for web app)
	for endpointName, endpoint := range rCtx.WebApplicationBinding.Spec.WorkloadSpec.Endpoints {
		httpRoute := makeHTTPRouteForWebApp(rCtx, endpointName, &endpoint)
		if httpRoute != nil {
			httpRoutes = append(httpRoutes, httpRoute)
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

func makeHTTPRouteForWebApp(rCtx Context, endpointName string, endpoint *choreov1.WorkloadEndpoint) *gwapiv1.HTTPRoute {
	pathType := gwapiv1.PathMatchPathPrefix
	hostname := makeHostname(&rCtx)
	name := makeHTTPRouteName(&rCtx, endpointName)
	port := gwapiv1.PortNumber(endpoint.Port)

	// Web applications use the root path prefix for the component
	prefixPath := "/"

	return &gwapiv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.networking.k8s.io/v1",
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeWebApplicationLabels(rCtx),
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Name:      gwapiv1.ObjectName(getGatewayName()),
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
									ReplacePrefixMatch: ptr.To("/"),
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

// makeHostname generates the hostname for a web application using the pattern: component-environment.choreoapps.localhost
func makeHostname(rCtx *Context) gatewayv1.Hostname {
	return gatewayv1.Hostname(fmt.Sprintf("%s-%s.%s", rCtx.WebApplicationBinding.Spec.Owner.ComponentName,
		rCtx.WebApplicationBinding.Spec.Environment, "choreoapps.localhost"))
}

func makeHTTPRouteName(rCtx *Context, endpointName string) string {
	// Create a unique name for the HTTPRoute using WebApplicationBinding name and endpoint name
	return dpkubernetes.GenerateK8sName(rCtx.WebApplicationBinding.Name, endpointName, "httproute")
}

func getGatewayName() string {
	// Assumption: Web applications typically use external gateway for public access
	// ToDo: Confirm this
	return "gateway-external"
}

func makeHTTPRouteResourceID(httpRoute *gwapiv1.HTTPRoute) string {
	return httpRoute.Name
}
