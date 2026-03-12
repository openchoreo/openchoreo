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
				if e.ValueFrom.SecretKeyRef != nil {
					valueFrom["secretKeyRef"] = map[string]interface{}{
						"name": e.ValueFrom.SecretKeyRef.Name,
						"key":  e.ValueFrom.SecretKeyRef.Key,
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
				if f.ValueFrom.SecretKeyRef != nil {
					valueFrom["secretKeyRef"] = map[string]interface{}{
						"name": f.ValueFrom.SecretKeyRef.Name,
						"key":  f.ValueFrom.SecretKeyRef.Key,
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

// GetEndpoints returns the endpoints definition as a map for template processing.
// The returned map is keyed by endpoint name, matching the WorkloadTemplateSpec.Endpoints structure.
func (w *Workload) GetEndpoints() map[string]interface{} {
	if len(w.Spec.Endpoints) == 0 {
		return nil
	}

	endpointsMap := make(map[string]interface{}, len(w.Spec.Endpoints))
	for name, ep := range w.Spec.Endpoints {
		epMap := map[string]interface{}{
			"type": string(ep.Type),
			"port": int64(ep.Port),
		}
		if ep.TargetPort != 0 {
			epMap["targetPort"] = int64(ep.TargetPort)
		}
		if ep.DisplayName != "" {
			epMap["displayName"] = ep.DisplayName
		}
		if ep.BasePath != "" {
			epMap["basePath"] = ep.BasePath
		}
		if len(ep.Visibility) > 0 {
			vis := make([]interface{}, len(ep.Visibility))
			for i, v := range ep.Visibility {
				vis[i] = string(v)
			}
			epMap["visibility"] = vis
		}
		if ep.Schema != nil {
			schemaMap := make(map[string]interface{})
			if ep.Schema.Type != "" {
				schemaMap["type"] = ep.Schema.Type
			}
			if ep.Schema.Content != "" {
				schemaMap["content"] = ep.Schema.Content
			}
			epMap["schema"] = schemaMap
		}
		endpointsMap[name] = epMap
	}
	return endpointsMap
}

// GetConnections returns the connections definition as a slice for template processing.
func (w *Workload) GetConnections() []interface{} {
	deps := w.Spec.GetDependencyEndpoints()
	if len(deps) == 0 {
		return nil
	}

	connections := make([]interface{}, len(deps))
	for i, conn := range deps {
		connMap := map[string]interface{}{
			"component":  conn.Component,
			"name":       conn.Name,
			"visibility": string(conn.Visibility),
		}
		if conn.Project != "" {
			connMap["project"] = conn.Project
		}

		envBindings := map[string]interface{}{}
		if conn.EnvBindings.Address != "" {
			envBindings["address"] = conn.EnvBindings.Address
		}
		if conn.EnvBindings.Host != "" {
			envBindings["host"] = conn.EnvBindings.Host
		}
		if conn.EnvBindings.Port != "" {
			envBindings["port"] = conn.EnvBindings.Port
		}
		if conn.EnvBindings.BasePath != "" {
			envBindings["basePath"] = conn.EnvBindings.BasePath
		}
		connMap["envBindings"] = envBindings

		connections[i] = connMap
	}
	return connections
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
