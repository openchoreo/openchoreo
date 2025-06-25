// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// SecurityPolicies renders the SecurityPolicy resources for the API.
func SecurityPolicies(rCtx *Context) []*choreov1.Resource {
	apiType := rCtx.API.Spec.Type
	switch apiType {
	case choreov1.EndpointTypeREST:
		return makeSecurityPolicies(rCtx)
	default:
		rCtx.AddError(fmt.Errorf("unsupported API type: %s", apiType))
		return nil
	}
}

func makeSecurityPolicies(rCtx *Context) []*choreov1.Resource {
	if rCtx.API.Spec.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing"))
		return nil
	}
	if rCtx.APIClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the API class"))
		return nil
	}

	// Generate SecurityPolicy for each expose level and operation
	securityPolicies := make([]*egv1a1.SecurityPolicy, 0)

	// Process each operation and its expose levels
	for _, operation := range rCtx.API.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == choreov1.ExposeLevelProject {
				continue // Skip project level for now
			}

			// Get merged policies for this expose level
			mergedPolicy, err := MergePoliciesForExposeLevel(rCtx.APIClass.Spec.RESTPolicy, exposeLevel)
			if err != nil {
				rCtx.AddError(fmt.Errorf("failed to merge policies for expose level %s: %w", exposeLevel, err))
				continue
			}

			securityPolicy := makeSecurityPolicyForRestOperation(rCtx, operation, exposeLevel, mergedPolicy)
			if securityPolicy != nil {
				securityPolicies = append(securityPolicies, securityPolicy)
			}
		}
	}

	resources := make([]*choreov1.Resource, 0, len(securityPolicies))

	for _, securityPolicy := range securityPolicies {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = securityPolicy

		resources = append(resources, &choreov1.Resource{
			ID:     makeSecurityPolicyResourceID(securityPolicy),
			Object: rawExt,
		})
	}

	return resources
}

func makeSecurityPolicyForRestOperation(rCtx *Context, restOperation choreov1.RESTEndpointOperation,
	exposeLevel choreov1.RESTOperationExposeLevel, mergedPolicy *choreov1.RESTPolicyWithConditionals) *egv1a1.SecurityPolicy {
	name := makeHTTPRouteName(rCtx, restOperation, exposeLevel)
	actionDeny := egv1a1.AuthorizationActionDeny
	actionAllow := egv1a1.AuthorizationActionAllow

	// Convert RESTOperation.Scopes to []egv1a1.JWTScope
	jwtScopes := make([]egv1a1.JWTScope, len(restOperation.Scopes))
	for i, scope := range restOperation.Scopes {
		jwtScopes[i] = egv1a1.JWTScope(scope)
	}

	securityPolicy := &egv1a1.SecurityPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.envoyproxy.io/v1alpha1",
			Kind:       "SecurityPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeAPILabels(rCtx),
		},
		Spec: egv1a1.SecurityPolicySpec{
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
		},
	}

	// Configure authentication if available
	if mergedPolicy != nil && mergedPolicy.Authentication != nil {
		if mergedPolicy.Authentication.Type == "jwt" {
			if mergedPolicy.Authentication.JWT != nil {
				securityPolicy.Spec.JWT = &egv1a1.JWT{
					Providers: []egv1a1.JWTProvider{
						{
							Name: "default",
							RemoteJWKS: egv1a1.RemoteJWKS{
								URI: mergedPolicy.Authentication.JWT.JWKS,
							},
							Issuer:    mergedPolicy.Authentication.JWT.Issuer,
							Audiences: mergedPolicy.Authentication.JWT.Audience,
						},
					},
				}

				// Add authorization if scopes are present
				if len(jwtScopes) > 0 {
					securityPolicy.Spec.Authorization = &egv1a1.Authorization{
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
					}
				}
			}
		}
	}

	return securityPolicy
}

func makeSecurityPolicyResourceID(policy *egv1a1.SecurityPolicy) string {
	return policy.Name
}
