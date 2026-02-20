// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ExtractConfigurationsFromWorkload extracts env and file configurations from the workload container
// and organizes them with configs vs secrets separation.
// Returns the container's configs and secrets.
// Always initializes empty slices for envs and files to ensure they're never nil.
// Example structure: {"configs": {"envs": [...], "files": [...]}, "secrets": {"envs": [...], "files": [...]}}
func ExtractConfigurationsFromWorkload(secretReferences map[string]*v1alpha1.SecretReference, workload *v1alpha1.Workload) ContainerConfigurations {
	result := ContainerConfigurations{
		Configs: ConfigurationItems{
			Envs:  []EnvConfiguration{},
			Files: []FileConfiguration{},
		},
		Secrets: ConfigurationItems{
			Envs:  []EnvConfiguration{},
			Files: []FileConfiguration{},
		},
	}

	if workload == nil {
		return result
	}

	container := workload.Spec.Container

	// Process environment variables from container
	for _, env := range container.Env {
		if env.Value != "" {
			// Direct value - goes to configs
			result.Configs.Envs = append(result.Configs.Envs, EnvConfiguration{
				Name:  env.Key,
				Value: env.Value,
			})
		} else if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
			// Resolve secret reference and add to secrets
			if remoteRef := resolveSecretRef(secretReferences, env.ValueFrom.SecretRef); remoteRef != nil {
				result.Secrets.Envs = append(result.Secrets.Envs, EnvConfiguration{
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
			result.Configs.Files = append(result.Configs.Files, FileConfiguration{
				Name:      file.Key,
				MountPath: file.MountPath,
				Value:     file.Value,
			})
		} else if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
			// Resolve secret reference and add to secrets
			if remoteRef := resolveSecretRef(secretReferences, file.ValueFrom.SecretRef); remoteRef != nil {
				result.Secrets.Files = append(result.Secrets.Files, FileConfiguration{
					Name:      file.Key,
					MountPath: file.MountPath,
					RemoteRef: remoteRef,
				})
			}
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
