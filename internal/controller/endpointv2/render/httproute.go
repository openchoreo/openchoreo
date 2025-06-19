// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"path"
	"regexp"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
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
			httpRoute := makeHTTPRouteForRestOperation(rCtx, operation, exposeLevel)
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

func makeHTTPRouteForRestOperation(rCtx *Context, restOperation choreov1.RESTEndpointOperation,
	exposeLevel choreov1.RESTOperationExposeLevel) *gwapiv1.HTTPRoute {
	pathType := gwapiv1.PathMatchRegularExpression
	method := restOperation.Method
	hostname := makeHostname(rCtx, exposeLevel)
	name := makeHTTPRouteName(rCtx, restOperation, exposeLevel)
	port := gwapiv1.PortNumber(rCtx.EndpointV2.Spec.RESTEndpoint.Backend.Port)
	// ToDo: Handle the case for webapps
	basePath := rCtx.EndpointV2.Spec.RESTEndpoint.Backend.BasePath
	endpointPath := path.Join(basePath, restOperation.Path)

	regexEpPath := GenerateRegexWithCaptureGroup(basePath, restOperation.Path, endpointPath)

	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeEndpointLabels(rCtx),
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Name:      gwapiv1.ObjectName(getGatewayName(rCtx)),
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
								Value: &regexEpPath,
							},
							Method: (*gwapiv1.HTTPMethod)(&method),
						},
					},
					Filters: []gwapiv1.HTTPRouteFilter{
						{
							Type: gwapiv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwapiv1.LocalObjectReference{
								Group: "gateway.envoyproxy.io",
								Kind:  "HTTPRouteFilter",
								Name:  gwapiv1.ObjectName(name),
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

// makeHostname generates the hostname for an endpoint based on gateway type and component type
func makeHostname(rCtx *Context, exposeLevel choreov1.RESTOperationExposeLevel) gatewayv1.Hostname {
	// ToDO: Handle the case for webapps
	//if rCtx.Component.Spec.Workload.Type == choreov1.WorkloadTypeWebApplication {
	//	return gatewayv1.Hostname(fmt.Sprintf("%s-%s.%s", rCtx.EndpointV2.Spec.Owner.ComponentName,
	//		rCtx.EndpointV2.Spec.EnvironmentName, "choreoapps.localhost"))
	//}
	var domain string
	switch exposeLevel {
	// ToDo: Find a correct way to get the domain & env prefix for both expose levels
	case choreov1.ExposeLevelOrganization:
		domain = "choreoapis.internal"
	default:
		domain = "choreoapis.localhost"
	}
	return gatewayv1.Hostname(fmt.Sprintf("%s.%s", "dev", domain))
}

// makePathPrefix returns the URL path prefix based on component type
func makePathPrefix(rCtx *Context) string {
	//if rCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication {
	//	return "/"
	//}
	return path.Clean(path.Join("/", rCtx.EndpointV2.Spec.Owner.ProjectName, rCtx.EndpointV2.Spec.Owner.ComponentName))
}

// GenerateRegexWithCaptureGroup generates a regex pattern that captures the basePath + operation part
// Parameters:
//   - basePath: the base path to match (e.g., "/api/v1/reading-list")
//   - operation: the operation path with parameters (e.g., "/books/{id}")
//   - pathMatch: the full path to match against (e.g., "/default-project/reading-list-service/api/v1/reading-list/books/{id}")
//
// Returns a regex with a capture group around the basePath + operation portion
func GenerateRegexWithCaptureGroup(basePath, operation, pathMatch string) string {
	// Define a regex pattern to match parameters in the operation
	paramPattern := regexp.MustCompile(`\{[^}]+\}`)

	// Combine basePath and operation to get the full path we want to capture
	capturablePath := basePath + operation

	// Remove leading double slashes of the capturable path
	capturablePath = regexp.MustCompile(`^//+`).ReplaceAllString(capturablePath, "/")

	// Remove trailing double slashes of the capturable path
	capturablePath = regexp.MustCompile(`//+$`).ReplaceAllString(capturablePath, "/")

	// Find where the capturable path starts in the full pathMatch
	captureStartIndex := strings.Index(pathMatch, capturablePath)
	if captureStartIndex == -1 {
		// If basePath is not found, return a simple regex
		return "^" + escapeExceptPatterns(paramPattern.ReplaceAllString(pathMatch, "[^/]+")) + "$"
	}

	// Split the pathMatch into prefix and the part we want to capture
	prefix := pathMatch[:captureStartIndex]
	capturablePart := pathMatch[captureStartIndex:]

	// Convert parameters in the capturable part to regex patterns
	capturableWithRegex := paramPattern.ReplaceAllString(capturablePart, "[^/]+")

	// Escape the prefix (the part before basePath)
	escapedPrefix := regexp.QuoteMeta(prefix)

	// Escape the capturable part (but we already handled parameters)
	// We need to escape everything except our [^/]+ patterns
	escapedCapturable := escapeExceptPatterns(capturableWithRegex)

	// Build the final regex with capture group
	return fmt.Sprintf("^%s(%s)$", escapedPrefix, escapedCapturable)
}

// escapeExceptPatterns escapes regex special characters but preserves [^/]+ patterns
func escapeExceptPatterns(input string) string {
	// First, replace [^/]+ with a placeholder
	placeholder := "REGEX_PATTERN_PLACEHOLDER"
	temp := strings.ReplaceAll(input, "[^/]+", placeholder)

	// Escape the rest
	escaped := regexp.QuoteMeta(temp)

	// Restore the regex patterns
	return strings.ReplaceAll(escaped, placeholder, "[^/]+")
}

func getGatewayName(rCtx *Context) string {
	// Default to internal gateway
	defaultGateway := "gateway-internal"

	// Check if we have a REST endpoint with operations
	if rCtx.EndpointV2.Spec.RESTEndpoint == nil || len(rCtx.EndpointV2.Spec.RESTEndpoint.Operations) == 0 {
		return defaultGateway
	}

	// Check for any public expose level operations - if found, use external gateway
	for _, operation := range rCtx.EndpointV2.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == choreov1.ExposeLevelPublic {
				return "gateway-external"
			}
		}
	}

	// If no public operations found, return internal gateway
	return defaultGateway
}

func makeServiceName(epCtx *Context) string {
	// ToDO: figure out how to get the service name from the workload
	return "choreo-service"
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
