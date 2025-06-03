// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"path"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"k8s.io/utils/ptr"
)

// HTTPRoutes renders the HTTPRoute resources for the given endpoint context.
func HTTPRoutes(rCtx *Context) []*choreov1.Resource {
	epType := rCtx.EndpointV2.Spec.Type
	switch epType {
	case choreov1.EndpointTypeREST:
		return makeRESTHTTPRoutes(rCtx)
	default:
		rCtx.AddError(UnsupportedEndpointTypeError(epType))
		return nil
	}
}

func makeRESTHTTPRoutes(rCtx *Context) []*choreov1.Resource {
	if rCtx.EndpointV2.Spec.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing"))
		return nil
	}
	if rCtx.EndpointClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the endpoint class"))
		return nil
	}

	// Generate HTTPRoute for each expose level and operation
	httpRoutes := make([]*gwapiv1.HTTPRoute, 0)

	// Process each operation and its expose levels
	for _, operation := range rCtx.EndpointV2.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == choreov1.ExposeLevelProject {
				continue
			}
			httpRoute := makeRESTHTTPRoute(rCtx, operation, exposeLevel)
			if httpRoute != nil {
				httpRoutes = append(httpRoutes, httpRoute)
			}
		}
	}

	resources := make([]*choreov1.Resource, len(httpRoutes))

	for _, httpRoute := range httpRoutes {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = httpRoute

		resources = append(resources, &choreov1.Resource{
			ID:     makeHTTPRouteResourceId(httpRoute),
			Object: rawExt,
		})
	}

	return resources
}

func makeHTTPRouteSpec(rCtx *Context, operation choreov1.RESTEndpointOperation, exposeLevel choreov1.RESTOperationExposeLevel) gwapiv1.HTTPRouteSpec {
	pathType := gwapiv1.PathMatchPathPrefix
	hostname := hostnameForExposeLevel(exposeLevel)
	port := gwapiv1.PortNumber(rCtx.EndpointV2.Spec.RESTEndpoint.Backend.Port)
	basePath := rCtx.EndpointV2.Spec.RESTEndpoint.Backend.BasePath
	endpointPath := path.Clean(path.Join("/", rCtx.EndpointV2.Spec.Owner.ProjectName,
		rCtx.EndpointV2.Spec.Owner.ComponentName, basePath))

	return gwapiv1.HTTPRouteSpec{
		CommonRouteSpec: gwapiv1.CommonRouteSpec{
			ParentRefs: []gwapiv1.ParentReference{
				{
					Name:      gwapiv1.ObjectName(gatewayNameForExposeLevel(exposeLevel)),
					Namespace: (*gwapiv1.Namespace)(ptr.To("choreo-system")), // Change NS based on where envoy gateway is deployed
				},
			},
		},
		Hostnames: []gwapiv1.Hostname{gwapiv1.Hostname(hostname)},
		Rules: []gwapiv1.HTTPRouteRule{
			{
				Matches: []gwapiv1.HTTPRouteMatch{
					{
						Path: &gwapiv1.HTTPPathMatch{
							Type:  &pathType,
							Value: ptr.To(endpointPath),
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
								Name: gwapiv1.ObjectName("choreo-service"), // TODO: This should point to the service exposed by the workload
								Port: &port,
							},
						},
					},
				},
			},
		},
	}
}

func gatewayNameForExposeLevel(exposeLevel choreov1.RESTOperationExposeLevel) string {
	switch exposeLevel {
	case choreov1.ExposeLevelOrganization:
		return "gateway-internal" // Organization-level internal gateway
	case choreov1.ExposeLevelPublic:
		return "gateway-external" // Public external gateway
	default:
		return "gateway-internal"
	}
}

// todo: take this from dp
func hostnameForExposeLevel(exposeLevel choreov1.RESTOperationExposeLevel) string {
	switch exposeLevel {
	case choreov1.ExposeLevelOrganization:
		return "choreoapis.internal" // Internal organization-level hostname
	case choreov1.ExposeLevelPublic:
		return "choreoapis.localhost" // Public external hostname
	default:
		return "choreoapis.internal" // Default internal hostname
	}
}

// makeRESTHTTPRoute creates an HTTPRoute for a specific operation and expose level
func makeRESTHTTPRoute(rCtx *Context, operation choreov1.RESTEndpointOperation, exposeLevel choreov1.RESTOperationExposeLevel) *gwapiv1.HTTPRoute {
	return &gwapiv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.networking.k8s.io/v1",
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(rCtx, operation, exposeLevel),
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeEndpointLabels(rCtx),
		},
		Spec: makeHTTPRouteSpec(rCtx, operation, exposeLevel),
	}
}

func makeHTTPRouteName(rCtx *Context, operation choreov1.RESTEndpointOperation, exposeLevel choreov1.RESTOperationExposeLevel) string {
	operationStr := fmt.Sprintf("%s-%s", strings.ToLower(string(operation.Method)), strings.TrimPrefix(operation.Path, "/"))
	return dpkubernetes.GenerateK8sName(rCtx.EndpointV2.Name, strings.ToLower(string(exposeLevel)), operationStr)
}

func makeNamespaceName(rCtx *Context) string {
	organizationName := rCtx.EndpointV2.Namespace // Namespace is the organization name
	projectName := rCtx.EndpointV2.Spec.Owner.ProjectName
	environmentName := rCtx.EndpointV2.Spec.EnvironmentName
	// Limit the name to 63 characters to comply with the K8s name length limit for Namespaces
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength,
		"dp", organizationName, projectName, environmentName)
}

func makeEndpointLabels(rCtx *Context) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: rCtx.EndpointV2.Namespace,
		dpkubernetes.LabelKeyProjectName:      rCtx.EndpointV2.Spec.Owner.ProjectName,
		dpkubernetes.LabelKeyEnvironmentName:  rCtx.EndpointV2.Spec.EnvironmentName,
		dpkubernetes.LabelKeyComponentName:    rCtx.EndpointV2.Spec.Owner.ComponentName,
		//dpkubernetes.LabelKeyManagedBy:        dpkubernetes.LabelValueManagedBy,
		//dpkubernetes.LabelKeyBelongTo:         dpkubernetes.LabelValueBelongTo,
	}
}

// TODO: Find a better way to generate resource IDs
func makeHTTPRouteResourceId(httpRoute *gwapiv1.HTTPRoute) string {
	return httpRoute.Name
}
