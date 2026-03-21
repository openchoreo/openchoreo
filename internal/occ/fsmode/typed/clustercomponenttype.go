// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// ClusterComponentType wraps v1alpha1.ClusterComponentType with domain-specific helper methods
type ClusterComponentType struct {
	*v1alpha1.ClusterComponentType
}

// NewClusterComponentType creates a ClusterComponentType wrapper from a ResourceEntry
func NewClusterComponentType(entry *index.ResourceEntry) (*ClusterComponentType, error) {
	cct, err := FromEntry[v1alpha1.ClusterComponentType](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to ClusterComponentType: %w", err)
	}
	return &ClusterComponentType{ClusterComponentType: cct}, nil
}

// GetSchema returns the schema as a map for template processing
func (cct *ClusterComponentType) GetSchema() map[string]interface{} {
	schema := make(map[string]interface{})

	if params := cct.Spec.Parameters.GetRaw(); params != nil {
		schema["parameters"] = map[string]interface{}{
			"openAPIV3Schema": rawExtensionToMap(params),
		}
	}
	if envConfig := cct.Spec.EnvironmentConfigs.GetRaw(); envConfig != nil {
		schema["environmentConfigs"] = map[string]interface{}{
			"openAPIV3Schema": rawExtensionToMap(envConfig),
		}
	}

	return schema
}

// GetResources returns the resource templates as a slice for template processing
func (cct *ClusterComponentType) GetResources() []interface{} {
	if len(cct.Spec.Resources) == 0 {
		return nil
	}

	resources := make([]interface{}, len(cct.Spec.Resources))
	for i, res := range cct.Spec.Resources {
		resourceMap := map[string]interface{}{
			"id": res.ID,
		}

		if res.TargetPlane != "" {
			resourceMap["targetPlane"] = res.TargetPlane
		}
		if res.IncludeWhen != "" {
			resourceMap["includeWhen"] = res.IncludeWhen
		}
		if res.ForEach != "" {
			resourceMap["forEach"] = res.ForEach
		}
		if res.Var != "" {
			resourceMap["var"] = res.Var
		}
		if res.Template != nil {
			resourceMap["template"] = rawExtensionToMap(res.Template)
		}

		resources[i] = resourceMap
	}

	return resources
}

// WorkloadType returns the workload type
func (cct *ClusterComponentType) WorkloadType() string {
	return cct.Spec.WorkloadType
}
