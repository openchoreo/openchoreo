// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// extractConfigurationsFromWorkload extracts env and file configurations from workload containers
// and separates them into configs vs secrets based on valueFrom usage.
func extractConfigurationsFromWorkload(ctx context.Context, client client.Client, namespace string, workload *v1alpha1.Workload) map[string]any {
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
				envMap := map[string]any{
					"name": env.Key,
				}

				if env.Value != "" {
					// Direct value - goes to configs
					envMap["value"] = env.Value
					configs["envs"] = append(configs["envs"], envMap)
				} else if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
					// Fetch SecretReference from cluster
					if ctx != nil && client != nil && namespace != "" {
						secretRef := &v1alpha1.SecretReference{}
						secretRefKey := types.NamespacedName{
							Name:      env.ValueFrom.SecretRef.Name,
							Namespace: namespace,
						}
						if err := client.Get(ctx, secretRefKey, secretRef); err == nil {
							// Find the matching secret key in the SecretReference data
							for _, dataSource := range secretRef.Spec.Data {
								if dataSource.SecretKey == env.ValueFrom.SecretRef.Key {
									// Add remoteRef information to secrets
									secretEnvMap := map[string]any{
										"name": env.Key,
										"remoteRef": map[string]any{
											"key": dataSource.RemoteRef.Key,
										},
									}
									if dataSource.RemoteRef.Property != "" {
										secretEnvMap["remoteRef"].(map[string]any)["property"] = dataSource.RemoteRef.Property
									}
									secrets["envs"] = append(secrets["envs"], secretEnvMap)
									break
								}
							}
						}
					}
				}
			}

			// Process file configurations
			for _, file := range container.File {
				fileMap := map[string]any{
					"name":      file.Key,
					"mountPath": file.MountPath,
				}

				if file.Value != "" {
					// Direct content - goes to configs
					fileMap["value"] = file.Value
					configs["files"] = append(configs["files"], fileMap)
				} else if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
					// Fetch SecretReference from cluster
					if ctx != nil && client != nil && namespace != "" {
						secretRef := &v1alpha1.SecretReference{}
						secretRefKey := types.NamespacedName{
							Name:      file.ValueFrom.SecretRef.Name,
							Namespace: namespace,
						}
						if err := client.Get(ctx, secretRefKey, secretRef); err == nil {
							// Find the matching secret key in the SecretReference data
							for _, dataSource := range secretRef.Spec.Data {
								if dataSource.SecretKey == file.ValueFrom.SecretRef.Key {
									// Add remoteRef information to secrets
									secretFileMap := map[string]any{
										"name":      file.Key,
										"mountPath": file.MountPath,
										"remoteRef": map[string]any{
											"key": dataSource.RemoteRef.Key,
										},
									}
									if dataSource.RemoteRef.Property != "" {
										secretFileMap["remoteRef"].(map[string]any)["property"] = dataSource.RemoteRef.Property
									}
									secrets["files"] = append(secrets["files"], secretFileMap)
									break
								}
							}
						}
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
func applyConfigurationOverrides(ctx context.Context, client client.Client, namespace string, baseConfigurations map[string]any, overrides *v1alpha1.EnvConfigurationOverrides) map[string]any {
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
	processEnvOverrides(ctx, client, namespace, overrides.Env, configEnvMap, secretEnvMap)

	// Process file overrides
	processFileOverrides(ctx, client, namespace, overrides.Files, configFileMap, secretFileMap)

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

// processEnvOverrides processes environment variable overrides.
func processEnvOverrides(ctx context.Context, client client.Client, namespace string,
	envOverrides []v1alpha1.EnvVar, configEnvMap, secretEnvMap map[string]map[string]any) {
	for _, envOverride := range envOverrides {
		if envOverride.Value != "" {
			// Direct value - goes to configs
			configEnvMap[envOverride.Key] = map[string]any{
				"name":  envOverride.Key,
				"value": envOverride.Value,
			}
		} else if envOverride.ValueFrom != nil && envOverride.ValueFrom.SecretRef != nil {
			// Fetch SecretReference and add to secrets
			resolveSecretRefForEnv(ctx, client, namespace, envOverride, secretEnvMap)
		}
	}
}

// processFileOverrides processes file configuration overrides.
func processFileOverrides(ctx context.Context, client client.Client, namespace string,
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
			// Fetch SecretReference and add to secrets
			resolveSecretRefForFile(ctx, client, namespace, fileOverride, secretFileMap)
		}
	}
}

// resolveSecretRefForEnv resolves a SecretReference for an environment variable.
func resolveSecretRefForEnv(ctx context.Context, client client.Client, namespace string,
	envOverride v1alpha1.EnvVar, secretEnvMap map[string]map[string]any) {
	if ctx == nil || client == nil || namespace == "" {
		return
	}

	secretRef := &v1alpha1.SecretReference{}
	secretRefKey := types.NamespacedName{
		Name:      envOverride.ValueFrom.SecretRef.Name,
		Namespace: namespace,
	}

	if err := client.Get(ctx, secretRefKey, secretRef); err != nil {
		return
	}

	// Find the matching secret key in the SecretReference data
	for _, dataSource := range secretRef.Spec.Data {
		if dataSource.SecretKey == envOverride.ValueFrom.SecretRef.Key {
			remoteRef := map[string]any{"key": dataSource.RemoteRef.Key}
			if dataSource.RemoteRef.Property != "" {
				remoteRef["property"] = dataSource.RemoteRef.Property
			}
			secretEnvMap[envOverride.Key] = map[string]any{
				"name":      envOverride.Key,
				"remoteRef": remoteRef,
			}
			break
		}
	}
}

// resolveSecretRefForFile resolves a SecretReference for a file configuration.
func resolveSecretRefForFile(ctx context.Context, client client.Client, namespace string,
	fileOverride v1alpha1.FileVar, secretFileMap map[string]map[string]any) {
	if ctx == nil || client == nil || namespace == "" {
		return
	}

	secretRef := &v1alpha1.SecretReference{}
	secretRefKey := types.NamespacedName{
		Name:      fileOverride.ValueFrom.SecretRef.Name,
		Namespace: namespace,
	}

	if err := client.Get(ctx, secretRefKey, secretRef); err != nil {
		return
	}

	// Find the matching secret key in the SecretReference data
	for _, dataSource := range secretRef.Spec.Data {
		if dataSource.SecretKey == fileOverride.ValueFrom.SecretRef.Key {
			remoteRef := map[string]any{"key": dataSource.RemoteRef.Key}
			if dataSource.RemoteRef.Property != "" {
				remoteRef["property"] = dataSource.RemoteRef.Property
			}
			secretFileMap[fileOverride.Key] = map[string]any{
				"name":      fileOverride.Key,
				"mountPath": fileOverride.MountPath,
				"remoteRef": remoteRef,
			}
			break
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
