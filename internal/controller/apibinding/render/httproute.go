// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// HTTPRoutes renders the HTTPRoute resources for the given API context.
func HTTPRoutes(rCtx *Context) []*choreov1.Resource {
	apiType := rCtx.API.Spec.Type
	switch apiType {
	case choreov1.EndpointTypeREST:
		return makeRESTHTTPRoutes(rCtx)
	default:
		rCtx.AddError(fmt.Errorf("unsupported API type: %s", apiType))
		return nil
	}
}

func makeRESTHTTPRoutes(rCtx *Context) []*choreov1.Resource {
	if rCtx.API.Spec.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing"))
		return nil
	}
	if rCtx.APIClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the API class"))
		return nil
	}

	// Generate HTTPRoute for each expose level and operation
	httpRoutes := make([]*gwapiv1.HTTPRoute, 0)

	// Process each operation and its expose levels
	for _, operation := range rCtx.API.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == choreov1.ExposeLevelProject {
				continue // Skip project level for now
			}
			httpRoute := makeHTTPRouteForRestOperation(rCtx, operation, exposeLevel)
			if httpRoute != nil {
				httpRoutes = append(httpRoutes, httpRoute)
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

func makeHTTPRouteForRestOperation(rCtx *Context, restOperation choreov1.RESTEndpointOperation,
	exposeLevel choreov1.RESTOperationExposeLevel) *gwapiv1.HTTPRoute {
	pathType := gwapiv1.PathMatchRegularExpression
	method := restOperation.Method
	hostname := makeHostname(rCtx, exposeLevel)
	name := makeHTTPRouteName(rCtx, restOperation, exposeLevel)
	port := gwapiv1.PortNumber(rCtx.API.Spec.RESTEndpoint.Backend.Port)
	basePath := rCtx.API.Spec.RESTEndpoint.Backend.BasePath
	endpointPath := path.Join(basePath, restOperation.Path)

	regexEpPath := GenerateRegexWithCaptureGroup(basePath, restOperation.Path, endpointPath)

	return &gwapiv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.networking.k8s.io/v1",
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeAPILabels(rCtx),
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

// makeAPILabels creates common labels for API-related resources.
func makeAPILabels(rCtx *Context) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: rCtx.APIBinding.Namespace,
		// TODO: Add more labels when APIBinding spec is defined:
		// dpkubernetes.LabelKeyProjectName:      rCtx.APIBinding.Spec.ProjectName,
		// dpkubernetes.LabelKeyEnvironmentName:  rCtx.APIBinding.Spec.EnvironmentName,
		// dpkubernetes.LabelKeyComponentName:    rCtx.APIBinding.Spec.ComponentName,
	}
}

// makeHostname generates the hostname for an API based on gateway type and expose level
func makeHostname(_ *Context, exposeLevel choreov1.RESTOperationExposeLevel) gatewayv1.Hostname {
	var domain string
	switch exposeLevel {
	case choreov1.ExposeLevelOrganization:
		domain = "choreoapis.internal"
	default:
		domain = "choreoapis.localhost"
	}
	return gatewayv1.Hostname(fmt.Sprintf("%s.%s", "dev", domain))
}

func makeHTTPRouteName(rCtx *Context, operation choreov1.RESTEndpointOperation, exposeLevel choreov1.RESTOperationExposeLevel) string {
	operationStr := fmt.Sprintf("%s-%s", strings.ToLower(string(operation.Method)), strings.TrimPrefix(operation.Path, "/"))
	return dpkubernetes.GenerateK8sName(rCtx.APIBinding.Name, strings.ToLower(string(exposeLevel)), operationStr)
}

func makeNamespaceName(rCtx *Context) string {
	organizationName := rCtx.APIBinding.Namespace // Namespace is the organization name
	projectName := rCtx.API.Spec.Owner.ProjectName
	environmentName := rCtx.APIBinding.Spec.EnvironmentName
	// Limit the name to 63 characters to comply with the K8s name length limit for Namespaces
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength,
		"dp", organizationName, projectName, environmentName)
}

func makeServiceName(rCtx *Context) string {
	// TODO: figure out how to get the service name from the workload
	return "choreo-service"
}

func getGatewayName(rCtx *Context) string {
	// Default to internal gateway
	defaultGateway := "gateway-internal"

	// Check if we have a REST endpoint with operations
	if rCtx.API.Spec.RESTEndpoint == nil || len(rCtx.API.Spec.RESTEndpoint.Operations) == 0 {
		return defaultGateway
	}

	// Check for any public expose level operations - if found, use external gateway
	for _, operation := range rCtx.API.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == choreov1.ExposeLevelPublic {
				return "gateway-external"
			}
		}
	}

	// If no public operations found, return internal gateway
	return defaultGateway
}

func makeHTTPRouteResourceID(httpRoute *gwapiv1.HTTPRoute) string {
	return httpRoute.Name
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
