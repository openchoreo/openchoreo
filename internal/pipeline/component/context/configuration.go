// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// extractConfigurationsFromWorkload extracts env and file configurations from workload containers
// and separates them into configs vs secrets based on valueFrom usage.
func extractConfigurationsFromWorkload(secretReferences map[string]*v1alpha1.SecretReference, workload *v1alpha1.Workload) map[string]any {
	configs := map[string][]any{
		"envs":  make([]any, 0),
		"files": make([]any, 0),
	}
	secrets := map[string][]any{
		"envs":  make([]any, 0),
		"files": make([]any, 0),
	}

	// Process all containers (only if workload exists and has containers)
	if workload != nil && len(workload.Spec.Containers) > 0 {
		for _, container := range workload.Spec.Containers {
			// Process environment variables
			for _, env := range container.Env {
				if env.Value != "" {
					// Direct value - goes to configs
					configs["envs"] = append(configs["envs"], map[string]any{
						"name":  env.Key,
						"value": env.Value,
					})
				} else if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
					// Resolve secret reference and add to secrets
					if remoteRef := resolveSecretRef(secretReferences, env.ValueFrom.SecretRef); remoteRef != nil {
						secrets["envs"] = append(secrets["envs"], map[string]any{
							"name":      env.Key,
							"remoteRef": remoteRef,
						})
					}
				}
			}

			// Process file configurations
			for _, file := range container.File {
				if file.Value != "" {
					// Direct content - goes to configs
					configs["files"] = append(configs["files"], map[string]any{
						"name":      file.Key,
						"mountPath": file.MountPath,
						"value":     file.Value,
					})
				} else if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
					// Resolve secret reference and add to secrets
					if remoteRef := resolveSecretRef(secretReferences, file.ValueFrom.SecretRef); remoteRef != nil {
						secrets["files"] = append(secrets["files"], map[string]any{
							"name":      file.Key,
							"mountPath": file.MountPath,
							"remoteRef": remoteRef,
						})
					}
				}
			}
		}
	}

	result := make(map[string]any)

	configsResult := make(map[string]any)
	configsResult["envs"] = configs["envs"]
	configsResult["files"] = configs["files"]
	result["configs"] = configsResult

	secretsResult := make(map[string]any)
	secretsResult["envs"] = secrets["envs"]
	secretsResult["files"] = secrets["files"]
	result["secrets"] = secretsResult

	return result
}

// applyConfigurationOverrides merges configuration overrides from ComponentDeployment into existing configurations.
// If a configuration with the same name exists, it updates the value. If it's new, it adds it.
func applyConfigurationOverrides(secretReferences map[string]*v1alpha1.SecretReference, baseConfigurations map[string]any, overrides *v1alpha1.EnvConfigurationOverrides) map[string]any {
	// Create maps for easy lookup by name
	configEnvMap := make(map[string]map[string]any)
	configFileMap := make(map[string]map[string]any)
	secretEnvMap := make(map[string]map[string]any)
	secretFileMap := make(map[string]map[string]any)

	// Populate maps from base configurations
	configs := baseConfigurations["configs"].(map[string]any)
	secrets := baseConfigurations["secrets"].(map[string]any)

	populateConfigMaps(configs, secrets, configEnvMap, configFileMap, secretEnvMap, secretFileMap)

	// Process environment variable overrides
	processEnvOverrides(secretReferences, overrides.Env, configEnvMap, secretEnvMap)

	// Process file overrides
	processFileOverrides(secretReferences, overrides.Files, configFileMap, secretFileMap)

	// Convert maps back to arrays and update base configurations
	configs["envs"] = mapToSlice(configEnvMap)
	configs["files"] = mapToSlice(configFileMap)
	secrets["envs"] = mapToSlice(secretEnvMap)
	secrets["files"] = mapToSlice(secretFileMap)

	baseConfigurations["configs"] = configs
	baseConfigurations["secrets"] = secrets

	return baseConfigurations
}

// populateConfigMaps populates config and secret maps from base configurations.
func populateConfigMaps(configs, secrets map[string]any,
	configEnvMap, configFileMap, secretEnvMap, secretFileMap map[string]map[string]any) {
	for _, envItem := range configs["envs"].([]any) {
		if envMap, ok := envItem.(map[string]any); ok {
			if name, ok := envMap["name"].(string); ok {
				configEnvMap[name] = envMap
			}
		}
	}

	for _, fileItem := range configs["files"].([]any) {
		if fileMap, ok := fileItem.(map[string]any); ok {
			if name, ok := fileMap["name"].(string); ok {
				configFileMap[name] = fileMap
			}
		}
	}

	for _, envItem := range secrets["envs"].([]any) {
		if envMap, ok := envItem.(map[string]any); ok {
			if name, ok := envMap["name"].(string); ok {
				secretEnvMap[name] = envMap
			}
		}
	}

	for _, fileItem := range secrets["files"].([]any) {
		if fileMap, ok := fileItem.(map[string]any); ok {
			if name, ok := fileMap["name"].(string); ok {
				secretFileMap[name] = fileMap
			}
		}
	}
}

// resolveSecretRef is a reusable helper that resolves a SecretReference to remoteRef information.
// Returns nil if the SecretReference cannot be resolved.
func resolveSecretRef(secretReferences map[string]*v1alpha1.SecretReference, secretRef *v1alpha1.SecretKeyRef) map[string]any {
	if secretRef == nil {
		return nil
	}

	// Look up SecretReference from map
	ref, ok := secretReferences[secretRef.Name]
	if !ok {
		return nil
	}

	// Find the matching secret key in the SecretReference data
	for _, dataSource := range ref.Spec.Data {
		if dataSource.SecretKey == secretRef.Key {
			remoteRef := map[string]any{"key": dataSource.RemoteRef.Key}
			if dataSource.RemoteRef.Property != "" {
				remoteRef["property"] = dataSource.RemoteRef.Property
			}
			return remoteRef
		}
	}

	return nil
}

// processEnvOverrides processes environment variable overrides.
func processEnvOverrides(secretReferences map[string]*v1alpha1.SecretReference,
	envOverrides []v1alpha1.EnvVar, configEnvMap, secretEnvMap map[string]map[string]any) {
	for _, envOverride := range envOverrides {
		if envOverride.Value != "" {
			// Direct value - goes to configs
			configEnvMap[envOverride.Key] = map[string]any{
				"name":  envOverride.Key,
				"value": envOverride.Value,
			}
		} else if envOverride.ValueFrom != nil && envOverride.ValueFrom.SecretRef != nil {
			// Resolve secret reference and add to secrets
			if remoteRef := resolveSecretRef(secretReferences, envOverride.ValueFrom.SecretRef); remoteRef != nil {
				secretEnvMap[envOverride.Key] = map[string]any{
					"name":      envOverride.Key,
					"remoteRef": remoteRef,
				}
			}
		}
	}
}

// processFileOverrides processes file configuration overrides.
func processFileOverrides(secretReferences map[string]*v1alpha1.SecretReference,
	fileOverrides []v1alpha1.FileVar, configFileMap, secretFileMap map[string]map[string]any) {
	for _, fileOverride := range fileOverrides {
		if fileOverride.Value != "" {
			// Direct value - goes to configs
			configFileMap[fileOverride.Key] = map[string]any{
				"name":      fileOverride.Key,
				"mountPath": fileOverride.MountPath,
				"value":     fileOverride.Value,
			}
		} else if fileOverride.ValueFrom != nil && fileOverride.ValueFrom.SecretRef != nil {
			// Resolve secret reference and add to secrets
			if remoteRef := resolveSecretRef(secretReferences, fileOverride.ValueFrom.SecretRef); remoteRef != nil {
				secretFileMap[fileOverride.Key] = map[string]any{
					"name":      fileOverride.Key,
					"mountPath": fileOverride.MountPath,
					"remoteRef": remoteRef,
				}
			}
		}
	}
}

// mapToSlice converts a map to a slice of its values.
func mapToSlice(m map[string]map[string]any) []any {
	result := make([]any, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}
