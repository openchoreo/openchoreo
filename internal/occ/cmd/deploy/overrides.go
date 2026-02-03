// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploy

import (
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Overrides holds parsed override values from --set flags
type Overrides struct {
	ComponentTypeEnvOverrides map[string]interface{}
	TraitOverrides            map[string]interface{}
	WorkloadOverrides         *gen.WorkloadOverrides
}

// ParseOverrides parses --set flag values into structured overrides
// Format: type.path=value
// Examples:
//   - componentTypeEnvOverrides.replicas=3
//   - traitOverrides.autoscaler.minReplicas=2
//   - workloadOverrides.main.env.LOG_LEVEL=debug
func ParseOverrides(setValues []string) (*Overrides, error) {
	overrides := &Overrides{
		ComponentTypeEnvOverrides: make(map[string]interface{}),
		TraitOverrides:            make(map[string]interface{}),
	}

	workloadContainers := make(map[string]*gen.ContainerOverride)

	for _, setValue := range setValues {
		// Split by = to get key and value
		parts := strings.SplitN(setValue, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --set format '%s', expected format: type.path=value", setValue)
		}

		key := parts[0]
		value := parts[1]

		// Parse the key to determine override type
		keyParts := strings.Split(key, ".")
		if len(keyParts) < 2 {
			return nil, fmt.Errorf("invalid --set key '%s', expected format: type.path=value", key)
		}

		overrideType := keyParts[0]

		switch overrideType {
		case "componentTypeEnvOverrides":
			// componentTypeEnvOverrides.KEY=VALUE
			if len(keyParts) != 2 {
				return nil, fmt.Errorf("invalid componentTypeEnvOverrides format '%s', expected: componentTypeEnvOverrides.KEY=VALUE", key)
			}
			overrides.ComponentTypeEnvOverrides[keyParts[1]] = value

		case "traitOverrides":
			// traitOverrides.INSTANCE.KEY=VALUE
			if len(keyParts) < 3 {
				return nil, fmt.Errorf("invalid traitOverrides format '%s', expected: traitOverrides.INSTANCE.KEY=VALUE", key)
			}
			traitInstance := keyParts[1]
			traitKey := strings.Join(keyParts[2:], ".")

			// Get or create trait instance map
			var traitMap map[string]interface{}
			if existing, ok := overrides.TraitOverrides[traitInstance]; ok {
				traitMap = existing.(map[string]interface{})
			} else {
				traitMap = make(map[string]interface{})
				overrides.TraitOverrides[traitInstance] = traitMap
			}
			traitMap[traitKey] = value

		case "workloadOverrides":
			// workloadOverrides.CONTAINER.env.KEY=VALUE
			// workloadOverrides.CONTAINER.files.KEY.mountPath=VALUE
			if len(keyParts) < 4 {
				return nil, fmt.Errorf("invalid workloadOverrides format '%s', expected: workloadOverrides.CONTAINER.env.KEY=VALUE", key)
			}

			containerName := keyParts[1]
			varType := keyParts[2] // env or files

			// Get or create container override
			container, exists := workloadContainers[containerName]
			if !exists {
				container = &gen.ContainerOverride{}
				workloadContainers[containerName] = container
			}

			switch varType {
			case "env":
				// workloadOverrides.CONTAINER.env.KEY=VALUE
				if len(keyParts) != 4 {
					return nil, fmt.Errorf("invalid workloadOverrides env format '%s', expected: workloadOverrides.CONTAINER.env.KEY=VALUE", key)
				}
				envKey := keyParts[3]

				// Initialize env array if needed
				if container.Env == nil {
					envVars := []gen.EnvVar{}
					container.Env = &envVars
				}

				// Add or update env var
				*container.Env = append(*container.Env, gen.EnvVar{
					Key:   envKey,
					Value: &value,
				})

			case "files":
				// workloadOverrides.CONTAINER.files.KEY.mountPath=VALUE
				if len(keyParts) < 5 {
					return nil, fmt.Errorf("invalid workloadOverrides files format '%s', expected: workloadOverrides.CONTAINER.files.KEY.mountPath=VALUE", key)
				}
				fileKey := keyParts[3]
				fileProperty := keyParts[4] // mountPath or value

				// Initialize files array if needed
				if container.Files == nil {
					fileVars := []gen.FileVar{}
					container.Files = &fileVars
				}

				// Find existing file var or create new one
				var fileVar *gen.FileVar
				for i := range *container.Files {
					if (*container.Files)[i].Key == fileKey {
						fileVar = &(*container.Files)[i]
						break
					}
				}
				if fileVar == nil {
					newFileVar := gen.FileVar{
						Key: fileKey,
					}
					*container.Files = append(*container.Files, newFileVar)
					fileVar = &(*container.Files)[len(*container.Files)-1]
				}

				// Set property
				switch fileProperty {
				case "mountPath":
					fileVar.MountPath = value
				case "value":
					fileVar.Value = &value
				default:
					return nil, fmt.Errorf("unsupported file property '%s', expected 'mountPath' or 'value'", fileProperty)
				}

			default:
				return nil, fmt.Errorf("unsupported workload var type '%s', expected 'env' or 'files'", varType)
			}

		default:
			return nil, fmt.Errorf("unsupported override type '%s', expected 'componentTypeEnvOverrides', 'traitOverrides', or 'workloadOverrides'", overrideType)
		}
	}

	// Convert workload containers map to WorkloadOverrides
	if len(workloadContainers) > 0 {
		// Convert map[string]*gen.ContainerOverride to map[string]gen.ContainerOverride
		containers := make(map[string]gen.ContainerOverride)
		for name, container := range workloadContainers {
			containers[name] = *container
		}
		overrides.WorkloadOverrides = &gen.WorkloadOverrides{
			Containers: &containers,
		}
	}

	return overrides, nil
}
