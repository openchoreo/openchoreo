// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"fmt"
	"time"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// ============================================================================
// PDP Implementation
// ============================================================================

// Evaluate evaluates a single authorization request and returns a decision
func (ce *CasbinEnforcer) Evaluate(ctx context.Context, request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	err := validateEvaluateRequest(request)
	if err != nil {
		return &authzcore.Decision{Decision: false}, err
	}
	return ce.check(request)
}

// BatchEvaluate evaluates multiple authorization requests and returns corresponding decisions
// NOTE: if needed, can be enhanced to do in parallel
func (ce *CasbinEnforcer) BatchEvaluate(ctx context.Context, request *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	err := validateBatchEvaluateRequest(request)
	if err != nil {
		return &authzcore.BatchEvaluateResponse{}, err
	}

	decisions := make([]authzcore.Decision, len(request.Requests))
	for i, req := range request.Requests {
		// Check for context cancellation
		if ctx.Err() != nil {
			return &authzcore.BatchEvaluateResponse{}, ctx.Err()
		}
		decision, err := ce.check(&req)
		if err != nil {
			return &authzcore.BatchEvaluateResponse{}, fmt.Errorf("batch evaluate failed at index %d: %w", i, err)
		}
		decisions[i] = *decision
	}

	return &authzcore.BatchEvaluateResponse{
		Decisions: decisions,
	}, nil
}

// GetSubjectProfile retrieves the authorization profile for a given subject
func (ce *CasbinEnforcer) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	ce.logger.Debug("get subject profile called",
		"subject_context", request.SubjectContext,
		"scope", request.Scope)

	if err := validateProfileRequest(request); err != nil {
		return nil, err
	}

	subjectCtx := request.SubjectContext
	scopePath := resourceHierarchyToPath(request.Scope)

	allConcreteActions := authzcore.ConcretePublicActions()

	actionIndex := indexActions(allConcreteActions)

	policies, err := ce.filterPoliciesBySubjectAndScope(subjectCtx, scopePath)
	if err != nil {
		return nil, err
	}

	capabilities, err := ce.buildCapabilitiesFromPolicies(policies, actionIndex)
	if err != nil {
		return nil, err
	}

	return &authzcore.UserCapabilitiesResponse{
		User:         subjectCtx,
		Capabilities: capabilities,
		GeneratedAt:  time.Now(),
	}, nil
}

// filterPoliciesBySubjectAndScope retrieves and filters policies relevant to the subject and scope
func (ce *CasbinEnforcer) filterPoliciesBySubjectAndScope(subjectCtx *authzcore.SubjectContext, scopePath string) ([]policyInfo, error) {
	var filteredPolicies []policyInfo

	for _, entitlementValue := range subjectCtx.EntitlementValues {
		subject, err := formatSubject(subjectCtx.EntitlementClaim, entitlementValue)
		if err != nil {
			return nil, fmt.Errorf("failed to format subject: %w", err)
		}
		policies, err := ce.enforcer.GetFilteredPolicy(0, subject)
		if err != nil {
			return nil, fmt.Errorf("failed to get policies for subject '%s': %w", subject, err)
		}

		for _, policy := range policies {
			if len(policy) != 7 {
				ce.logger.Warn("skipping malformed policy", "policy", policy, "expected", 7, "got", len(policy))
				continue
			}

			resourcePath := policy[1]
			roleName := policy[2]
			roleNamespace := policy[3]
			effect := policy[4]
			conditions := policy[5]

			if !isWithinScope(resourcePath, scopePath) {
				continue
			}

			filteredPolicies = append(filteredPolicies, policyInfo{
				resourcePath:  resourcePath,
				roleName:      roleName,
				roleNamespace: roleNamespace,
				effect:        effect,
				conditions:    conditions,
			})
		}
	}

	return filteredPolicies, nil
}

// buildCapabilitiesFromPolicies constructs the capabilities map from filtered policies
func (ce *CasbinEnforcer) buildCapabilitiesFromPolicies(policies []policyInfo, actionIdx actionIndex) (map[string]*authzcore.ActionCapability, error) {
	type resourceKey struct {
		path   string
		effect string
	}

	roleToActions := make(map[authzcore.RoleRef][]string)
	// actionExpressions accumulates CEL expressions per (action, resourceKey).
	// nil slice means no conditions (unconditional);
	actionExpressions := make(map[string]map[resourceKey][]string)
	// unconditionalKeys tracks which (action, resourceKey) pairs have been marked
	// unconditional so that later conditional bindings cannot re-add constraints.
	unconditionalKeys := make(map[string]map[resourceKey]bool)

	for _, p := range policies {
		roleKey := authzcore.RoleRef{
			Name:      p.roleName,
			Namespace: p.roleNamespace,
		}

		// Fetch role actions if not already cached
		if _, ok := roleToActions[roleKey]; !ok {
			roleActions, err := ce.enforcer.GetFilteredGroupingPolicy(0, p.roleName, "", p.roleNamespace)
			if err != nil {
				return nil, fmt.Errorf("failed to get actions for role '%s' in namespace '%s': %w", p.roleName, p.roleNamespace, err)
			}

			var actions []string
			for _, ra := range roleActions {
				if len(ra) != 3 {
					ce.logger.Warn("skipping malformed role-action mapping", "rule", ra)
					continue
				}
				expandedActions := expandActionWildcard(ra[1], actionIdx)
				actions = append(actions, expandedActions...)
			}
			roleToActions[roleKey] = actions
		}

		// Decode conditions from this policy, filtered per action below
		allConds, err := decodeConditions(p.conditions)
		if err != nil {
			return nil, fmt.Errorf("policy conditions corrupt for role '%s': %w", p.roleName, err)
		}

		actions := roleToActions[roleKey]
		for _, action := range actions {
			if actionExpressions[action] == nil {
				actionExpressions[action] = make(map[resourceKey][]string)
				unconditionalKeys[action] = make(map[resourceKey]bool)
			}
			key := resourceKey{path: p.resourcePath, effect: p.effect}

			// Filter conditions to only those targeting this action
			actionConds := filterConditionsByAction(allConds, action)
			if len(actionConds) > 0 {
				// An unconditional binding already claimed this key; skip.
				if unconditionalKeys[action][key] {
					continue
				}
				for _, c := range actionConds {
					actionExpressions[action][key] = append(actionExpressions[action][key], c.Expression)
				}
				continue
			}
			// No conditions target this action — unconditional for this action.
			// Discard any previously accumulated conditional expressions and mark unconditional.
			unconditionalKeys[action][key] = true
			actionExpressions[action][key] = nil
		}
	}

	// Convert to final capabilities structure
	capabilities := make(map[string]*authzcore.ActionCapability)
	for action, resources := range actionExpressions {
		capability := &authzcore.ActionCapability{
			Allowed: []*authzcore.CapabilityResource{},
			Denied:  []*authzcore.CapabilityResource{},
		}

		for res, expressions := range resources {
			var constraints *authzcore.Constraints
			if len(expressions) > 0 {
				constraints = &authzcore.Constraints{Expressions: expressions}
			}
			capRes := &authzcore.CapabilityResource{
				Path:        res.path,
				Constraints: constraints,
			}
			if res.effect == string(authzcore.PolicyEffectAllow) {
				capability.Allowed = append(capability.Allowed, capRes)
			} else if res.effect == string(authzcore.PolicyEffectDeny) {
				capability.Denied = append(capability.Denied, capRes)
			}
		}

		capabilities[action] = capability
	}

	return capabilities, nil
}

// check performs the actual authorization check using Casbin
func (ce *CasbinEnforcer) check(request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	resourcePath := resourceHierarchyToPath(request.Resource.Hierarchy)
	subjectCtx := request.SubjectContext

	// Validate subject context
	if subjectCtx == nil {
		return &authzcore.Decision{Decision: false}, fmt.Errorf("subject context is required")
	}

	ce.logger.Debug("evaluate called",
		"subject_type", subjectCtx.Type,
		"entitlement_claim", subjectCtx.EntitlementClaim,
		"entitlements", subjectCtx.EntitlementValues,
		"resource", resourcePath,
		"action", request.Action,
		"context", request.Context)

	// Serialize the request context once; it is invariant across entitlements.
	ctxJSON, err := serializeAuthzContext(request.Context)
	if err != nil {
		return &authzcore.Decision{Decision: false}, fmt.Errorf("failed to serialize request context: %w", err)
	}

	result := false
	decision := &authzcore.Decision{Decision: false,
		Context: &authzcore.DecisionContext{
			Reason: "no matching policies found",
		}}
	for _, entitlementValue := range subjectCtx.EntitlementValues {
		entitlement, err := formatSubject(subjectCtx.EntitlementClaim, entitlementValue)
		if err != nil {
			ce.logger.Warn("failed to format subject", "error", err)
			return &authzcore.Decision{Decision: false}, fmt.Errorf("failed to format subject: %w", err)
		}
		result, err = ce.enforcer.Enforce(
			entitlement,
			resourcePath,
			request.Action,
			ctxJSON,
		)
		if err != nil {
			ce.logger.Warn("enforcement failed", "error", err)
			return &authzcore.Decision{Decision: false}, fmt.Errorf("enforcement failed: %w", err)
		}
		if result {
			decision.Decision = true
			resourceInfo := fmt.Sprintf("hierarchy '%s'", resourcePath)
			if request.Resource.ID != "" {
				resourceInfo = fmt.Sprintf("%s (id: %s)", resourceInfo, request.Resource.ID)
			}
			decision.Context.Reason = fmt.Sprintf("Access granted: entitlement value '%s' authorized to perform '%s' on %s", entitlementValue, request.Action, resourceInfo)
			break
		}
	}
	return decision, nil
}
