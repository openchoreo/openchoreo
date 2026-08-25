// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"fmt"
	"sort"
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
				subject:       policy[0],
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

// resourceKey identifies one (subject, path, effect) triple within a single action.
// The subject is part of the key because enforcement resolves allow/deny per
// entitlement value; a deny bound to one entitlement must not cancel an allow
// bound to another.
type resourceKey struct {
	subject string
	path    string
	effect  string
}

// buildCapabilitiesFromPolicies constructs the capabilities map from filtered policies
func (ce *CasbinEnforcer) buildCapabilitiesFromPolicies(policies []policyInfo, actionIdx actionIndex) (map[string]*authzcore.ActionCapability, error) {
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
			key := resourceKey{subject: p.subject, path: p.resourcePath, effect: p.effect}

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

	// Convert to final capabilities structure, resolving allow/deny per entitlement
	capabilities := make(map[string]*authzcore.ActionCapability, len(actionExpressions))
	for action, resources := range actionExpressions {
		capabilities[action] = resolveCapability(resources)
	}

	return capabilities, nil
}

// capEntry is a single (path, constraints) pair for one action.
// An empty expressions slice means the grant or denial is unconditional.
type capEntry struct {
	path        string
	expressions []string
}

func (e capEntry) unconditional() bool { return len(e.expressions) == 0 }

// subjectPolicies holds one entitlement's allow and deny entries for a single action.
type subjectPolicies struct {
	allows []capEntry
	denies []capEntry
}

// resolveCapability turns the accumulated (subject, path, effect) entries for one action
// into the reported capability, mirroring how enforcement resolves a request: allow and
// deny are resolved within a single entitlement value, and the per-entitlement results
// are unioned. A path therefore stays in Denied only while no entitlement grants it.
//
// The two lists are meant to be read as: a resource is allowed when some allowed entry
// applies to it (its path covers the resource and its constraints hold) and no denied
// entry that applies to it sits at least as deep in the hierarchy as that allowed entry.
// A denial therefore carves out of a broader grant while a narrower grant survives a
// broader denial, which is the only faithful flat encoding of the per-entitlement result:
// given "s1: allow ns/a, deny ns/a/p1" and "s2: allow ns/a/p1/c1", enforcement grants
// ns/a/p1/c1 but not ns/a/p1/c2, so dropping the deny would report c2 as allowed while
// honoring it globally would report c1 as denied.
//
// Known gap: a deny survives when the only entitlement granting past it does so
// conditionally, because "granted while the expression holds, denied otherwise" cannot be
// expressed for a single path. That direction fails closed, matching the rest of the PDP.
func resolveCapability(resources map[resourceKey][]string) *authzcore.ActionCapability {
	bySubject := groupBySubject(resources)

	// A deny cancels an allow only when both are bound to the same entitlement.
	// A conditional deny never cancels outright: it carves out the resources its CEL
	// expression matches and stays in Denied to describe that carve-out.
	survivingAllows := make(map[string][]capEntry, len(bySubject))
	for subject, sp := range bySubject {
		for _, allow := range sp.allows {
			if coveredByUnconditional(allow.path, sp.denies) {
				continue
			}
			survivingAllows[subject] = append(survivingAllows[subject], allow)
		}
	}

	allows := make([]capEntry, 0, len(resources))
	denies := make([]capEntry, 0, len(resources))
	for _, subject := range sortedSubjects(bySubject) {
		allows = append(allows, survivingAllows[subject]...)
		for _, deny := range bySubject[subject].denies {
			// Another entitlement unconditionally grants everything this deny covers,
			// so those resources are reachable and must not be reported as denied.
			if neutralizedByOtherSubject(deny.path, subject, survivingAllows) {
				continue
			}
			denies = append(denies, deny)
		}
	}

	return &authzcore.ActionCapability{
		Allowed: mergeByPath(allows),
		Denied:  mergeByPath(denies),
	}
}

// groupBySubject splits the accumulated entries for one action into per-entitlement
// allow and deny lists. Entries are visited in a stable order so the reported
// capability does not depend on map iteration order.
func groupBySubject(resources map[resourceKey][]string) map[string]*subjectPolicies {
	keys := make([]resourceKey, 0, len(resources))
	for key := range resources {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].subject != keys[j].subject {
			return keys[i].subject < keys[j].subject
		}
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].effect < keys[j].effect
	})

	bySubject := make(map[string]*subjectPolicies)
	for _, key := range keys {
		sp, ok := bySubject[key.subject]
		if !ok {
			sp = &subjectPolicies{}
			bySubject[key.subject] = sp
		}
		entry := capEntry{path: key.path, expressions: resources[key]}
		switch key.effect {
		case string(authzcore.PolicyEffectAllow):
			sp.allows = append(sp.allows, entry)
		case string(authzcore.PolicyEffectDeny):
			sp.denies = append(sp.denies, entry)
		}
	}
	return bySubject
}

func sortedSubjects(bySubject map[string]*subjectPolicies) []string {
	subjects := make([]string, 0, len(bySubject))
	for subject := range bySubject {
		subjects = append(subjects, subject)
	}
	sort.Strings(subjects)
	return subjects
}

// coveredByUnconditional reports whether an unconditional entry covers path.
// Coverage uses the same hierarchical matcher as enforcement, so a wildcard or
// ancestor path covers everything beneath it.
func coveredByUnconditional(path string, entries []capEntry) bool {
	for _, entry := range entries {
		if entry.unconditional() && resourceMatch(path, entry.path) {
			return true
		}
	}
	return false
}

// neutralizedByOtherSubject reports whether an entitlement other than owner
// unconditionally grants every resource the given deny path covers.
func neutralizedByOtherSubject(path, owner string, allowsBySubject map[string][]capEntry) bool {
	for subject, allows := range allowsBySubject {
		if subject == owner {
			continue
		}
		if coveredByUnconditional(path, allows) {
			return true
		}
	}
	return false
}

// mergedEntry accumulates the constraints reported for one path.
// unconditional is tracked separately from expressions because "no expressions"
// is only meaningful once at least one entry has been merged in.
type mergedEntry struct {
	unconditional bool
	expressions   []string
	seen          map[string]bool
}

// mergeByPath collapses entries sharing a path into one reported resource.
// An unconditional entry wins over conditional ones — the path is reachable without
// satisfying any expression; otherwise the distinct expressions are OR'd together.
func mergeByPath(entries []capEntry) []*authzcore.CapabilityResource {
	order := make([]string, 0, len(entries))
	byPath := make(map[string]*mergedEntry, len(entries))

	for _, entry := range entries {
		merged, ok := byPath[entry.path]
		if !ok {
			merged = &mergedEntry{seen: make(map[string]bool)}
			byPath[entry.path] = merged
			order = append(order, entry.path)
		}
		if merged.unconditional {
			continue
		}
		if entry.unconditional() {
			merged.unconditional = true
			merged.expressions = nil
			continue
		}
		for _, expression := range entry.expressions {
			if merged.seen[expression] {
				continue
			}
			merged.seen[expression] = true
			merged.expressions = append(merged.expressions, expression)
		}
	}

	resources := make([]*authzcore.CapabilityResource, 0, len(order))
	for _, path := range order {
		merged := byPath[path]
		var constraints *authzcore.Constraints
		if len(merged.expressions) > 0 {
			constraints = &authzcore.Constraints{Expressions: merged.expressions}
		}
		resources = append(resources, &authzcore.CapabilityResource{
			Path:        path,
			Constraints: constraints,
		})
	}
	return resources
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
