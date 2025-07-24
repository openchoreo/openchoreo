// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"strings"
	"time"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// SecurityPolicies renders the SecurityPolicy resources for the given ServiceBinding context.
func SecurityPolicies(rCtx Context) []*openchoreov1alpha1.Resource {
	if rCtx.ServiceBinding.Spec.APIs == nil || len(rCtx.ServiceBinding.Spec.APIs) == 0 {
		return nil
	}

	if rCtx.APIClasses == nil {
		rCtx.AddError(fmt.Errorf("APIClass is required for SecurityPolicy generation"))
		return nil
	}

	securityPolicies := make([]*egv1a1.SecurityPolicy, 0)

	// Generate SecurityPolicy per API and expose level
	for apiName, serviceAPI := range rCtx.ServiceBinding.Spec.APIs {
		if serviceAPI.Type != openchoreov1alpha1.EndpointTypeREST {
			continue // Skip non-REST APIs
		}

		if serviceAPI.RESTEndpoint == nil {
			rCtx.AddError(fmt.Errorf("REST endpoint specification is missing for API %s", apiName))
			continue
		}

		// Get the corresponding APIClass for this API
		apiClass, exists := rCtx.APIClasses[apiName]
		if !exists || apiClass == nil {
			rCtx.AddError(fmt.Errorf("APIClass not found for API %s", apiName))
			continue
		}

		// Generate SecurityPolicy for each expose level
		if len(serviceAPI.RESTEndpoint.ExposeLevels) == 0 {
			// Default to Organization level if no expose levels specified
			securityPolicy := makeSecurityPolicyForServiceAPI(rCtx, apiName, serviceAPI, apiClass, openchoreov1alpha1.ExposeLevelOrganization)
			if securityPolicy != nil {
				securityPolicies = append(securityPolicies, securityPolicy)
			}
		} else {
			for _, exposeLevel := range serviceAPI.RESTEndpoint.ExposeLevels {
				if exposeLevel == openchoreov1alpha1.ExposeLevelProject {
					continue // Skip project level for now
				}
				securityPolicy := makeSecurityPolicyForServiceAPI(rCtx, apiName, serviceAPI, apiClass, exposeLevel)
				if securityPolicy != nil {
					securityPolicies = append(securityPolicies, securityPolicy)
				}
			}
		}
	}

	resources := make([]*openchoreov1alpha1.Resource, 0, len(securityPolicies))

	for _, securityPolicy := range securityPolicies {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = securityPolicy

		resources = append(resources, &openchoreov1alpha1.Resource{
			ID:     makeSecurityPolicyResourceID(securityPolicy),
			Object: rawExt,
		})
	}

	return resources
}

func makeSecurityPolicyForServiceAPI(rCtx Context, apiName string, serviceAPI *openchoreov1alpha1.ServiceAPI, apiClass *openchoreov1alpha1.APIClass, exposeLevel openchoreov1alpha1.RESTOperationExposeLevel) *egv1a1.SecurityPolicy {
	if serviceAPI.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing for API %s", apiName))
		return nil
	}

	if apiClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the API class for API %s", apiName))
		return nil
	}

	name := makeSecurityPolicyName(rCtx, apiName, exposeLevel)
	httpRouteName := makeHTTPRouteName(&rCtx, apiName, exposeLevel)

	// Get the merged REST policy for the expose level
	mergedPolicy := getMergedRESTPolicy(apiClass.Spec.RESTPolicy, exposeLevel)
	if mergedPolicy == nil {
		// No security policy defined for this expose level
		return nil
	}

	securityPolicy := &egv1a1.SecurityPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.envoyproxy.io/v1alpha1",
			Kind:       "SecurityPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeServiceLabels(rCtx),
		},
		Spec: egv1a1.SecurityPolicySpec{
			PolicyTargetReferences: egv1a1.PolicyTargetReferences{
				TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
							Group: gwapiv1.GroupName,
							Kind:  "HTTPRoute",
							Name:  gwapiv1a2.ObjectName(httpRouteName),
						},
					},
				},
			},
		},
	}

	// Configure JWT authentication if specified
	if mergedPolicy.Authentication != nil && mergedPolicy.Authentication.Type == "jwt" && mergedPolicy.Authentication.JWT != nil {
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

		// Configure authorization rules
		actionAllow := egv1a1.AuthorizationActionAllow
		actionDeny := egv1a1.AuthorizationActionDeny

		// Convert RESTOperation.Scopes to []egv1a1.JWTScope
		if mergedPolicy.Authentication.OAuth2 != nil && len(mergedPolicy.Authentication.OAuth2.Scopes) > 0 {
			jwtScopes := make([]egv1a1.JWTScope, len(mergedPolicy.Authentication.OAuth2.Scopes))
			for i, scope := range mergedPolicy.Authentication.OAuth2.Scopes {
				jwtScopes[i] = egv1a1.JWTScope(scope)
			}

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

	// Configure CORS if specified
	if mergedPolicy.CORS != nil {
		securityPolicy.Spec.CORS = &egv1a1.CORS{
			AllowOrigins:  convertStringSliceToOrigins(mergedPolicy.CORS.AllowOrigins),
			AllowMethods:  mergedPolicy.CORS.AllowMethods,
			AllowHeaders:  mergedPolicy.CORS.AllowHeaders,
			ExposeHeaders: mergedPolicy.CORS.ExposeHeaders,
		}
		if mergedPolicy.CORS.MaxAge != nil {
			duration := &metav1.Duration{Duration: time.Duration(*mergedPolicy.CORS.MaxAge) * time.Second}
			securityPolicy.Spec.CORS.MaxAge = duration
		}
	}

	// Configure security policies (IP allowlists/blocklists)
	if mergedPolicy.Security != nil {
		if len(mergedPolicy.Security.AllowedIPs) > 0 || len(mergedPolicy.Security.BlockedIPs) > 0 {
			// Configure IP-based authorization rules
			if securityPolicy.Spec.Authorization == nil {
				actionDeny := egv1a1.AuthorizationActionDeny
				securityPolicy.Spec.Authorization = &egv1a1.Authorization{
					Rules:         []egv1a1.AuthorizationRule{},
					DefaultAction: &actionDeny,
				}
			}

			// Add allow rules for allowed IPs
			for _, allowedIP := range mergedPolicy.Security.AllowedIPs {
				actionAllow := egv1a1.AuthorizationActionAllow
				securityPolicy.Spec.Authorization.Rules = append(securityPolicy.Spec.Authorization.Rules, egv1a1.AuthorizationRule{
					Principal: egv1a1.Principal{
						ClientCIDRs: convertStringSliceToCIDRs([]string{allowedIP}),
					},
					Action: actionAllow,
				})
			}

			// Add deny rules for blocked IPs
			for _, blockedIP := range mergedPolicy.Security.BlockedIPs {
				actionDeny := egv1a1.AuthorizationActionDeny
				securityPolicy.Spec.Authorization.Rules = append(securityPolicy.Spec.Authorization.Rules, egv1a1.AuthorizationRule{
					Principal: egv1a1.Principal{
						ClientCIDRs: convertStringSliceToCIDRs([]string{blockedIP}),
					},
					Action: actionDeny,
				})
			}
		}
	}

	return securityPolicy
}

// getMergedRESTPolicy returns the merged REST policy for the given expose level
func getMergedRESTPolicy(restPolicy *openchoreov1alpha1.RESTAPIPolicy, exposeLevel openchoreov1alpha1.RESTOperationExposeLevel) *openchoreov1alpha1.RESTPolicy {
	var mergedPolicy *openchoreov1alpha1.RESTPolicy

	// Start with defaults if available
	if restPolicy.Defaults != nil {
		mergedPolicy = restPolicy.Defaults.DeepCopy()
	}

	// Apply expose level-specific overrides
	var override *openchoreov1alpha1.RESTPolicy
	switch exposeLevel {
	case openchoreov1alpha1.ExposeLevelPublic:
		override = restPolicy.Public
	case openchoreov1alpha1.ExposeLevelOrganization:
		override = restPolicy.Organization
	}

	if override != nil {
		if mergedPolicy == nil {
			mergedPolicy = override.DeepCopy()
		} else {
			// Merge override into defaults
			var err error
			mergedPolicy, err = merge(mergedPolicy, override)
			if err != nil {
				return nil
			}
		}
	}

	return mergedPolicy
}

// convertStringSliceToOrigins converts a slice of strings to a slice of Origins
func convertStringSliceToOrigins(origins []string) []egv1a1.Origin {
	if len(origins) == 0 {
		return nil
	}

	originList := make([]egv1a1.Origin, len(origins))
	for i, origin := range origins {
		originList[i] = egv1a1.Origin(origin)
	}
	return originList
}

// convertStringSliceToCIDRs converts a slice of strings to a slice of CIDRs
func convertStringSliceToCIDRs(cidrs []string) []egv1a1.CIDR {
	if len(cidrs) == 0 {
		return nil
	}

	cidrList := make([]egv1a1.CIDR, len(cidrs))
	for i, cidr := range cidrs {
		cidrList[i] = egv1a1.CIDR(cidr)
	}
	return cidrList
}

func makeSecurityPolicyName(rCtx Context, apiName string, exposeLevel openchoreov1alpha1.RESTOperationExposeLevel) string {
	// Create a unique name for the SecurityPolicy using ServiceBinding name, API name and expose level
	exposeLevelStr := strings.ToLower(string(exposeLevel))
	return dpkubernetes.GenerateK8sName(rCtx.ServiceBinding.Name, apiName, exposeLevelStr)
}

func makeSecurityPolicyResourceID(securityPolicy *egv1a1.SecurityPolicy) string {
	return securityPolicy.Name
}
