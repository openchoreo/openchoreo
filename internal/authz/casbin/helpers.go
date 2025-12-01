// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

type HierarchyResourcePrefix string

const (
	OrganizationResourcePrefix     HierarchyResourcePrefix = "org"
	OrganizationUnitResourcePrefix HierarchyResourcePrefix = "ou"
	ProjectResourcePrefix          HierarchyResourcePrefix = "project"
	ComponentResourcePrefix        HierarchyResourcePrefix = "component"
)

// resourceMatch checks if a requested resource matches a policy resource using hierarchical prefix matching.
// For example, policy "org/acme" matches request "org/acme/project/p1/component/c1"
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
	// e.g., policy "org/acme" matches request "org/acme/project/p1"
	return strings.HasPrefix(requestResource, policyResource+"/")
}

// ctxMatch checks if the request context matches the policy context.
// Currently a placeholder - you can implement custom context matching logic here.
// For example, matching based on environment, time, or other conditional attributes.
func ctxMatch(requestCtx, policyCtx string) bool {
	// Empty policy context means no constraints - always matches
	if policyCtx == "" {
		return true
	}

	// TODO: Implement context matching logic based on your requirements
	// For now, we'll just check for exact match or empty request context
	return requestCtx == policyCtx || requestCtx == ""
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

// ctxMatchWrapper is a wrapper for ctxMatch to work with Casbin's function interface
func ctxMatchWrapper(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return false, fmt.Errorf("ctxMatch requires exactly 2 arguments")
	}

	requestCtx, ok := args[0].(string)
	if !ok {
		return false, fmt.Errorf("first argument must be a string")
	}

	policyCtx, ok := args[1].(string)
	if !ok {
		return false, fmt.Errorf("second argument must be a string")
	}

	return ctxMatch(requestCtx, policyCtx), nil
}

// actionMatch checks if a requested action matches a role's action pattern with wildcard support.
// Supports:
// - Exact match: "component:read" matches "component:read"
// - Verb wildcard: "component:*" matches "component:read", "component:write", etc.
// - Full wildcard: "*" matches any action
func actionMatch(roleAction, requestAction string) bool {
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

// actionMatchWrapper is a wrapper for actionMatch to work with Casbin's MatchingFunc interface
// This is used for the g (role-action) matcher
func actionMatchWrapper(arg1, arg2 string) bool {
	return actionMatch(arg1, arg2)
}

// hierarchyToResourcePath converts ResourceHierarchy to a hierarchical resource path string
func hierarchyToResourcePath(hierarchy authzcore.ResourceHierarchy) string {
	path := ""

	if hierarchy.Organization != "" {
		path = fmt.Sprintf("%s/%s", OrganizationResourcePrefix, hierarchy.Organization)
	}

	for _, ou := range hierarchy.OrganizationUnits {
		if ou != "" {
			path = fmt.Sprintf("%s/%s/%s", path, OrganizationUnitResourcePrefix, ou)
		}
	}

	if hierarchy.Project != "" {
		path = fmt.Sprintf("%s/%s/%s", path, ProjectResourcePrefix, hierarchy.Project)
	}

	if hierarchy.Component != "" {
		path = fmt.Sprintf("%s/%s/%s", path, ComponentResourcePrefix, hierarchy.Component)
	}

	path = strings.Trim(path, "/")

	// Empty hierarchy means global wildcard
	if path == "" {
		return "*"
	}

	return path
}

// resourcePathToHierarchy converts a hierarchical resource path string back to ResourceHierarchy
func resourcePathToHierarchy(resourcePath string) authzcore.ResourceHierarchy {
	hierarchy := authzcore.ResourceHierarchy{}

	// Global wildcard  map to empty hierarchy
	if resourcePath == "*" {
		return hierarchy
	}

	segments := strings.Split(resourcePath, "/")

	for i := 0; i < len(segments)-1; i += 2 {
		prefix := segments[i]
		value := segments[i+1]

		switch HierarchyResourcePrefix(prefix) {
		case OrganizationResourcePrefix:
			hierarchy.Organization = value
		case OrganizationUnitResourcePrefix:
			hierarchy.OrganizationUnits = append(hierarchy.OrganizationUnits, value)
		case ProjectResourcePrefix:
			hierarchy.Project = value
		case ComponentResourcePrefix:
			hierarchy.Component = value
		}
	}

	return hierarchy
}

// Extract group, service_account from subject
// hack: this is temporarily done to work with thunder jwt token structure
// need a proper layer to parse different token types in future
func populateSubjectClaims(subject *authzcore.Subject) error {
	jwtToken := subject.JwtToken

	// Parse JWT without verification (just to extract claims)
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(jwtToken, jwt.MapClaims{})
	if err != nil {
		return fmt.Errorf("failed to parse JWT token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("failed to parse JWT claims")
	}

	// Extract groups and service accounts
	var entitlements []string

	// Extract "group" (singular) - Thunder JWT uses this
	if group, ok := claims["group"]; ok {
		switch v := group.(type) {
		case string:
			if v != "" {
				entitlements = append(entitlements, v)
			}
		case []interface{}:
			for _, g := range v {
				if str, ok := g.(string); ok && str != "" {
					entitlements = append(entitlements, str)
				}
			}
		}
		subject.Type = authzcore.SubjectTypeUser
		subject.Claims = entitlements
		return nil
	}

	// Extract service account
	if sa, ok := claims["service_account"].(string); ok && sa != "" {
		entitlements = append(entitlements, sa)
		subject.Type = authzcore.SubjectTypeServiceAccount
		subject.Claims = entitlements
		return nil
	}

	return fmt.Errorf("no valid subject claims found in token")

}
