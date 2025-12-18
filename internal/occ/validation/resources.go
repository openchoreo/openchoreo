// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"regexp"
)

const (
	// ResourceNamePattern is the regex pattern that resource names must follow
	ResourceNamePattern = "^[a-z0-9][a-z0-9-]*[a-z0-9]$"
)

var namePattern = regexp.MustCompile(ResourceNamePattern)

// ValidateName validates that a resource name follows the required pattern
func ValidateName(resourceType string, val interface{}) error {
	str, ok := val.(string)
	if !ok {
		return fmt.Errorf("invalid type for resource name")
	}

	if str == "" {
		return fmt.Errorf("empty field for %s name", resourceType)
	}
	if !namePattern.MatchString(str) {
		errMsg := fmt.Sprintf("invalid %s name format, expected: %s", resourceType, ResourceNamePattern)
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// ValidateOrganizationName validates an organization name
func ValidateOrganizationName(val interface{}) error {
	return ValidateName("organization", val)
}

// ValidateProjectName validates a project name
func ValidateProjectName(val interface{}) error {
	return ValidateName("project", val)
}

// ValidateComponentName validates a component name
func ValidateComponentName(val interface{}) error {
	return ValidateName("component", val)
}
