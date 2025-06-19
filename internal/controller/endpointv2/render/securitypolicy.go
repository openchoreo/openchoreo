// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// SecurityPolicies renders the SecurityPolicy resources for the given endpoint context.
func SecurityPolicies(rCtx *Context) []*choreov1.Resource {
	epType := rCtx.EndpointV2.Spec.Type
	switch epType {
	case choreov1.EndpointTypeREST:
		return makeSecurityPolicies(rCtx)
	default:
		rCtx.AddError(UnsupportedEndpointTypeError(epType))
		return nil
	}
}

func makeSecurityPolicies(rCtx *Context) []*choreov1.Resource {
	if rCtx.EndpointV2.Spec.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing"))
		return nil
	}
	if rCtx.EndpointClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the endpoint class"))
		return nil
	}

	// Generate SecurityPolicy for each expose level and operation
	httpRouteFilters := make([]*egv1a1.SecurityPolicy, 0)

	// Process each operation and its expose levels
	for _, operation := range rCtx.EndpointV2.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == choreov1.ExposeLevelProject {
				continue
			}
			httpRouteFilter := makeSecurityPolicyForRestOperation(rCtx, operation, exposeLevel)
			if httpRouteFilter != nil {
				httpRouteFilters = append(httpRouteFilters, httpRouteFilter)
			}
		}
	}

	resources := make([]*choreov1.Resource, len(httpRouteFilters))

	for _, httpRouteFilter := range httpRouteFilters {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = httpRouteFilter

		resources = append(resources, &choreov1.Resource{
			ID:     makeSecurityPolicyResourceId(httpRouteFilter),
			Object: rawExt,
		})
	}

	return resources
}

func makeSecurityPolicyForRestOperation(rCtx *Context, restOperation choreov1.RESTEndpointOperation,
	exposeLevel choreov1.RESTOperationExposeLevel) *egv1a1.SecurityPolicy {
	name := makeHTTPRouteName(rCtx, restOperation, exposeLevel)
	actionDeny := egv1a1.AuthorizationActionDeny
	actionAllow := egv1a1.AuthorizationActionAllow

	// Convert RESTOperation.Scopes to []egv1a1.JWTScope
	jwtScopes := make([]egv1a1.JWTScope, len(restOperation.Scopes))
	for i, scope := range restOperation.Scopes {
		jwtScopes[i] = egv1a1.JWTScope(scope)
	}

	//mergedEndpointPolicy :=

	return &egv1a1.SecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeEndpointLabels(rCtx),
		},
		Spec: egv1a1.SecurityPolicySpec{
			JWT: &egv1a1.JWT{
				Providers: []egv1a1.JWTProvider{
					{
						Name: "default",
						RemoteJWKS: egv1a1.RemoteJWKS{
							URI: "rCtx.EndpointClass.Spec.RESTPolicy.",
						},
					},
				},
			},
			PolicyTargetReferences: egv1a1.PolicyTargetReferences{
				TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
							Group: gwapiv1.GroupName,
							Kind:  "HTTPRoute",
							Name:  gwapiv1a2.ObjectName(name),
						},
					},
				},
			},
			Authorization: &egv1a1.Authorization{
				Rules: []egv1a1.AuthorizationRule{
					{
						Principal: egv1a1.Principal{
							JWT: &egv1a1.JWTPrincipal{
								Provider: "default",
								Scopes:   jwtScopes,
							},
						},
						Action: actionAllow,
					},
				},
				DefaultAction: &actionDeny,
			},
		},
	}
}

// TODO: Find a better way to generate resource IDs
func makeSecurityPolicyResourceId(policy *egv1a1.SecurityPolicy) string {
	return policy.Name
}
