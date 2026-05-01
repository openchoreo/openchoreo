// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/cel-go/cel"
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

// failClosed returns the value to use when condition evaluation cannot
// produce a definitive result (parse, compile, eval, or non-bool errors).
// For deny policies we return true so the deny still applies; for allow
// policies we return false so the allow does not grant on bad data.
//
// Unknown / corrupted eft values are treated as deny — most conservative.
func failClosed(policyEft string) bool {
	return policyEft != string(authzcore.PolicyEffectAllow)
}

// serializeAuthzContext serializes an authzcore.Context to JSON for passing as a Casbin enforce arg.
func serializeAuthzContext(ctx authzcore.Context) (string, error) {
	b, err := json.Marshal(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to serialize context: %w", err)
	}
	return string(b), nil
}

// serializeAuthzConditions serializes a slice of AuthzCondition to JSON for storing in the policy.
// Returns an empty context JSON if the slice is empty.
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
func ConditionMatcher(requestCtxJSON, requestAction, policyCond, policyEft, bindingName string) bool {
	if isPolicyConditionEmpty(policyCond) {
		return true
	}

	var conditions []authzv1alpha1.AuthzCondition
	if err := json.Unmarshal([]byte(policyCond), &conditions); err != nil {
		slog.Default().Error("condMatch: failed to unmarshal policy conditions",
			"reason", "policy_unmarshal_error",
			"policy_eft", policyEft,
			"request_action", requestAction,
			"binding_name", bindingName,
			"error", err)
		return failClosed(policyEft)
	}

	matching := filterConditionsByAction(conditions, requestAction)
	// if the condition entries don't target the considered action then the RBAC decision stands as-is
	if len(matching) == 0 {
		return true
	}

	activation, ok := buildActivationForRequest(requestCtxJSON, requestAction)
	if !ok {
		slog.Default().Error("condMatch: activation build failed",
			"reason", "activation_build_error",
			"policy_eft", policyEft,
			"request_action", requestAction,
			"binding_name", bindingName)
		return failClosed(policyEft)
	}

	return anyConditionMatches(matching, activation, policyEft, bindingName)
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

// evalResult is the tri-state outcome of evaluating a single CEL condition entry.
// Splitting "errored" from "cleanly false" lets callers distinguish a policy
// that genuinely doesn't match from one whose match status is unknown
type evalResult int

const (
	evalAllow  evalResult = iota // expression cleanly evaluated to true
	evalReject                   // expression cleanly evaluated to false
	evalError                    // expression could not be evaluated (compile/eval/non-bool)
)

// anyConditionMatches decides whether at least one entry's CEL expression matches.
//
// Behavior:
//   - any entry errored → policy CR is in an undefined state, so we fail closed
//     by effect (deny policies match, allow policies do not). This takes
//     precedence over any sibling entry's clean result, because we don't know
//     what the broken entry was supposed to do.
//   - otherwise any entry cleanly matched → match (true).
//   - otherwise all entries cleanly rejected → no match (false).
func anyConditionMatches(entries []authzv1alpha1.AuthzCondition, activation interpreter.Activation, policyEft, bindingName string) bool {
	sawMatch := false
	sawError := false
	for _, entry := range entries {
		switch evalCondition(entry.Expression, activation, policyEft, bindingName) {
		case evalAllow:
			sawMatch = true
		case evalError:
			sawError = true
		case evalReject:
			// noop - keep checking other entries for a possible match or error
		}
	}
	if sawError {
		return failClosed(policyEft)
	}
	return sawMatch
}

// evalCondition compiles and evaluates a single CEL expression and returns the evalResult.
func evalCondition(expr string, activation interpreter.Activation, policyEft, bindingName string) evalResult {
	prg, err := compileCEL(expr)
	if err != nil {
		logEvalError(policyEft, expr, bindingName, "compile_error", err)
		return evalError
	}

	out, _, err := prg.Eval(activation)
	if err != nil {
		logEvalError(policyEft, expr, bindingName, "eval_error", err)
		return evalError
	}

	result, isBool := out.Value().(bool)
	if !isBool {
		logEvalError(policyEft, expr, bindingName, "non_bool_result", fmt.Errorf("got %T", out.Value()))
		return evalError
	}

	slog.Default().Debug("condMatch: CEL eval result", "expression", expr, "result", result)
	if result {
		return evalAllow
	}
	return evalReject
}

func logEvalError(policyEft, expr, bindingName, reason string, err error) {
	slog.Default().Error("condMatch: condition evaluation failed",
		"reason", reason,
		"policy_eft", policyEft,
		"binding_name", bindingName,
		"expression", expr,
		"error", err)
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

// condMatchWrapper is a wrapper for condMatch to work with Casbin's function interface.
func condMatchWrapper(args ...interface{}) (interface{}, error) {
	if len(args) != 5 {
		return false, fmt.Errorf("condMatch requires exactly 5 arguments")
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

	policyEft, ok := args[3].(string)
	if !ok {
		return false, fmt.Errorf("policy eft argument must be a string")
	}

	bindingName, ok := args[4].(string)
	if !ok {
		return false, fmt.Errorf("binding name argument must be a string")
	}

	return ConditionMatcher(requestCtx, requestAction, policyConds, policyEft, bindingName), nil
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

// compileCEL compiles a CEL expression and returns a ready-to-evaluate Program.
func compileCEL(expr string) (cel.Program, error) {
	env, err := authzcore.GetCELEnv()
	if err != nil {
		return nil, fmt.Errorf("CEL environment unavailable: %w", err)
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile error: %w", issues.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("program construction error: %w", err)
	}
	return prg, nil
}

// buildCelActivation constructs the CEL activation map from the request context,
// gated by the allowed attributes for the action.
// Every allowed attribute is bound — either to the value from ctx
// or to a type-appropriate zero — so CEL expressions never fault on unbound variables
// when a request omits an optional field.
func buildCelActivation(authzCtx authzcore.Context, allowedAttrs []authzcore.AttributeSpec) (interpreter.Activation, error) {
	if len(allowedAttrs) == 0 {
		return interpreter.NewActivation(map[string]any{})
	}

	ctxAttrs, err := convertCtxToAttrMap(authzCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to convert context for CEL activation: %w", err)
	}

	activationByRoot := map[string]map[string]any{}
	for _, spec := range allowedAttrs {
		root, leaf := spec.Root(), spec.Leaf()
		if root == "" || leaf == "" {
			continue
		}
		if activationByRoot[root] == nil {
			activationByRoot[root] = map[string]any{}
		}
		attrValue, ok := ctxAttrs[root][leaf]
		if !ok {
			attrValue = zeroForCELType(spec.CELType)
		}
		activationByRoot[root][leaf] = attrValue
	}

	activation := make(map[string]any, len(activationByRoot))
	for root, leafValues := range activationByRoot {
		activation[root] = leafValues
	}
	return interpreter.NewActivation(activation)
}

// convertCtxToAttrMap JSON-round-trips ctx into a two-level map (root → leaf → value)
// so the json tags on authzcore.Context drive the CEL variable names automatically.
func convertCtxToAttrMap(ctx authzcore.Context) (map[string]map[string]any, error) {
	ctxJSON, err := json.Marshal(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize context for CEL activation: %w", err)
	}
	var ctxAttrs map[string]map[string]any
	if err := json.Unmarshal(ctxJSON, &ctxAttrs); err != nil {
		return nil, fmt.Errorf("failed to deserialize context for CEL activation: %w", err)
	}
	return ctxAttrs, nil
}

// zeroForCELType returns the Go zero value corresponding to a CEL type
func zeroForCELType(t *cel.Type) any {
	switch t {
	case cel.StringType:
		return ""
	case cel.BoolType:
		return false
	case cel.IntType:
		return int64(0)
	case cel.DoubleType:
		return 0.0
	default:
		return nil
	}
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
