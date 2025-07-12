// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Context holds all the information needed for rendering EndpointV2 resources
type Context struct {
	EndpointV2    *openchoreov1alpha1.EndpointV2
	EndpointClass *openchoreov1alpha1.EndpointClass

	// ResolvedPolicies contains the merged policies from EndpointClass and EndpointV2
	ResolvedPolicies *ResolvedPolicies

	// Stores the errors encountered during rendering.
	errs []error
}

// ResolvedPolicies represents the final resolved API management policies
type ResolvedPolicies struct {
	REST *RESTResolutionResult
	// Future: gRPC *gRPCResolutionResult, TCP *TCPResolutionResult, etc.
}

// RESTResolutionResult contains the resolved REST endpoint policies
type RESTResolutionResult struct {
	// Per-operation policies organized by expose level
	Operations map[string]*RESTOperationPolicies // key: method:path

	// Default policies that apply to all operations
	DefaultPolicies *openchoreov1alpha1.RESTPolicy
}

// RESTOperationPolicies contains the resolved policies for a specific REST operation
type RESTOperationPolicies struct {
	Operation    *openchoreov1alpha1.RESTEndpointOperation
	ExposeLevels map[openchoreov1alpha1.RESTOperationExposeLevel]*openchoreov1alpha1.RESTPolicy
}

func (c *Context) AddError(err error) {
	if err != nil {
		c.errs = append(c.errs, err)
	}
}

func (c *Context) Errors() []error {
	if len(c.errs) == 0 {
		return nil
	}
	return c.errs
}

func (c *Context) Error() error {
	if len(c.errs) > 0 {
		return utilerrors.NewAggregate(c.errs)
	}
	return nil
}

// EnsurePolicyResolution resolves policies from EndpointClass and EndpointV2
func (c *Context) EnsurePolicyResolution() {
	if c.ResolvedPolicies != nil {
		return // Already resolved
	}

	c.ResolvedPolicies = &ResolvedPolicies{}

	// Resolve REST policies if this is a REST endpoint
	if c.EndpointV2.Spec.Type == openchoreov1alpha1.EndpointTypeREST && c.EndpointV2.Spec.RESTEndpoint != nil {
		c.ResolvedPolicies.REST = c.resolveRESTPolicy()
	}
}

// resolveRESTPolicy resolves REST-specific policies
func (c *Context) resolveRESTPolicy() *RESTResolutionResult {
	result := &RESTResolutionResult{
		Operations: make(map[string]*RESTOperationPolicies),
	}

	// Get default policies from EndpointClass
	// if c.EndpointClass.Spec.RESTPolicy != nil && c.EndpointClass.Spec.RESTPolicy.Defaults != nil {
	//	result.DefaultPolicies = &c.EndpointClass.Spec.RESTPolicy.Defaults.RESTPolicy
	//}

	// Process each operation in the EndpointV2
	for _, operation := range c.EndpointV2.Spec.RESTEndpoint.Operations {
		opKey := string(operation.Method) + ":" + operation.Path
		opPolicies := &RESTOperationPolicies{
			Operation:    &operation,
			ExposeLevels: make(map[openchoreov1alpha1.RESTOperationExposeLevel]*openchoreov1alpha1.RESTPolicy),
		}

		// Apply policies for each expose level
		for _, exposeLevel := range operation.ExposeLevels {
			policy := c.resolveRESTOperationPolicy(operation, exposeLevel)
			opPolicies.ExposeLevels[exposeLevel] = policy
		}

		result.Operations[opKey] = opPolicies
	}

	return result
}

// resolveRESTOperationPolicy resolves the final policy for a specific operation and expose level
func (c *Context) resolveRESTOperationPolicy(operation openchoreov1alpha1.RESTEndpointOperation, exposeLevel openchoreov1alpha1.RESTOperationExposeLevel) *openchoreov1alpha1.RESTPolicy {
	// Start with default policy
	var finalPolicy *openchoreov1alpha1.RESTPolicy
	//if c.EndpointClass.Spec.RESTPolicy != nil && c.EndpointClass.Spec.RESTPolicy.Defaults != nil {
	//	finalPolicy = c.EndpointClass.Spec.RESTPolicy.Defaults.RESTPolicy.DeepCopy()
	//} else {
	//	finalPolicy = &openchoreov1alpha1.RESTPolicy{}
	//}
	//
	//// Apply expose level overrides from EndpointClass
	//if c.EndpointClass.Spec.RESTPolicy != nil {
	//	switch exposeLevel {
	//	case openchoreov1alpha1.ExposeLevelOrganization:
	//		if c.EndpointClass.Spec.RESTPolicy.Organization != nil {
	//			finalPolicy = c.mergeRESTPolicy(finalPolicy, &c.EndpointClass.Spec.RESTPolicy.Organization.RESTPolicy)
	//		}
	//	case openchoreov1alpha1.ExposeLevelPublic:
	//		if c.EndpointClass.Spec.RESTPolicy.Public != nil {
	//			finalPolicy = c.mergeRESTPolicy(finalPolicy, &c.EndpointClass.Spec.RESTPolicy.Public.RESTPolicy)
	//		}
	//	}
	//
	//	// Apply conditional policies from EndpointClass based on expose level
	//	var conditionalPolicies []openchoreov1alpha1.RESTConditionalPolicy
	//	switch exposeLevel {
	//	case openchoreov1alpha1.ExposeLevelOrganization:
	//		if c.EndpointClass.Spec.RESTPolicy.Organization != nil {
	//			conditionalPolicies = c.EndpointClass.Spec.RESTPolicy.Organization.ConditionalPolicies
	//		}
	//	case openchoreov1alpha1.ExposeLevelPublic:
	//		if c.EndpointClass.Spec.RESTPolicy.Public != nil {
	//			conditionalPolicies = c.EndpointClass.Spec.RESTPolicy.Public.ConditionalPolicies
	//		}
	//	default:
	//		if c.EndpointClass.Spec.RESTPolicy.Defaults != nil {
	//			conditionalPolicies = c.EndpointClass.Spec.RESTPolicy.Defaults.ConditionalPolicies
	//		}
	//	}
	//
	//	for _, conditionalPolicy := range conditionalPolicies {
	//		if c.matchesCondition(operation, conditionalPolicy.Condition) {
	//			finalPolicy = c.mergeRESTPolicy(finalPolicy, conditionalPolicy.Policy)
	//		}
	//	}
	//}

	// TODO: Apply EndpointV2-specific policy overrides if they exist in the API

	return finalPolicy
}

// mergeRESTPolicy merges two REST policies, with override taking precedence
func (c *Context) mergeRESTPolicy(base, override *openchoreov1alpha1.RESTPolicy) *openchoreov1alpha1.RESTPolicy {
	if base == nil {
		return override.DeepCopy()
	}
	if override == nil {
		return base.DeepCopy()
	}

	merged := base.DeepCopy()

	// Merge rate limiting
	if override.RateLimit != nil {
		merged.RateLimit = override.RateLimit
	}

	// Merge authentication
	if override.Authentication != nil {
		merged.Authentication = override.Authentication
	}

	// Merge CORS
	if override.CORS != nil {
		merged.CORS = override.CORS
	}

	// Merge security
	if override.Security != nil {
		merged.Security = override.Security
	}

	// Merge mediation
	if override.Mediation != nil {
		merged.Mediation = override.Mediation
	}

	// Merge retries
	if override.Retries != nil {
		merged.Retries = override.Retries
	}

	// Merge circuit breaker
	if override.CircuitBreaker != nil {
		merged.CircuitBreaker = override.CircuitBreaker
	}

	// Merge monitoring
	if override.Monitoring != nil {
		merged.Monitoring = override.Monitoring
	}

	return merged
}

// matchesCondition checks if an operation matches a conditional policy condition
func (c *Context) matchesCondition(operation openchoreov1alpha1.RESTEndpointOperation, condition *openchoreov1alpha1.RESTPolicyCondition) bool {
	if condition == nil {
		return true
	}

	// Check method match
	if condition.Method != nil {
		if *condition.Method != operation.Method {
			return false
		}
	}

	// Check path pattern match
	if len(condition.Paths) > 0 {
		pathMatches := false
		for _, pattern := range condition.Paths {
			// Simple prefix matching - could be enhanced with regex
			if len(operation.Path) >= len(pattern) && operation.Path[:len(pattern)] == pattern {
				pathMatches = true
				break
			}
		}
		if !pathMatches {
			return false
		}
	}

	return true
}
