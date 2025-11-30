// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// extractConfigurationsFromWorkload extracts env and file configurations from all containers
// and organizes them by container name with configs vs secrets separation.
// Returns a map where each key is a container name, and the value contains configs and secrets.
// Example structure: {"app": {"configs": {"envs": [...], "files": [...]}, "secrets": {"envs": [...], "files": [...]}}}
func extractConfigurationsFromWorkload(secretReferences map[string]*v1alpha1.SecretReference, workload *v1alpha1.Workload) map[string]ContainerConfigurations {
	result := make(map[string]ContainerConfigurations)

	// Process all containers in the workload
	if workload != nil && len(workload.Spec.Containers) > 0 {
		for containerName, container := range workload.Spec.Containers {
			containerConfig := ContainerConfigurations{
				Configs: ConfigurationItems{
					Envs:  make([]EnvConfiguration, 0),
					Files: make([]FileConfiguration, 0),
				},
				Secrets: ConfigurationItems{
					Envs:  make([]EnvConfiguration, 0),
					Files: make([]FileConfiguration, 0),
				},
			}

			// Process environment variables from container
			for _, env := range container.Env {
				if env.Value != "" {
					// Direct value - goes to configs
					containerConfig.Configs.Envs = append(containerConfig.Configs.Envs, EnvConfiguration{
						Name:  env.Key,
						Value: env.Value,
					})
				} else if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
					// Resolve secret reference and add to secrets
					if remoteRef := resolveSecretRef(secretReferences, env.ValueFrom.SecretRef); remoteRef != nil {
						containerConfig.Secrets.Envs = append(containerConfig.Secrets.Envs, EnvConfiguration{
							Name:      env.Key,
							RemoteRef: remoteRef,
						})
					}
				}
			}

			// Process file configurations from container
			for _, file := range container.Files {
				if file.Value != "" {
					// Direct content - goes to configs
					containerConfig.Configs.Files = append(containerConfig.Configs.Files, FileConfiguration{
						Name:      file.Key,
						MountPath: file.MountPath,
						Value:     file.Value,
					})
				} else if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
					// Resolve secret reference and add to secrets
					if remoteRef := resolveSecretRef(secretReferences, file.ValueFrom.SecretRef); remoteRef != nil {
						containerConfig.Secrets.Files = append(containerConfig.Secrets.Files, FileConfiguration{
							Name:      file.Key,
							MountPath: file.MountPath,
							RemoteRef: remoteRef,
						})
					}
				}
			}

			result[containerName] = containerConfig
		}
	}

	return result
}

// resolveSecretRef is a reusable helper that resolves a SecretReference to remoteRef information.
// Returns nil if the SecretReference cannot be resolved.
func resolveSecretRef(secretReferences map[string]*v1alpha1.SecretReference, secretRef *v1alpha1.SecretKeyRef) *RemoteRefData {
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
			return &RemoteRefData{
				Key:      dataSource.RemoteRef.Key,
				Property: dataSource.RemoteRef.Property,
				Version:  dataSource.RemoteRef.Version,
			}
		}
	}

	return nil
}
