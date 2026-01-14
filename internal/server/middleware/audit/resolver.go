// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"net/http"
	"regexp"
	"strings"
)

// ActionResolver resolves HTTP requests to audit action definitions
type ActionResolver struct {
	definitions []ActionDefinition
	patterns    []*routePattern
}

// routePattern represents a compiled route pattern for matching
type routePattern struct {
	definition ActionDefinition
	regex      *regexp.Regexp
}

// NewActionResolver creates a new action resolver with the given definitions
func NewActionResolver(definitions []ActionDefinition) *ActionResolver {
	patterns := make([]*routePattern, 0, len(definitions))

	for _, def := range definitions {
		// Convert Go 1.22+ ServeMux pattern to regex
		// Pattern format: "METHOD /path/{var}/subpath" or "/path/{var}/subpath"
		pattern := def.Pattern
		method := def.Method

		// If pattern includes method prefix, extract it
		if extractedMethod, extractedPattern, ok := strings.Cut(pattern, " "); ok {
			method = strings.ToUpper(extractedMethod)
			pattern = strings.TrimSpace(extractedPattern)
		}

		// Convert path pattern to regex
		// Replace {var} with named capture groups
		regexPattern := "^" + regexp.QuoteMeta(pattern) + "$"
		regexPattern = strings.ReplaceAll(regexPattern, `\{`, `(?P<`)
		regexPattern = strings.ReplaceAll(regexPattern, `\}`, `>[^/]+)`)

		regex, err := regexp.Compile(regexPattern)
		if err != nil {
			// Skip invalid patterns (should not happen with valid patterns)
			continue
		}

		patterns = append(patterns, &routePattern{
			definition: ActionDefinition{
				Method:   method,
				Pattern:  def.Pattern,
				Action:   def.Action,
				Category: def.Category,
			},
			regex: regex,
		})
	}

	return &ActionResolver{
		definitions: definitions,
		patterns:    patterns,
	}
}

// Resolve attempts to match an HTTP request to an action definition
// Returns the matching definition or nil if no match is found
func (r *ActionResolver) Resolve(req *http.Request) *ActionDefinition {
	method := req.Method
	path := req.URL.Path

	for _, pattern := range r.patterns {
		// Check method match (empty method matches all)
		if pattern.definition.Method != "" && pattern.definition.Method != method {
			continue
		}

		// Check path match
		if pattern.regex.MatchString(path) {
			return &pattern.definition
		}
	}

	return nil
}
