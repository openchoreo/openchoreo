// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

type httpRoutesHandler struct {
	client     client.Client
	visibility visibility.VisibilityStrategy
}

var _ dataplane.ResourceHandler[dataplane.EndpointContext] = (*httpRoutesHandler)(nil)

func NewHTTPRouteHandler(kubernetesClient client.Client, visibility visibility.VisibilityStrategy) dataplane.ResourceHandler[dataplane.EndpointContext] {
	return &httpRoutesHandler{
		client:     kubernetesClient,
		visibility: visibility,
	}
}

func (h *httpRoutesHandler) Name() string {
	return "KubernetesHTTPRoutesHandler"
}

func (h *httpRoutesHandler) IsRequired(epCtx *dataplane.EndpointContext) bool {
	return h.visibility.IsHTTPRouteRequired(epCtx)
}

func (h *httpRoutesHandler) GetCurrentState(ctx context.Context, epCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := makeNamespaceName(epCtx)
	labels := makeWorkloadLabels(epCtx, h.visibility.GetGatewayType())

	listOption := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	out := &gwapiv1.HTTPRouteList{}
	err := h.client.List(ctx, out, listOption...)
	if err != nil {
		return nil, fmt.Errorf("error while listing HTTPRoutes: %w", err)
	}
	return out, nil
}

func (h *httpRoutesHandler) Create(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	httpRoutes := MakeHTTPRoutes(epCtx, h.visibility.GetGatewayType())
	for _, httpRoute := range httpRoutes {
		if err := h.client.Create(ctx, httpRoute); err != nil {
			return fmt.Errorf("error while creating HTTPRoute %s: %w", httpRoute.Name, err)
		}
	}
	return nil
}

func (h *httpRoutesHandler) Update(ctx context.Context, epCtx *dataplane.EndpointContext, currentState interface{}) error {
	currentHTTPRoutesList, ok := currentState.(*gwapiv1.HTTPRouteList)
	currentHTTPRoutes := currentHTTPRoutesList.Items
	if !ok {
		return errors.New("failed to cast current state to the list of HTTPRoutes")
	}

	desiredHTTPRoutes := MakeHTTPRoutes(epCtx, h.visibility.GetGatewayType())

	// Create a map of current HTTP routes by name for easier lookup
	currentHTTPRoutesMap := make(map[string]*gwapiv1.HTTPRoute)
	for _, route := range currentHTTPRoutes {
		currentHTTPRoutesMap[route.Name] = &route
	}

	// Create a map of desired HTTP routes by name for easier lookup
	desiredHTTPRoutesMap := make(map[string]*gwapiv1.HTTPRoute)
	for _, route := range desiredHTTPRoutes {
		desiredHTTPRoutesMap[route.Name] = route
	}

	// Update each HTTPRoute individually if needed
	for name, newHTTPRoute := range desiredHTTPRoutesMap {
		currentHTTPRoute, exists := currentHTTPRoutesMap[name]
		if !exists {
			// If the route doesn't exist in the current state, create it
			if err := h.client.Create(ctx, newHTTPRoute); err != nil {
				return fmt.Errorf("error while creating HTTPRoute %s: %w", newHTTPRoute.Name, err)
			}
			continue
		}

		// Check if the route needs to be updated
		if !cmp.Equal(currentHTTPRoute.Spec, newHTTPRoute.Spec) ||
			!cmp.Equal(extractManagedLabels(currentHTTPRoute.Labels), extractManagedLabels(newHTTPRoute.Labels)) {
			updatedHTTPRoute := currentHTTPRoute.DeepCopy()
			updatedHTTPRoute.Spec = newHTTPRoute.Spec
			updatedHTTPRoute.Labels = newHTTPRoute.Labels

			if err := h.client.Update(ctx, updatedHTTPRoute); err != nil {
				return fmt.Errorf("error while updating HTTPRoute %s: %w", newHTTPRoute.Name, err)
			}
		}
	}

	// Delete HTTPRoutes that exist in the current state but not in the desired state
	for name, currentHTTPRoute := range currentHTTPRoutesMap {
		if _, exists := desiredHTTPRoutesMap[name]; !exists {
			if err := h.client.Delete(ctx, currentHTTPRoute); err != nil {
				if !apierrors.IsNotFound(err) {
					return fmt.Errorf("error while deleting HTTPRoute %s: %w", currentHTTPRoute.Name, err)
				}
			}
		}
	}

	return nil
}

func (h *httpRoutesHandler) Delete(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	namespace := makeNamespaceName(epCtx)
	labels := makeWorkloadLabels(epCtx, h.visibility.GetGatewayType())
	deleteAllOption := []client.DeleteAllOfOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	err := h.client.DeleteAllOf(ctx, &gwapiv1.HTTPRoute{}, deleteAllOption...)
	if err != nil {
		return fmt.Errorf("error while deleting HTTPRoutes: %w", err)
	}

	return nil
}

func MakeHTTPRoutes(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) []*gwapiv1.HTTPRoute {
	out := make([]*gwapiv1.HTTPRoute, 0)

	policies := extractPoliciesFromCtx(epCtx, gwType)

	// Create wildcard HTTPRoute as by default we have to expose everything
	httpRoute := makeWildcardHTTPRoute(epCtx, gwType)
	out = append(out, httpRoute)

	// Check if we should only create the wildcard HTTPRoute
	if shouldOnlyCreateWildCardHTTPRoute(epCtx, gwType, policies) {
		return out
	}

	for _, policy := range policies {
		// Skip policies without specs or if not OAuth2 type
		if policy.PolicySpec == nil || policy.Type != openchoreov1alpha1.Oauth2PolicyType {
			continue
		}

		// Skip if OAuth2 config is missing or JWT operations are not configured
		if policy.PolicySpec.OAuth2 == nil ||
			policy.PolicySpec.OAuth2.JWT.Authorization.Rest == nil ||
			policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations == nil {
			continue
		}

		// Generate separate HTTP routes for each operation
		operations := policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations
		for _, operation := range *operations {
			httpRoute := makeHTTPRouteForOperation(epCtx, &operation, gwType)
			out = append(out, httpRoute)
		}
	}

	return out
}

// shouldOnlyCreateWildCardHTTPRoute checks if we should only create the wildcard HTTPRoute
func shouldOnlyCreateWildCardHTTPRoute(epCtx *dataplane.EndpointContext,
	gwType visibility.GatewayType, policies []openchoreov1alpha1.Policy) bool {
	if epCtx.Endpoint.Spec.NetworkVisibilities == nil {
		return true
	}

	if gwType == visibility.GatewayExternal {
		if epCtx.Endpoint.Spec.NetworkVisibilities.Public == nil ||
			!epCtx.Endpoint.Spec.NetworkVisibilities.Public.Enable {
			return true
		}
	}

	if gwType == visibility.GatewayInternal {
		if epCtx.Endpoint.Spec.NetworkVisibilities.Organization == nil ||
			!epCtx.Endpoint.Spec.NetworkVisibilities.Organization.Enable {
			return true
		}
	}

	if len(policies) == 0 {
		return true
	}

	// Check if any of the policies have OAuth2 configured
	for _, policy := range policies {
		if policy.PolicySpec != nil &&
			policy.Type == openchoreov1alpha1.Oauth2PolicyType &&
			policy.PolicySpec.OAuth2 != nil &&
			policy.PolicySpec.OAuth2.JWT.Authorization.Rest != nil &&
			policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations != nil &&
			len(*policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations) > 0 {
			// OAuth2 is configured, no need for wildcard route
			return false
		}
	}

	return true
}

// makeWildcardHTTPRoute creates a wildcard HTTPRoute for the endpoint
// This route will match all requests with the path prefix of the endpoint's base path
// For example, if the endpoint's base path is "/api/v1/reading-list",
// it will match all requests with "/<environment>/<component>/api/v1/reading-list/*"
//
// if need to apply any policies for specific path, this can be overridden with specific HTTPRoutes
func makeWildcardHTTPRoute(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) *gwapiv1.HTTPRoute {
	hostname := makeHostname(epCtx, gwType)
	pathType := gwapiv1.PathMatchPathPrefix
	port := gwapiv1.PortNumber(epCtx.Endpoint.Spec.BackendRef.ComponentRef.Port)
	prefix := makePathPrefix(epCtx)
	basePath := epCtx.Endpoint.Spec.BackendRef.BasePath
	endpointPath := basePath
	if epCtx.Component.Spec.Type == openchoreov1alpha1.ComponentTypeService {
		endpointPath = path.Clean(path.Join(prefix, basePath))
	}
	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(epCtx, gwType),
			Namespace: makeNamespaceName(epCtx),
			Labels:    makeWorkloadLabels(epCtx, gwType),
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Name:      gwapiv1.ObjectName(gwType),
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
									Name: gwapiv1.ObjectName(makeServiceName(epCtx)),
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

// makeHTTPRouteForOperation creates an HTTPRoute for a specific REST operation
func makeHTTPRouteForOperation(epCtx *dataplane.EndpointContext, restOperation *openchoreov1alpha1.RESTOperation,
	gwType visibility.GatewayType) *gwapiv1.HTTPRoute {
	pathType := gwapiv1.PathMatchRegularExpression
	method := restOperation.Method
	hostname := makeHostname(epCtx, gwType)
	name := makeHTTPRouteNameForOperation(epCtx, gwType, string(restOperation.Method), restOperation.Target)
	port := gwapiv1.PortNumber(epCtx.Endpoint.Spec.BackendRef.ComponentRef.Port)
	prefix := makePathPrefix(epCtx)
	basePath := epCtx.Endpoint.Spec.BackendRef.BasePath
	endpointPath := path.Join(basePath, restOperation.Target)
	if epCtx.Component.Spec.Type == openchoreov1alpha1.ComponentTypeService {
		endpointPath = path.Clean(path.Join(prefix, endpointPath))
	}

	regexEpPath := GenerateRegexWithCaptureGroup(basePath, restOperation.Target, endpointPath)

	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(epCtx),
			Labels:    makeWorkloadLabels(epCtx, gwType),
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				ParentRefs: []gwapiv1.ParentReference{
					{
						Name:      gwapiv1.ObjectName(gwType),
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
									Name: gwapiv1.ObjectName(makeServiceName(epCtx)),
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

func extractPoliciesFromCtx(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) []openchoreov1alpha1.Policy {
	if epCtx.Endpoint.Spec.NetworkVisibilities == nil {
		return nil
	}

	switch gwType {
	case visibility.GatewayExternal:
		if epCtx.Endpoint.Spec.NetworkVisibilities.Public == nil ||
			epCtx.Endpoint.Spec.NetworkVisibilities.Public.Policies == nil {
			return nil
		}
		return epCtx.Endpoint.Spec.NetworkVisibilities.Public.Policies
	case visibility.GatewayInternal:
		if epCtx.Endpoint.Spec.NetworkVisibilities.Organization == nil ||
			epCtx.Endpoint.Spec.NetworkVisibilities.Organization.Policies == nil {
			return nil
		}
		return epCtx.Endpoint.Spec.NetworkVisibilities.Organization.Policies
	default:
		return nil
	}
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
