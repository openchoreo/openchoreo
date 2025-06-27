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

// BackendTrafficPolicies renders the BackendTrafficPolicy resources for the given API context.
func BackendTrafficPolicies(rCtx *Context) []*choreov1.Resource {
	apiType := rCtx.API.Spec.Type
	switch apiType {
	case choreov1.EndpointTypeREST:
		return makeBackendTrafficPolicies(rCtx)
	default:
		rCtx.AddError(fmt.Errorf("unsupported API type: %s", apiType))
		return nil
	}
}

func makeBackendTrafficPolicies(rCtx *Context) []*choreov1.Resource {
	if rCtx.API.Spec.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing"))
		return nil
	}
	if rCtx.APIClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the API class"))
		return nil
	}

	// Generate BackendTrafficPolicy for each expose level and operation
	backendTrafficPolicies := make([]*egv1a1.BackendTrafficPolicy, 0)

	// Process each operation and its expose levels
	for _, operation := range rCtx.API.Spec.RESTEndpoint.Operations {
		for _, exposeLevel := range operation.ExposeLevels {
			if exposeLevel == choreov1.ExposeLevelProject {
				continue // Skip project level for now
			}
			backendTrafficPolicy := makeBackendTrafficPolicyForRestOperation(rCtx, operation, exposeLevel)
			if backendTrafficPolicy != nil {
				backendTrafficPolicies = append(backendTrafficPolicies, backendTrafficPolicy)
			}
		}
	}

	resources := make([]*choreov1.Resource, 0, len(backendTrafficPolicies))

	for _, backendTrafficPolicy := range backendTrafficPolicies {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = backendTrafficPolicy

		resources = append(resources, &choreov1.Resource{
			ID:     makeBackendTrafficPolicyResourceID(backendTrafficPolicy),
			Object: rawExt,
		})
	}

	return resources
}

func makeBackendTrafficPolicyForRestOperation(rCtx *Context, restOperation choreov1.RESTEndpointOperation,
	exposeLevel choreov1.RESTOperationExposeLevel) *egv1a1.BackendTrafficPolicy {
	name := makeHTTPRouteName(rCtx, restOperation, exposeLevel)

	// Resolve rate limit policy for this operation and expose level
	rateLimitConfig := resolveRateLimitPolicy(rCtx, restOperation, exposeLevel)
	if rateLimitConfig == nil {
		// No rate limiting configured for this operation
		return nil
	}

	// Convert rate limit window to BackendTrafficPolicy format
	rateLimitType := egv1a1.RateLimitType("Local")
	rateLimitUnit := convertWindowToUnit(rateLimitConfig.Window)

	return &egv1a1.BackendTrafficPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.envoyproxy.io/v1alpha1",
			Kind:       "BackendTrafficPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeAPILabels(rCtx),
		},
		Spec: egv1a1.BackendTrafficPolicySpec{
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
			RateLimit: &egv1a1.RateLimitSpec{
				Type: rateLimitType,
				Local: &egv1a1.LocalRateLimit{
					Rules: []egv1a1.RateLimitRule{
						{
							ClientSelectors: buildClientSelectors(rateLimitConfig),
							Limit: egv1a1.RateLimitValue{
								Requests: convertToUint(rateLimitConfig.Requests),
								Unit:     rateLimitUnit,
							},
						},
					},
				},
			},
		},
	}
}

// resolveRateLimitPolicy resolves the rate limit configuration for a specific operation and expose level
func resolveRateLimitPolicy(rCtx *Context, restOperation choreov1.RESTEndpointOperation,
	exposeLevel choreov1.RESTOperationExposeLevel) *choreov1.RateLimitPolicy {
	restPolicy := rCtx.APIClass.Spec.RESTPolicy

	// Start with defaults
	var basePolicy *choreov1.RESTPolicyWithConditionals
	if restPolicy.Defaults != nil {
		basePolicy = restPolicy.Defaults
	}

	// Apply expose level overrides
	var exposeLevelPolicy *choreov1.RESTPolicyWithConditionals
	switch exposeLevel {
	case choreov1.ExposeLevelPublic:
		if restPolicy.Public != nil {
			exposeLevelPolicy = restPolicy.Public
		}
	case choreov1.ExposeLevelOrganization:
		if restPolicy.Organization != nil {
			exposeLevelPolicy = restPolicy.Organization
		}
	}

	// Merge policies: expose level overrides defaults
	var mergedPolicy *choreov1.RESTPolicy
	if exposeLevelPolicy != nil {
		mergedPolicy = &exposeLevelPolicy.RESTPolicy
		// If expose level policy doesn't have rate limit, fall back to defaults
		if mergedPolicy.RateLimit == nil && basePolicy != nil {
			mergedPolicy.RateLimit = basePolicy.RateLimit
		}
	} else if basePolicy != nil {
		mergedPolicy = &basePolicy.RESTPolicy
	}

	if mergedPolicy == nil || mergedPolicy.RateLimit == nil {
		return nil
	}

	// Check for conditional policies that match this operation
	var conditionalPolicies []choreov1.RESTConditionalPolicy
	if exposeLevelPolicy != nil {
		conditionalPolicies = exposeLevelPolicy.ConditionalPolicies
	} else if basePolicy != nil {
		conditionalPolicies = basePolicy.ConditionalPolicies
	}

	// Apply conditional policy overrides
	for _, conditionalPolicy := range conditionalPolicies {
		if conditionalPolicy.Condition != nil && matchesCondition(conditionalPolicy.Condition, restOperation) {
			if conditionalPolicy.Policy != nil && conditionalPolicy.Policy.RateLimit != nil {
				return conditionalPolicy.Policy.RateLimit
			}
		}
	}

	return mergedPolicy.RateLimit
}

// matchesCondition checks if a REST operation matches the given policy condition
func matchesCondition(condition *choreov1.RESTPolicyCondition, operation choreov1.RESTEndpointOperation) bool {
	// Check method match
	if condition.Method != nil && *condition.Method != operation.Method {
		return false
	}

	// Check path match
	if len(condition.Paths) > 0 {
		pathMatched := false
		for _, conditionPath := range condition.Paths {
			if conditionPath == operation.Path {
				pathMatched = true
				break
			}
		}
		if !pathMatched {
			return false
		}
	}

	return true
}

// buildClientSelectors builds client selectors for rate limiting based on the configuration
func buildClientSelectors(rateLimitConfig *choreov1.RateLimitPolicy) []egv1a1.RateLimitSelectCondition {
	if rateLimitConfig.KeyBy == nil {
		// Default: rate limit by client IP (all IPs)
		return []egv1a1.RateLimitSelectCondition{
			{
				SourceCIDR: &egv1a1.SourceMatch{
					Value: "0.0.0.0/0",
				},
			},
		}
	}

	keyBy := *rateLimitConfig.KeyBy

	// Parse keyBy format: "header:X-API-Key", "jwt:sub", "clientIP"
	switch {
	case keyBy == "clientIP":
		return []egv1a1.RateLimitSelectCondition{
			{
				SourceCIDR: &egv1a1.SourceMatch{
					Value: "0.0.0.0/0",
				},
			},
		}
	case len(keyBy) > 7 && keyBy[:7] == "header:":
		headerName := keyBy[7:]
		return []egv1a1.RateLimitSelectCondition{
			{
				Headers: []egv1a1.HeaderMatch{
					{
						Name: headerName,
					},
				},
			},
		}
	case len(keyBy) > 4 && keyBy[:4] == "jwt:":
		// JWT-based rate limiting would require additional implementation
		// For now, fall back to client IP
		return []egv1a1.RateLimitSelectCondition{
			{
				SourceCIDR: &egv1a1.SourceMatch{
					Value: "0.0.0.0/0",
				},
			},
		}
	default:
		// Default: rate limit by client IP (all IPs)
		return []egv1a1.RateLimitSelectCondition{
			{
				SourceCIDR: &egv1a1.SourceMatch{
					Value: "0.0.0.0/0",
				},
			},
		}
	}
}

// convertToUint safely converts int64 to uint with bounds checking
func convertToUint(value int64) uint {
	if value < 0 {
		return 0
	}
	return uint(value)
}

// convertWindowToUnit converts rate limit window string to BackendTrafficPolicy unit
func convertWindowToUnit(window string) egv1a1.RateLimitUnit {
	switch window {
	case "Second", "second", "1s":
		return egv1a1.RateLimitUnit("Second")
	case "Minute", "minute", "1m":
		return egv1a1.RateLimitUnit("Minute")
	case "Hour", "hour", "1h":
		return egv1a1.RateLimitUnit("Hour")
	case "Day", "day", "1d":
		return egv1a1.RateLimitUnit("Day")
	default:
		// Default to minute if unknown
		return egv1a1.RateLimitUnit("Minute")
	}
}

// makeBackendTrafficPolicyResourceID generates a resource ID for the BackendTrafficPolicy
func makeBackendTrafficPolicyResourceID(policy *egv1a1.BackendTrafficPolicy) string {
	return policy.Name
}
