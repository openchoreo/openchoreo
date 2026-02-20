// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// Workload wraps v1alpha1.Workload with domain-specific helper methods
type Workload struct {
	*v1alpha1.Workload
}

// NewWorkload creates a Workload wrapper from a ResourceEntry
func NewWorkload(entry *index.ResourceEntry) (*Workload, error) {
	wl, err := FromEntry[v1alpha1.Workload](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to Workload: %w", err)
	}
	return &Workload{Workload: wl}, nil
}

// GetContainer returns the container definition as a map for template processing
func (w *Workload) GetContainer() map[string]interface{} {
	container := w.Spec.Container
	if container.Image == "" {
		return nil
	}

	containerMap := map[string]interface{}{
		"image": container.Image,
	}

	if len(container.Command) > 0 {
		containerMap["command"] = stringsToInterfaceSlice(container.Command)
	}
	if len(container.Args) > 0 {
		containerMap["args"] = stringsToInterfaceSlice(container.Args)
	}

	// Convert env vars
	if len(container.Env) > 0 {
		env := make([]interface{}, len(container.Env))
		for i, e := range container.Env {
			envMap := map[string]interface{}{
				"key": e.Key,
			}
			if e.Value != "" {
				envMap["value"] = e.Value
			}
			if e.ValueFrom != nil {
				valueFrom := make(map[string]interface{})
				if e.ValueFrom.ConfigurationGroupRef != nil {
					valueFrom["configurationGroupRef"] = map[string]interface{}{
						"name": e.ValueFrom.ConfigurationGroupRef.Name,
						"key":  e.ValueFrom.ConfigurationGroupRef.Key,
					}
				}
				if e.ValueFrom.SecretRef != nil {
					valueFrom["secretRef"] = map[string]interface{}{
						"name": e.ValueFrom.SecretRef.Name,
						"key":  e.ValueFrom.SecretRef.Key,
					}
				}
				envMap["valueFrom"] = valueFrom
			}
			env[i] = envMap
		}
		containerMap["env"] = env
	}

	// Convert files
	if len(container.Files) > 0 {
		files := make([]interface{}, len(container.Files))
		for i, f := range container.Files {
			fileMap := map[string]interface{}{
				"key":       f.Key,
				"mountPath": f.MountPath,
			}
			if f.Value != "" {
				fileMap["value"] = f.Value
			}
			if f.ValueFrom != nil {
				valueFrom := make(map[string]interface{})
				if f.ValueFrom.ConfigurationGroupRef != nil {
					valueFrom["configurationGroupRef"] = map[string]interface{}{
						"name": f.ValueFrom.ConfigurationGroupRef.Name,
						"key":  f.ValueFrom.ConfigurationGroupRef.Key,
					}
				}
				if f.ValueFrom.SecretRef != nil {
					valueFrom["secretRef"] = map[string]interface{}{
						"name": f.ValueFrom.SecretRef.Name,
						"key":  f.ValueFrom.SecretRef.Key,
					}
				}
				fileMap["valueFrom"] = valueFrom
			}
			files[i] = fileMap
		}
		containerMap["files"] = files
	}

	return containerMap
}

// stringsToInterfaceSlice converts []string to []interface{} for JSON compatibility
// This is needed because DeepCopyJSONValue cannot handle []string directly
func stringsToInterfaceSlice(strs []string) []interface{} {
	result := make([]interface{}, len(strs))
	for i, s := range strs {
		result[i] = s
	}
	return result
}
