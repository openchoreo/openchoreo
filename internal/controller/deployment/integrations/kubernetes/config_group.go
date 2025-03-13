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

package kubernetes

import (
	"regexp"
	"strings"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
)

// This file contains the helper functions that are related to deploying configuration groups in Kubernetes.

// findConfigGroupByName returns the first matching configuration group with the given name.
// If no matching configuration group is found, it returns nil.
func findConfigGroupByName(configGroups []*choreov1.ConfigurationGroup, name string) *choreov1.ConfigurationGroup {
	for _, cg := range configGroups {
		if controller.GetName(cg) == name {
			return cg
		}
	}
	return nil
}

// findConfigGroupValueForEnv returns the configuration value for the given environment
// from the given configuration value list. Returns nil if no matching configuration value is found.
func findConfigGroupValueForEnv(value []choreov1.ConfigurationValue,
	envGroup []choreov1.EnvironmentGroup, env *choreov1.Environment) *choreov1.ConfigurationValue {
	for _, v := range value {
		envName := controller.GetName(env)
		if v.Environment == envName {
			return &v
		}
		// If the environment group reference is set,
		// find if there is a value for the environment in the environment group
		for _, eg := range envGroup {
			if eg.Name == v.EnvironmentGroupRef {
				for _, ege := range eg.Environments {
					if ege == envName {
						return &v
					}
				}
			}
		}
	}
	return nil
}

var invalidEnvKeyChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeEnvVarKey(key string) string {
	// Replace invalid characters with underscore.
	newKey := strings.ToUpper(invalidEnvKeyChars.ReplaceAllString(key, "_"))

	// If the result is empty, return a default key.
	if len(newKey) == 0 {
		return ""
	}

	// Ensure the first character is a letter or an underscore.
	// Example: "123" -> "_123"
	first := newKey[0]
	if !(first >= 'A' && first <= 'Z' || first == '_') {
		newKey = "_" + newKey
	}
	return newKey
}
