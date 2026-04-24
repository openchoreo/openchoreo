// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/cel-go/interpreter"

	authzv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// HierarchyResourcePrefix constants for resource path formatting
const (
	NamespaceResourcePrefix = "ns"
	ProjectResourcePrefix   = "project"
	ComponentResourcePrefix = "component"
)

// CRD type constants for authz resources
const (
	CRDTypeAuthzRole               = "AuthzRole"
	CRDTypeClusterAuthzRole        = "ClusterAuthzRole"
	CRDTypeAuthzRoleBinding        = "AuthzRoleBinding"
	CRDTypeClusterAuthzRoleBinding = "ClusterAuthzRoleBinding"
)

const (
	// emptyContextJSON represents an empty context used when no contextual conditions are applied
	emptyContextJSON = "{}"
)

// serializeAuthzContext serializes an authzcore.Context to JSON for passing as a Casbin enforce arg.
func serializeAuthzContext(ctx authzcore.Context) (string, error) {
	b, err := json.Marshal(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to serialize context: %w", err)
	}
	return string(b), nil
}

// serializeAuthzConditions serializes a slice of AuthzCondition to JSON for storing in the policy.
// Returns a empty context JSON if the slice is empty.
func serializeAuthzConditions(conds []authzv1alpha1.AuthzCondition) (string, error) {
	if len(conds) == 0 {
		return emptyContextJSON, nil
	}
	b, err := json.Marshal(conds)
	if err != nil {
		return "", fmt.Errorf("failed to serialize conditions: %w", err)
	}
	return string(b), nil
}

// resourceMatch checks if a requested resource matches a policy resource using hierarchical prefix matching.
// For example, policy "namespace/acme" matches request "namespace/acme/project/p1/component/c1"
// This allows policies to apply to all resources under a hierarchical scope.
func resourceMatch(requestResource, policyResource string) bool {
	// Full wildcard matches any resource
	if policyResource == "*" {
		return true
	}
	// Exact match
	if requestResource == policyResource {
		return true
	}

	// Hierarchical prefix match: policy resource is a prefix of the requested resource
	// e.g., policy "namespace/acme" matches request "namespace/acme/project/p1"
	return strings.HasPrefix(requestResource, policyResource+"/")
}

// ConditionMatcher evaluates the per-binding ABAC conditions against the request.
func ConditionMatcher(requestCtxJSON, requestAction, policyCond string) bool {
	if isPolicyConditionEmpty(policyCond) {
		return true
	}

	var conditions []authzv1alpha1.AuthzCondition
	if err := json.Unmarshal([]byte(policyCond), &conditions); err != nil {
		slog.Default().Error("ConditionMatcher: failed to unmarshal policy conditions", "error", err)
		return false
	}

	matching := filterConditionsByAction(conditions, requestAction)
	// If no condition entries match the action, RBAC decision stands
	if len(matching) == 0 {
		return true
	}

	activation, ok := buildActivationForRequest(requestCtxJSON, requestAction)
	if !ok {
		return false
	}

	return anyConditionAllows(matching, activation)
}

// isPolicyConditionEmpty reports whether the stored policy condition carries no constraints.
func isPolicyConditionEmpty(policyCond string) bool {
	return policyCond == "" || policyCond == emptyContextJSON
}

// filterConditionsByAction returns conditions whose Actions include a pattern matching requestAction.
func filterConditionsByAction(conds []authzv1alpha1.AuthzCondition, requestAction string) []authzv1alpha1.AuthzCondition {
	var matching []authzv1alpha1.AuthzCondition
	for _, c := range conds {
		for _, pattern := range c.Actions {
			if actionMatch(requestAction, pattern) {
				matching = append(matching, c)
				break
			}
		}
	}
	return matching
}

// buildActivationForRequest parses the request context JSON and builds the CEL
// activation gated by the registry-allowed attributes for requestAction.
func buildActivationForRequest(requestCtxJSON, requestAction string) (interpreter.Activation, bool) {
	var authzCtx authzcore.Context
	if err := json.Unmarshal([]byte(requestCtxJSON), &authzCtx); err != nil {
		slog.Default().Error("condMatch: failed to parse request context", "error", err)
		return nil, false
	}

	activation, err := buildCelActivation(authzCtx, authzcore.LookupConditions(requestAction))
	if err != nil {
		slog.Default().Error("condMatch: failed to build CEL activation", "error", err)
		return nil, false
	}
	return activation, true
}

// anyConditionAllows returns true if any entry's CEL expression evaluates to true.
func anyConditionAllows(entries []authzv1alpha1.AuthzCondition, activation interpreter.Activation) bool {
	for _, entry := range entries {
		if evalCondition(entry.Expression, activation) {
			return true
		}
	}
	return false
}

// evalCondition compiles and evaluates a single CEL expression, returning true only
// on a boolean-true result. Compile and eval errors are logged and treated as false.
func evalCondition(expr string, activation interpreter.Activation) bool {
	prg, err := compileCEL(expr)
	if err != nil {
		slog.Default().Error("condMatch: CEL compile error", "expression", expr, "error", err)
		return false
	}
	out, _, err := prg.Eval(activation)
	if err != nil {
		slog.Default().Error("condMatch: CEL eval error", "expression", expr, "error", err)
		return false
	}
	b, ok := out.Value().(bool)
	slog.Default().Debug("condMatch: CEL eval result", "expression", expr, "result", out.Value(), "bool", b, "ok", ok)
	return ok && b
}

// resourceMatchWrapper is a wrapper for resourceMatch to work with Casbin's function interface
func resourceMatchWrapper(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return false, fmt.Errorf("resourceMatch requires exactly 2 arguments")
	}

	requestResource, ok := args[0].(string)
	if !ok {
		return false, fmt.Errorf("first argument must be a string")
	}

	policyResource, ok := args[1].(string)
	if !ok {
		return false, fmt.Errorf("second argument must be a string")
	}

	return resourceMatch(requestResource, policyResource), nil
}

// condMatchWrapper is a wrapper for condMatch to work with Casbin's function interface
func condMatchWrapper(args ...interface{}) (interface{}, error) {
	if len(args) != 3 {
		return false, fmt.Errorf("condMatch requires exactly 3 arguments")
	}

	requestCtx, ok := args[0].(string)
	if !ok {
		return false, fmt.Errorf("request context argument must be a string")
	}

	requestAction, ok := args[1].(string)
	if !ok {
		return false, fmt.Errorf("request action argument must be a string")
	}

	policyConds, ok := args[2].(string)
	if !ok {
		return false, fmt.Errorf("policy conditions argument must be a string")
	}

	return ConditionMatcher(requestCtx, requestAction, policyConds), nil
}

// actionMatch checks if a requested action matches a role's action pattern with wildcard support.
// Supports:
// - Exact match: "component:read" matches "component:read"
// - Verb wildcard: "component:*" matches "component:read", "component:write", etc.
// - Full wildcard: "*" matches any action
func actionMatch(requestAction, roleAction string) bool {
	// Full wildcard matches any action
	if roleAction == "*" {
		return true
	}

	if roleAction == requestAction {
		return true
	}
	// Verb wildcard match: "component:*" matches "component:read", "component:write", etc.
	if strings.HasSuffix(roleAction, ":*") {
		prefixLen := len(roleAction) - 1
		return len(requestAction) > prefixLen && requestAction[:prefixLen] == roleAction[:prefixLen]
	}
	return false
}

func roleActionMatchWrapper(requestValue, storedRuleValue string) bool {
	// If storedRuleValue looks like an action (contains ":" or is a wildcard "*"),
	// use action matching with wildcard support
	if strings.Contains(storedRuleValue, ":") || storedRuleValue == "*" {
		return actionMatch(requestValue, storedRuleValue)
	}
	// Otherwise, it's a role name or namespace - use exact matching
	return requestValue == storedRuleValue
}

// validateEvaluateRequest checks if the EvaluateRequest has all required fields
func validateEvaluateRequest(req *authzcore.EvaluateRequest) error {
	if req == nil {
		return fmt.Errorf("%w: evaluate request is nil", authzcore.ErrInvalidRequest)
	}
	if req.SubjectContext == nil {
		return fmt.Errorf("%w: subject context is required", authzcore.ErrInvalidRequest)
	}
	if req.Resource.Type == "" {
		return fmt.Errorf("%w: resource type is required", authzcore.ErrInvalidRequest)
	}
	if req.Action == "" {
		return fmt.Errorf("%w: action is required", authzcore.ErrInvalidRequest)
	}
	return nil
}

// resourceHierarchyToPath converts ResourceHierarchy to a hierarchical resource path string
// Examples:
//   - {Namespace: "acme"} -> "ns/acme"
//   - {Namespace: "acme", Project: "p1"} -> "ns/acme/project/p1"
//   - {Namespace: "acme", Project: "p1", Component: "c1"} -> "ns/acme/project/p1/component/c1"
//   - {} (empty) -> "*" (wildcard)
func resourceHierarchyToPath(hierarchy authzcore.ResourceHierarchy) string {
	// Empty hierarchy means global wildcard
	if hierarchy.Namespace == "" && hierarchy.Project == "" && hierarchy.Component == "" {
		return "*"
	}

	path := ""

	if hierarchy.Namespace != "" {
		path = fmt.Sprintf("%s/%s", NamespaceResourcePrefix, hierarchy.Namespace)
	}

	if hierarchy.Project != "" {
		path = fmt.Sprintf("%s/%s/%s", path, ProjectResourcePrefix, hierarchy.Project)
	}

	if hierarchy.Component != "" {
		path = fmt.Sprintf("%s/%s/%s", path, ComponentResourcePrefix, hierarchy.Component)
	}

	path = strings.Trim(path, "/")

	return path
}

// resourcePathToHierarchy converts a hierarchical resource path string back to ResourceHierarchy
// Examples:
//   - "ns/acme" -> {Namespace: "acme"}
//   - "ns/acme/project/p1" -> {Namespace: "acme", Project: "p1"}
//   - "ns/acme/project/p1/component/c1" -> {Namespace: "acme", Project: "p1", Component: "c1"}
//   - "*" -> {} (empty hierarchy)
func resourcePathToHierarchy(resourcePath string) authzcore.ResourceHierarchy {
	hierarchy := authzcore.ResourceHierarchy{}

	// Global wildcard maps to empty hierarchy
	if resourcePath == "*" || resourcePath == "" {
		return hierarchy
	}

	segments := strings.Split(resourcePath, "/")

	for i := 0; i < len(segments)-1; i += 2 {
		prefix := segments[i]
		value := segments[i+1]

		switch prefix {
		case NamespaceResourcePrefix:
			hierarchy.Namespace = value
		case ProjectResourcePrefix:
			hierarchy.Project = value
		case ComponentResourcePrefix:
			hierarchy.Component = value
		}
	}

	return hierarchy
}

// formatSubject creates a subject string from claim and value
// Format: "claim:value"
func formatSubject(claim, value string) (string, error) {
	if claim == "" || value == "" {
		return "", fmt.Errorf("claim and value cannot be empty")
	}
	return fmt.Sprintf("%s:%s", claim, value), nil
}

// parseSubject extracts claim and value from a subject string
// Expected format: "claim:value"
func parseSubject(subject string) (claim, value string, err error) {
	parts := strings.SplitN(subject, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid subject format: expected 'claim:value', got '%s'", subject)
	}
	return parts[0], parts[1], nil
}

// validateBatchEvaluateRequest checks if each EvaluateRequest in the BatchEvaluateRequest has all required fields
func validateBatchEvaluateRequest(req *authzcore.BatchEvaluateRequest) error {
	if req == nil {
		return fmt.Errorf("%w: batch evaluate request is nil", authzcore.ErrInvalidRequest)
	}
	if len(req.Requests) == 0 {
		return fmt.Errorf("%w: batch evaluate request contains no requests", authzcore.ErrInvalidRequest)
	}
	for i, req := range req.Requests {
		if req.SubjectContext == nil {
			return fmt.Errorf("%w: subject context is required at index %d", authzcore.ErrInvalidRequest, i)
		}
		if req.Resource.Type == "" {
			return fmt.Errorf("%w: resource type is required at index %d", authzcore.ErrInvalidRequest, i)
		}
		if req.Action == "" {
			return fmt.Errorf("%w: action is required at index %d", authzcore.ErrInvalidRequest, i)
		}
	}
	return nil
}

type actionIndex struct {
	ByResourceType    map[string][]string // "component" -> ["component:read", ...]
	actionsStringList []string
}

func indexActions(allActions []authzcore.Action) actionIndex {
	index := actionIndex{
		ByResourceType:    make(map[string][]string),
		actionsStringList: make([]string, 0, len(allActions)),
	}

	for _, action := range allActions {
		resourceType := extractActionResourceType(action.Name)
		index.ByResourceType[resourceType] = append(index.ByResourceType[resourceType], action.Name)
		index.actionsStringList = append(index.actionsStringList, action.Name)
	}

	return index
}

// extractActionResource extracts the resource part from an action string.
func extractActionResourceType(action string) string {
	colonIdx := strings.LastIndex(action, ":")
	if colonIdx > 0 {
		return action[:colonIdx]
	}
	return action
}

// isWithinScope checks if a policy resource is relevant within the requested scope.
func isWithinScope(policyResource, scopePath string) bool {
	// Wildcard policy matches any scope
	if policyResource == "*" || scopePath == "*" {
		return true
	}

	// Exact match
	if policyResource == scopePath {
		return true
	}

	// Policy is broader (parent) - grants permissions that include the scope
	// e.g., policy "namespace/acme" includes scope "namespace/acme/project/p1"
	if strings.HasPrefix(scopePath, policyResource+"/") {
		return true
	}

	// Policy is narrower (child) - grants permissions within the scope
	// e.g., scope "namespace/acme" includes policy "namespace/acme/project/p1"
	if strings.HasPrefix(policyResource, scopePath+"/") {
		return true
	}

	return false
}

// expandActionWildcard expands a potentially wildcarded action to all matching concrete actions.
// Uses a pre-built map for O(1) lookups instead of O(A) iteration.
func expandActionWildcard(actionPattern string, actionIndex actionIndex) []string {
	// Full wildcard matches all actions
	if actionPattern == "*" {
		return actionIndex.actionsStringList
	}
	actionsByResource := actionIndex.ByResourceType

	// Verb wildcard: "component:*" -> lookup "component:" in map
	if strings.HasSuffix(actionPattern, ":*") {
		resourcePrefix := actionPattern[:len(actionPattern)-2]

		if actions, ok := actionsByResource[resourcePrefix]; ok {
			return actions
		}

		// No actions found for this resource
		return []string{}
	}

	// Concrete action - return as-is
	return []string{actionPattern}
}

// validateProfileRequest checks if the ProfileRequest has all required fields
func validateProfileRequest(req *authzcore.ProfileRequest) error {
	if req == nil {
		return fmt.Errorf("%w: profile request is nil", authzcore.ErrInvalidRequest)
	}
	if req.SubjectContext == nil {
		return fmt.Errorf("%w: subject context is required", authzcore.ErrInvalidRequest)
	}
	return nil
}

// normalizeNamespace converts empty namespace to "*" for cluster-scoped resources
func normalizeNamespace(namespace string) string {
	if namespace == "" {
		return "*"
	}
	return namespace
}

// policyKey joins a policy tuple into a single string key for set-based comparison. The null byte separator avoids collisions between field values.
func policyKey(policy []string) string {
	return strings.Join(policy, "\x00")
}

// computePolicyDiff computes added and removed policies between two sets of policy tuples
func computePolicyDiff(oldPolicies, newPolicies [][]string) (added, removed [][]string) {
	oldSet := make(map[string][]string, len(oldPolicies))
	for _, p := range oldPolicies {
		oldSet[policyKey(p)] = p
	}
	newSet := make(map[string][]string, len(newPolicies))
	for _, p := range newPolicies {
		newSet[policyKey(p)] = p
	}
	for key, p := range oldSet {
		if _, exists := newSet[key]; !exists {
			removed = append(removed, p)
		}
	}
	for key, p := range newSet {
		if _, exists := oldSet[key]; !exists {
			added = append(added, p)
		}
	}
	return added, removed
}

// computeActionsDiff computes the difference between existing and new actions for a role
// Returns added actions (in new but not in existing) and removed actions (in existing but not in new)
func computeActionsDiff(existingActions, newActions []string) (added, removed []string) {
	existingSet := make(map[string]struct{}, len(existingActions))
	for _, action := range existingActions {
		existingSet[action] = struct{}{}
	}

	newSet := make(map[string]struct{}, len(newActions))
	for _, action := range newActions {
		newSet[action] = struct{}{}
	}

	// Find removed actions
	for action := range existingSet {
		if _, exists := newSet[action]; !exists {
			removed = append(removed, action)
		}
	}

	// Find added actions
	for action := range newSet {
		if _, exists := existingSet[action]; !exists {
			added = append(added, action)
		}
	}

	return added, removed
}
