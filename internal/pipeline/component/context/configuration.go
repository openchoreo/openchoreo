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
func extractConfigurationsFromWorkload(secretReferences map[string]*v1alpha1.SecretReference, workload *v1alpha1.Workload) map[string]any {
	result := make(map[string]any)

	// Process all containers in the workload
	if workload != nil && len(workload.Spec.Containers) > 0 {
		for containerName, container := range workload.Spec.Containers {
			configs := map[string][]any{
				"envs":  make([]any, 0),
				"files": make([]any, 0),
			}
			secrets := map[string][]any{
				"envs":  make([]any, 0),
				"files": make([]any, 0),
			}

			// Process environment variables from container
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

			// Process file configurations from container
			for _, file := range container.Files {
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

			// Create the container's configuration structure
			containerResult := make(map[string]any)

			configsResult := make(map[string]any)
			configsResult["envs"] = configs["envs"]
			configsResult["files"] = configs["files"]
			containerResult["configs"] = configsResult

			secretsResult := make(map[string]any)
			secretsResult["envs"] = secrets["envs"]
			secretsResult["files"] = secrets["files"]
			containerResult["secrets"] = secretsResult

			result[containerName] = containerResult
		}
	}

	return result
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
			if dataSource.RemoteRef.Version != "" {
				remoteRef["version"] = dataSource.RemoteRef.Version
			}
			return remoteRef
		}
	}

	return nil
}
