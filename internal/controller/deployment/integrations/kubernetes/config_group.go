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

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

// This file contains the helper functions that are related to deploying configuration groups in Kubernetes.

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

// mappedConfig stores the filtered configuration values (plain text and secret) that are mapped
// in the deployable artifact to a single configuration group.
type mappedConfig struct {
	PlainConfigs  []plainConfig
	SecretConfigs []secretConfig
}

// plainConfig stores an individual plain text configuration value
// that is mapped in the deployable artifact to a configuration group.
type plainConfig struct {
	EnvVarKey      string
	ConfigGroupKey string
}

// secretConfig stores an individual secret configuration value
// that is mapped in the deployable artifact to a configuration group.
type secretConfig struct {
	EnvVarKey      string
	ConfigGroupKey string
}

// newMappedConfig creates a new mappedConfig instance for the given configuration group.
func newMappedConfig(deployCtx *dataplane.DeploymentContext, cg *choreov1.ConfigurationGroup) *mappedConfig {
	// Create a mapping of key to env specific value for fast lookup
	// The map value can be nil if the configuration value is not found for the environment
	cgKeyValueMapping := make(map[string]*choreov1.ConfigurationValue)
	for _, cgConfig := range cg.Spec.Configurations {
		cgKeyValueMapping[cgConfig.Key] = findConfigGroupValueForEnv(cgConfig.Values,
			cg.Spec.EnvironmentGroups, deployCtx.Environment)
	}

	appCfg := deployCtx.DeployableArtifact.Spec.Configuration.Application

	plainConfigs := make([]plainConfig, 0)
	secretConfigs := make([]secretConfig, 0)

	// Find individual configuration group key mappings in the Env section
	// Example Configuration group mapping:
	// env:
	//   - key: REDIS_HOST
	//	   valueFrom:
	//	     configurationGroupRef:
	//		   name: redis-config
	//		   key: redis-host
	for _, ev := range appCfg.Env {
		if ev.Key == "" {
			continue
		}
		if ev.ValueFrom == nil {
			continue
		}
		if ev.ValueFrom.ConfigurationGroupRef == nil {
			continue
		}
		if ev.ValueFrom.ConfigurationGroupRef.Name != controller.GetName(cg) {
			continue
		}
		cgRef := ev.ValueFrom.ConfigurationGroupRef
		// Find the matching configuration value for the key if it exists
		cgValue, ok := cgKeyValueMapping[cgRef.Key]
		if !ok || cgValue == nil {
			// This can happen in following cases:
			// 1. Deployable artifact is trying to map a configuration key that does not exist in the configuration group.
			// 2. Configuration value is not set for the environment in the configuration group.
			continue
		}

		if cgValue.Value != "" {
			p := plainConfig{
				EnvVarKey:      ev.Key,
				ConfigGroupKey: cgRef.Key,
			}
			plainConfigs = append(plainConfigs, p)
		} else if cgValue.VaultKey != "" {
			s := secretConfig{
				EnvVarKey:      ev.Key,
				ConfigGroupKey: cgRef.Key,
			}
			secretConfigs = append(secretConfigs, s)
		}
	}

	// Find bulk configuration group mappings in the EnvFrom section
	// Example Configuration group injection:
	// envFrom:
	//   - configurationGroupRef:
	//       name: redis-config
	for _, evf := range appCfg.EnvFrom {
		if evf.ConfigurationGroupRef == nil {
			continue
		}
		if evf.ConfigurationGroupRef.Name != controller.GetName(cg) {
			continue
		}
		// Add all the keys from the configuration group
		for key, value := range cgKeyValueMapping {
			// Value is not set for the environment in the configuration group
			if value == nil {
				continue
			}
			if value.Value != "" {
				p := plainConfig{
					EnvVarKey:      sanitizeEnvVarKey(key),
					ConfigGroupKey: key,
				}
				plainConfigs = append(plainConfigs, p)
			} else if value.VaultKey != "" {
				s := secretConfig{
					EnvVarKey:      sanitizeEnvVarKey(key),
					ConfigGroupKey: key,
				}
				secretConfigs = append(secretConfigs, s)
			}
		}
	}

	return &mappedConfig{
		PlainConfigs:  plainConfigs,
		SecretConfigs: secretConfigs,
	}
}
