// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"path"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

type httpRouteFiltersHandler struct {
	client     client.Client
	visibility visibility.VisibilityStrategy
}

var _ dataplane.ResourceHandler[dataplane.EndpointContext] = (*httpRouteFiltersHandler)(nil)

func NewHTTPRouteFiltersHandler(kubernetesClient client.Client, visibility visibility.VisibilityStrategy) dataplane.ResourceHandler[dataplane.EndpointContext] {
	return &httpRouteFiltersHandler{
		client:     kubernetesClient,
		visibility: visibility,
	}
}

func (h httpRouteFiltersHandler) Name() string {
	return "KubernetesHTTPRouteFiltersHandler"
}

func (h httpRouteFiltersHandler) IsRequired(epCtx *dataplane.EndpointContext) bool {
	return h.visibility.IsHTTPRouteFilterRequired(epCtx)
}

func (h httpRouteFiltersHandler) GetCurrentState(ctx context.Context, epCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := makeNamespaceName(epCtx)
	labels := makeWorkloadLabels(epCtx, h.visibility.GetGatewayType())

	listOption := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	out := &egv1a1.HTTPRouteFilterList{}
	err := h.client.List(ctx, out, listOption...)
	if err != nil {
		return nil, fmt.Errorf("error while listing HTTPRouteFilters: %w", err)
	}
	return out, nil
}

func (h httpRouteFiltersHandler) Create(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	httpRouteFilters := MakeHTTPRouteFilters(epCtx, h.visibility.GetGatewayType())
	for _, filter := range httpRouteFilters {
		if err := h.client.Create(ctx, filter); err != nil {
			return fmt.Errorf("error while creating HTTPRouteFilter %s: %w", filter.Name, err)
		}
	}
	return nil
}

func (h httpRouteFiltersHandler) Update(ctx context.Context, epCtx *dataplane.EndpointContext, currentState interface{}) error {
	currentHTTPRouteFiltersList, ok := currentState.(*egv1a1.HTTPRouteFilterList)
	currentHTTPRouteFilters := currentHTTPRouteFiltersList.Items
	if !ok {
		return errors.New("failed to cast current state to the list of HTTPRouteFilters")
	}

	desiredHTTPRouteFilters := MakeHTTPRouteFilters(epCtx, h.visibility.GetGatewayType())

	// Create a map of current HTTP routes by name for easier lookup
	currentHTTPRouteFiltersMap := make(map[string]*egv1a1.HTTPRouteFilter)
	for _, rf := range currentHTTPRouteFilters {
		currentHTTPRouteFiltersMap[rf.Name] = &rf
	}

	// Create a map of desired HTTP route filters by name for easier lookup
	desiredHTTPRouteFiltersMap := make(map[string]*egv1a1.HTTPRouteFilter)
	for _, routeFilter := range desiredHTTPRouteFilters {
		desiredHTTPRouteFiltersMap[routeFilter.Name] = routeFilter
	}

	// Update each HTTPRouteFilter individually if needed
	for name, newHTTPRouteFilter := range desiredHTTPRouteFiltersMap {
		currentHTTPRouteFilter, exists := currentHTTPRouteFiltersMap[name]
		if !exists {
			// If the routeFilter doesn't exist in the current state, create it
			if err := h.client.Create(ctx, newHTTPRouteFilter); err != nil {
				return fmt.Errorf("error while creating HTTPRouteFilter %s: %w", newHTTPRouteFilter.Name, err)
			}
			continue
		}

		// Check if the route needs to be updated
		if !cmp.Equal(currentHTTPRouteFilter.Spec, newHTTPRouteFilter.Spec) ||
			!cmp.Equal(extractManagedLabels(currentHTTPRouteFilter.Labels), extractManagedLabels(newHTTPRouteFilter.Labels)) {
			updatedHTTPRouteFilter := currentHTTPRouteFilter.DeepCopy()
			updatedHTTPRouteFilter.Spec = newHTTPRouteFilter.Spec
			updatedHTTPRouteFilter.Labels = newHTTPRouteFilter.Labels

			if err := h.client.Update(ctx, updatedHTTPRouteFilter); err != nil {
				return fmt.Errorf("error while updating HTTPRouteFilter %s: %w", newHTTPRouteFilter.Name, err)
			}
		}
	}

	// Delete HTTPRouteFilters that exist in the current state but not in the desired state
	for name, currentHTTPRouteFilter := range currentHTTPRouteFiltersMap {
		if _, exists := desiredHTTPRouteFiltersMap[name]; !exists {
			if err := h.client.Delete(ctx, currentHTTPRouteFilter); err != nil {
				if !apierrors.IsNotFound(err) {
					return fmt.Errorf("error while deleting HTTPRouteFilter %s: %w", currentHTTPRouteFilter.Name, err)
				}
			}
		}
	}

	return nil
}

func (h httpRouteFiltersHandler) Delete(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	namespace := makeNamespaceName(epCtx)
	labels := makeWorkloadLabels(epCtx, h.visibility.GetGatewayType())
	deleteAllOption := []client.DeleteAllOfOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	err := h.client.DeleteAllOf(ctx, &egv1a1.HTTPRouteFilter{}, deleteAllOption...)
	if err != nil {
		return fmt.Errorf("error while deleting HTTPRouteFilters: %w", err)
	}

	return nil
}

func MakeHTTPRouteFilters(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) []*egv1a1.HTTPRouteFilter {
	out := make([]*egv1a1.HTTPRouteFilter, 0)

	policies := extractPoliciesFromCtx(epCtx, gwType)
	for _, policy := range policies {
		// Skip policies without specs or if not OAuth2 type
		if policy.PolicySpec == nil || policy.Type != choreov1.Oauth2PolicyType {
			continue
		}

		// Skip if OAuth2 config is missing or JWT operations are not configured
		if policy.PolicySpec.OAuth2 == nil ||
			policy.PolicySpec.OAuth2.JWT.Authorization.Rest == nil ||
			policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations == nil {
			continue
		}

		// Generate a separate HTTPRouteFilter for each operation
		operations := policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations
		for _, op := range *operations {
			httpRouteFilter := makeHTTPRouteFilterForOperation(epCtx, gwType, &op)
			out = append(out, httpRouteFilter)
		}
	}

	return out
}

func makeHTTPRouteFilterForOperation(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType, restOperation *choreov1.RESTOperation) *egv1a1.HTTPRouteFilter {
	basePath := epCtx.Endpoint.Spec.BackendRef.BasePath
	prefix := makePathPrefix(epCtx)
	endpointPath := path.Join(basePath, restOperation.Target)

	if epCtx.Component.Spec.Type == choreov1.ComponentTypeService {
		endpointPath = path.Clean(path.Join(prefix, endpointPath))
	}
	pattern := GenerateRegexWithCaptureGroup(basePath, restOperation.Target, endpointPath)

	filter := &egv1a1.HTTPRouteFilter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteNameForOperation(epCtx, gwType, string(restOperation.Method), restOperation.Target),
			Namespace: makeNamespaceName(epCtx),
			Labels:    makeWorkloadLabels(epCtx, gwType),
		},
		Spec: egv1a1.HTTPRouteFilterSpec{
			URLRewrite: &egv1a1.HTTPURLRewriteFilter{
				Path: &egv1a1.HTTPPathModifier{
					Type: egv1a1.RegexHTTPPathModifier,
					ReplaceRegexMatch: &egv1a1.ReplaceRegexMatch{
						Pattern:      pattern,
						Substitution: "\\1",
					},
				},
			},
		},
	}

	return filter
}
