// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/ptr"
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
	httpRoutes := MakeHTTPRoutes(epCtx, h.visibility.GetGatewayType())
	var out []*gwapiv1.HTTPRoute
	for _, httpRoute := range httpRoutes {
		name := httpRoute.Name
		r := &gwapiv1.HTTPRoute{}
		err := h.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, r)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		out = append(out, r)
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
	currentHTTPRoutes, ok := currentState.([]*gwapiv1.HTTPRoute)
	if !ok {
		return errors.New("failed to cast current state to the list of HTTPRoutes")
	}

	desiredHTTPRoutes := MakeHTTPRoutes(epCtx, h.visibility.GetGatewayType())

	// Create a map of current HTTP routes by name for easier lookup
	currentHTTPRoutesMap := make(map[string]*gwapiv1.HTTPRoute)
	for _, route := range currentHTTPRoutes {
		currentHTTPRoutesMap[route.Name] = route
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

			if err := h.client.Update(ctx, newHTTPRoute); err != nil {
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
	labels := makeWorkloadLabels(epCtx)
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

	for _, policy := range policies {
		// Skip policies without specs or if not OAuth2 type
		if policy.PolicySpec == nil || policy.Type != "oauth2" {
			continue
		}

		// Skip if OAuth2 config is missing or JWT operations are not configured
		if policy.PolicySpec.OAuth2 == nil ||
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

// makeHTTPRouteForOperation creates an HTTPRoute for a specific REST operation
func makeHTTPRouteForOperation(epCtx *dataplane.EndpointContext, RESTOperation *choreov1.RESTOperation,
	gwType visibility.GatewayType) *gwapiv1.HTTPRoute {
	pathType := gwapiv1.PathMatchPathPrefix
	hostname := makeHostname(epCtx, gwType)
	port := gwapiv1.PortNumber(epCtx.Endpoint.Spec.BackendRef.ComponentRef.Port)
	prefix := makePathPrefix(epCtx)
	basePath := epCtx.Endpoint.Spec.BackendRef.BasePath
	endpointPath := basePath
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeService {
		endpointPath = path.Clean(path.Join(prefix, basePath))
	}

	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(epCtx, gwType, RESTOperation.Method, RESTOperation.Target),
			Namespace: makeNamespaceName(epCtx),
			Labels:    makeWorkloadLabels(epCtx),
		},
		Spec: gwapiv1.HTTPRouteSpec{
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

func extractPoliciesFromCtx(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) []choreov1.Policy {
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
