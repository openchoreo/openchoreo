// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"path"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// HTTPRouteFilters renders the HTTPRouteFilter resources for regex-based path replacement.
func HTTPRouteFilters(rCtx *Context) []*choreov1.Resource {
	apiType := rCtx.API.Spec.Type
	switch apiType {
	case choreov1.EndpointTypeREST:
		return makeRESTHTTPRouteFilters(rCtx)
	default:
		rCtx.AddError(fmt.Errorf("unsupported API type: %s", apiType))
		return nil
	}
}

func makeRESTHTTPRouteFilters(rCtx *Context) []*choreov1.Resource {
	if rCtx.API.Spec.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing"))
		return nil
	}
	if rCtx.APIClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the API class"))
		return nil
	}

	// Generate HTTPRouteFilters for each expose level and operation
	httpRouteFilters := make([]*egv1a1.HTTPRouteFilter, 0)

	// Process each operation and its expose levels
	for _, operation := range rCtx.API.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == choreov1.ExposeLevelProject {
				continue // Skip project level for now
			}
			httpRouteFilter := makeHTTPRouteFilterForRestOperation(rCtx, operation, exposeLevel)
			if httpRouteFilter != nil {
				httpRouteFilters = append(httpRouteFilters, httpRouteFilter)
			}
		}
	}

	resources := make([]*choreov1.Resource, 0, len(httpRouteFilters))

	for _, httpRouteFilter := range httpRouteFilters {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = httpRouteFilter

		resources = append(resources, &choreov1.Resource{
			ID:     makeHTTPRouteFilterResourceID(httpRouteFilter),
			Object: rawExt,
		})
	}

	return resources
}

func makeHTTPRouteFilterForRestOperation(rCtx *Context, restOperation choreov1.RESTEndpointOperation,
	exposeLevel choreov1.RESTOperationExposeLevel) *egv1a1.HTTPRouteFilter {
	basePath := rCtx.API.Spec.RESTEndpoint.Backend.BasePath
	endpointPath := path.Join(basePath, restOperation.Path)
	pattern := GenerateRegexWithCaptureGroup(basePath, restOperation.Path, endpointPath)

	filter := &egv1a1.HTTPRouteFilter{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.envoyproxy.io/v1alpha1",
			Kind:       "HTTPRouteFilter",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(rCtx, restOperation, exposeLevel),
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeAPILabels(rCtx),
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

func makeHTTPRouteFilterResourceID(httpRouteFilter *egv1a1.HTTPRouteFilter) string {
	return httpRouteFilter.Name
}
