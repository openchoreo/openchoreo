/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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
