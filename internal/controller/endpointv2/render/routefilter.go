// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"path"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// HTTPRouteFilters renders the HTTPRoute resources for the given endpoint context.
func HTTPRouteFilters(rCtx *Context) []*openchoreov1alpha1.Resource {
	epType := rCtx.EndpointV2.Spec.Type
	switch epType {
	case openchoreov1alpha1.EndpointTypeREST:
		return makeRESTHTTPRouteFilters(rCtx)
	default:
		rCtx.AddError(UnsupportedEndpointTypeError(epType))
		return nil
	}
}

func makeRESTHTTPRouteFilters(rCtx *Context) []*openchoreov1alpha1.Resource {
	if rCtx.EndpointV2.Spec.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing"))
		return nil
	}
	if rCtx.EndpointClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the endpoint class"))
		return nil
	}

	// Generate HTTPRouteFilters for each expose level and operation
	httpRouteFilters := make([]*egv1a1.HTTPRouteFilter, 0)

	// Process each operation and its expose levels
	for _, operation := range rCtx.EndpointV2.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == openchoreov1alpha1.ExposeLevelProject {
				continue
			}
			httpRouteFilter := makeHTTPRouteFilterForRestOperation(rCtx, operation, exposeLevel)
			if httpRouteFilter != nil {
				httpRouteFilters = append(httpRouteFilters, httpRouteFilter)
			}
		}
	}

	resources := make([]*openchoreov1alpha1.Resource, len(httpRouteFilters))

	for _, httpRouteFilter := range httpRouteFilters {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = httpRouteFilter

		resources = append(resources, &openchoreov1alpha1.Resource{
			ID:     makeHTTPRouteFilterResourceID(httpRouteFilter),
			Object: rawExt,
		})
	}

	return resources
}

func makeHTTPRouteFilterForRestOperation(rCtx *Context, restOperation openchoreov1alpha1.RESTEndpointOperation,
	exposeLevel openchoreov1alpha1.RESTOperationExposeLevel) *egv1a1.HTTPRouteFilter {
	basePath := rCtx.EndpointV2.Spec.RESTEndpoint.Backend.BasePath
	endpointPath := path.Join(basePath, restOperation.Path)
	pattern := GenerateRegexWithCaptureGroup(basePath, restOperation.Path, endpointPath)

	filter := &egv1a1.HTTPRouteFilter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(rCtx, restOperation, exposeLevel),
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeEndpointLabels(rCtx),
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

// TODO: Find a better way to generate resource IDs
func makeHTTPRouteFilterResourceID(httpRouteFilter *egv1a1.HTTPRouteFilter) string {
	return httpRouteFilter.Name
}
