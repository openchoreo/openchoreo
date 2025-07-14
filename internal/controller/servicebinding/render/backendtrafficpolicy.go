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

// BackendTrafficPolicies renders the BackendTrafficPolicy resources for the given ServiceBinding context.
func BackendTrafficPolicies(rCtx Context) []*openchoreov1alpha1.Resource {
	if rCtx.ServiceBinding.Spec.APIs == nil || len(rCtx.ServiceBinding.Spec.APIs) == 0 {
		return nil
	}

	if rCtx.APIClasses == nil {
		rCtx.AddError(fmt.Errorf("APIClass is required for BackendTrafficPolicy generation"))
		return nil
	}

	backendTrafficPolicies := make([]*egv1a1.BackendTrafficPolicy, 0)

	// Generate BackendTrafficPolicy per API and expose level
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

		// Generate BackendTrafficPolicy for each expose level
		if len(serviceAPI.RESTEndpoint.ExposeLevels) == 0 {
			// Default to Organization level if no expose levels specified
			backendTrafficPolicy := makeBackendTrafficPolicyForServiceAPI(rCtx, apiName, serviceAPI, apiClass, openchoreov1alpha1.ExposeLevelOrganization)
			if backendTrafficPolicy != nil {
				backendTrafficPolicies = append(backendTrafficPolicies, backendTrafficPolicy)
			}
		} else {
			for _, exposeLevel := range serviceAPI.RESTEndpoint.ExposeLevels {
				if exposeLevel == openchoreov1alpha1.ExposeLevelProject {
					continue // Skip project level for now
				}
				backendTrafficPolicy := makeBackendTrafficPolicyForServiceAPI(rCtx, apiName, serviceAPI, apiClass, exposeLevel)
				if backendTrafficPolicy != nil {
					backendTrafficPolicies = append(backendTrafficPolicies, backendTrafficPolicy)
				}
			}
		}
	}

	resources := make([]*openchoreov1alpha1.Resource, 0, len(backendTrafficPolicies))

	for _, backendTrafficPolicy := range backendTrafficPolicies {
		rawExt := &runtime.RawExtension{}
		rawExt.Object = backendTrafficPolicy

		resources = append(resources, &openchoreov1alpha1.Resource{
			ID:     makeBackendTrafficPolicyResourceID(backendTrafficPolicy),
			Object: rawExt,
		})
	}

	return resources
}

func makeBackendTrafficPolicyForServiceAPI(rCtx Context, apiName string, serviceAPI *openchoreov1alpha1.ServiceAPI, apiClass *openchoreov1alpha1.APIClass, exposeLevel openchoreov1alpha1.RESTOperationExposeLevel) *egv1a1.BackendTrafficPolicy {
	if serviceAPI.RESTEndpoint == nil {
		rCtx.AddError(fmt.Errorf("REST endpoint specification is missing for API %s", apiName))
		return nil
	}

	if apiClass.Spec.RESTPolicy == nil {
		rCtx.AddError(fmt.Errorf("REST policy is not defined in the API class for API %s", apiName))
		return nil
	}

	name := makeBackendTrafficPolicyName(rCtx, apiName, exposeLevel)
	httpRouteName := makeHTTPRouteName(&rCtx, apiName, exposeLevel)

	// Get the merged REST policy for the expose level
	mergedPolicy := getMergedRESTPolicy(apiClass.Spec.RESTPolicy, exposeLevel)
	if mergedPolicy == nil {
		// No backend traffic policy defined for this expose level
		return nil
	}

	backendTrafficPolicy := &egv1a1.BackendTrafficPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.envoyproxy.io/v1alpha1",
			Kind:       "BackendTrafficPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeServiceLabels(rCtx),
		},
		Spec: egv1a1.BackendTrafficPolicySpec{
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

	// Configure rate limiting if specified
	if mergedPolicy.RateLimit != nil {
		rateLimit, err := convertToEnvoyGatewayRateLimit(mergedPolicy.RateLimit)
		if err != nil {
			rCtx.AddError(fmt.Errorf("failed to convert rate limit for API %s: %v", apiName, err))
			return nil
		}
		backendTrafficPolicy.Spec.RateLimit = rateLimit
	}

	// Configure circuit breaker if specified
	if mergedPolicy.CircuitBreaker != nil {
		circuitBreaker, err := convertToEnvoyGatewayCircuitBreaker(mergedPolicy.CircuitBreaker)
		if err != nil {
			rCtx.AddError(fmt.Errorf("failed to convert circuit breaker for API %s: %v", apiName, err))
			return nil
		}
		backendTrafficPolicy.Spec.CircuitBreaker = circuitBreaker
	}

	return backendTrafficPolicy
}

// convertToEnvoyGatewayRateLimit converts OpenChoreo RateLimitPolicy to Envoy Gateway RateLimitSpec
func convertToEnvoyGatewayRateLimit(rateLimitPolicy *openchoreov1alpha1.RateLimitPolicy) (*egv1a1.RateLimitSpec, error) {
	if rateLimitPolicy == nil {
		return nil, nil
	}

	// Parse the time duration from the "per" field
	duration, err := time.ParseDuration(rateLimitPolicy.Window)
	if err != nil {
		return nil, fmt.Errorf("invalid duration format: %s", rateLimitPolicy.Window)
	}

	// Convert duration to Envoy Gateway time unit
	var unit egv1a1.RateLimitUnit
	switch {
	case duration <= time.Second:
		unit = egv1a1.RateLimitUnitSecond
	case duration <= time.Minute:
		unit = egv1a1.RateLimitUnitMinute
	case duration <= time.Hour:
		unit = egv1a1.RateLimitUnitHour
	default:
		unit = egv1a1.RateLimitUnitHour
	}

	return &egv1a1.RateLimitSpec{
		Type: egv1a1.LocalRateLimitType,
		Local: &egv1a1.LocalRateLimit{
			Rules: []egv1a1.RateLimitRule{
				{
					Limit: egv1a1.RateLimitValue{
						Requests: uint(rateLimitPolicy.Requests),
						Unit:     unit,
					},
				},
			},
		},
	}, nil
}

// convertToEnvoyGatewayCircuitBreaker converts OpenChoreo CircuitBreakerPolicy to Envoy Gateway CircuitBreaker
func convertToEnvoyGatewayCircuitBreaker(circuitBreakerPolicy *openchoreov1alpha1.CircuitBreakerPolicy) (*egv1a1.CircuitBreaker, error) {
	if circuitBreakerPolicy == nil || !circuitBreakerPolicy.Enabled {
		return nil, nil
	}

	circuitBreaker := &egv1a1.CircuitBreaker{}

	// Set max connections if specified
	if circuitBreakerPolicy.MaxConnections != nil {
		// Convert int32 to int64 as required by Envoy Gateway
		maxConnections := int64(*circuitBreakerPolicy.MaxConnections)
		circuitBreaker.MaxConnections = &maxConnections
	}

	// Set max pending requests if specified
	if circuitBreakerPolicy.MaxPendingRequests != nil {
		// Convert int32 to int64 as required by Envoy Gateway
		maxPendingRequests := int64(*circuitBreakerPolicy.MaxPendingRequests)
		circuitBreaker.MaxPendingRequests = &maxPendingRequests
	}

	// Set max parallel requests if specified
	if circuitBreakerPolicy.MaxParallelRequests != nil {
		// Convert int32 to int64 as required by Envoy Gateway
		maxParallelRequests := int64(*circuitBreakerPolicy.MaxParallelRequests)
		circuitBreaker.MaxParallelRequests = &maxParallelRequests
	}

	// Set max parallel retries if specified
	if circuitBreakerPolicy.MaxParallelRetries != nil {
		// Convert int32 to int64 as required by Envoy Gateway
		maxParallelRetries := int64(*circuitBreakerPolicy.MaxParallelRetries)
		circuitBreaker.MaxParallelRetries = &maxParallelRetries
	}

	return circuitBreaker, nil
}

func makeBackendTrafficPolicyName(rCtx Context, apiName string, exposeLevel openchoreov1alpha1.RESTOperationExposeLevel) string {
	// Create a unique name for the BackendTrafficPolicy using ServiceBinding name, API name and expose level
	exposeLevelStr := strings.ToLower(string(exposeLevel))
	return dpkubernetes.GenerateK8sName(rCtx.ServiceBinding.Name, apiName, exposeLevelStr, "btp")
}

func makeBackendTrafficPolicyResourceID(backendTrafficPolicy *egv1a1.BackendTrafficPolicy) string {
	return backendTrafficPolicy.Name
}
