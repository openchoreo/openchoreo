// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/parser"

	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

const (
	configurationsIdentifier = "configurations"
)

// CELExtensions returns CEL environment options for configuration helpers.
// These include:
//   - Macro: configurations.toConfigFileList(prefix) -> configurationsToConfigFileList(configurations, prefix)
//   - Function: configurationsToConfigFileList
//   - Macro: configurations.toSecretFileList(prefix) -> configurationsToSecretFileList(configurations, prefix)
//   - Function: configurationsToSecretFileList
//   - Macro: configurations.toContainerEnvFrom(containerName, prefix) -> configurationsToContainerEnvFrom(configurations, containerName, prefix)
//   - Function: configurationsToContainerEnvFrom
//   - Macro: configurations.toContainerVolumeMounts(containerName) -> configurationsToContainerVolumeMounts(configurations, containerName)
//   - Function: configurationsToContainerVolumeMounts
//   - Macro: configurations.toVolumes(prefix) -> configurationsToVolumes(configurations, prefix)
//   - Function: configurationsToVolumes
//   - Macro: configurations.toConfigEnvList(prefix) -> configurationsToConfigEnvList(configurations, prefix)
//   - Function: configurationsToConfigEnvList
//   - Macro: configurations.toSecretEnvList(prefix) -> configurationsToSecretEnvList(configurations, prefix)
//   - Function: configurationsToSecretEnvList
func CELExtensions() []cel.EnvOption {
	return []cel.EnvOption{
		// Register the macros
		cel.Macros(toConfigFileListMacro, toSecretFileListMacro, toContainerEnvFromMacro, toContainerVolumeMountsMacro, toVolumesMacro, toConfigEnvListMacro, toSecretEnvListMacro),
		// Register the functions
		cel.Function("configurationsToConfigFileList",
			cel.Overload("configurationsToConfigFileList_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToConfigFileListFunction),
			),
		),
		cel.Function("configurationsToSecretFileList",
			cel.Overload("configurationsToSecretFileList_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToSecretFileListFunction),
			),
		),
		cel.Function("configurationsToContainerEnvFrom",
			cel.Overload("configurationsToContainerEnvFrom_dyn_string_string",
				[]*cel.Type{cel.DynType, cel.StringType, cel.StringType}, cel.ListType(cel.DynType),
				cel.FunctionBinding(func(vals ...ref.Val) ref.Val {
					return configurationsToContainerEnvFromFunction(vals[0], vals[1], vals[2])
				}),
			),
		),
		cel.Function("configurationsToContainerVolumeMounts",
			cel.Overload("configurationsToContainerVolumeMounts_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToContainerVolumeMountsFunction),
			),
		),
		cel.Function("configurationsToVolumes",
			cel.Overload("configurationsToVolumes_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToVolumesFunction),
			),
		),
		cel.Function("configurationsToConfigEnvList",
			cel.Overload("configurationsToConfigEnvList_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToConfigEnvListFunction),
			),
		),
		cel.Function("configurationsToSecretEnvList",
			cel.Overload("configurationsToSecretEnvList_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToSecretEnvListFunction),
			),
		),
	}
}

// toConfigFileListMacro transforms configurations.toConfigFileList(prefix) into
// configurationsToConfigFileList(configurations, prefix) at compile time.
var toConfigFileListMacro = cel.ReceiverMacro("toConfigFileList", 1,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return eh.NewCall("configurationsToConfigFileList", target, args[0]), nil
		}
		return nil, nil
	})

// toSecretFileListMacro transforms configurations.toSecretFileList(prefix) into
// configurationsToSecretFileList(configurations, prefix) at compile time.
var toSecretFileListMacro = cel.ReceiverMacro("toSecretFileList", 1,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return eh.NewCall("configurationsToSecretFileList", target, args[0]), nil
		}
		return nil, nil
	})

// toContainerEnvFromMacro transforms configurations.toContainerEnvFrom(containerName, prefix) into
// configurationsToContainerEnvFrom(configurations, containerName, prefix) at compile time.
var toContainerEnvFromMacro = cel.ReceiverMacro("toContainerEnvFrom", 2,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return eh.NewCall("configurationsToContainerEnvFrom", target, args[0], args[1]), nil
		}
		return nil, nil
	})

// toContainerVolumeMountsMacro transforms configurations.toContainerVolumeMounts(containerName) into
// configurationsToContainerVolumeMounts(configurations, containerName) at compile time.
var toContainerVolumeMountsMacro = cel.ReceiverMacro("toContainerVolumeMounts", 1,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return eh.NewCall("configurationsToContainerVolumeMounts", target, args[0]), nil
		}
		return nil, nil
	})

// toVolumesMacro transforms configurations.toVolumes(prefix) into
// configurationsToVolumes(configurations, prefix) at compile time.
var toVolumesMacro = cel.ReceiverMacro("toVolumes", 1,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return eh.NewCall("configurationsToVolumes", target, args[0]), nil
		}
		return nil, nil
	})

// toConfigEnvListMacro transforms configurations.toConfigEnvList(prefix) into
// configurationsToConfigEnvList(configurations, prefix) at compile time.
var toConfigEnvListMacro = cel.ReceiverMacro("toConfigEnvList", 1,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return eh.NewCall("configurationsToConfigEnvList", target, args[0]), nil
		}
		return nil, nil
	})

// toSecretEnvListMacro transforms configurations.toSecretEnvList(prefix) into
// configurationsToSecretEnvList(configurations, prefix) at compile time.
var toSecretEnvListMacro = cel.ReceiverMacro("toSecretEnvList", 1,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return eh.NewCall("configurationsToSecretEnvList", target, args[0]), nil
		}
		return nil, nil
	})

// configurationsToConfigFileListFunction is the CEL binding for configurations.toConfigFileList(prefix).
// Returns a list of maps, each containing: name, mountPath, value, resourceName, and optionally remoteRef.
func configurationsToConfigFileListFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toConfigFileList: prefix must be a string")
	}

	// configurations is a map[string]any after JSON marshaling in ComponentContext.ToMap()
	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toConfigFileList: expected map[string]any, got %T", configurations.Value())
	}
	result := makeConfigFileList(configMap, prefixStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// configurationsToSecretFileListFunction is the CEL binding for configurations.toSecretFileList(prefix).
// Returns a list of maps, each containing: name, mountPath, value, resourceName, and optionally remoteRef.
func configurationsToSecretFileListFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toSecretFileList: prefix must be a string")
	}

	// configurations is a map[string]any after JSON marshaling in ComponentContext.ToMap()
	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toSecretFileList: expected map[string]any, got %T", configurations.Value())
	}
	result := makeSecretFileList(configMap, prefixStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// configurationsToContainerEnvFromFunction is the CEL binding for configurations.toContainerEnvFrom(containerName, prefix).
// Returns a list of envFrom entries (configMapRef and/or secretRef) based on container config.
func configurationsToContainerEnvFromFunction(configurations, containerName, prefix ref.Val) ref.Val {
	containerNameStr, ok := containerName.Value().(string)
	if !ok {
		return types.NewErr("toContainerEnvFrom: containerName must be a string")
	}

	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toContainerEnvFrom: prefix must be a string")
	}

	// configurations is a map[string]any after JSON marshaling in ComponentContext.ToMap()
	configurationsMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toContainerEnvFrom: expected map[string]any, got %T", configurations.Value())
	}

	// Get the specific container configuration
	containerConfig, ok := configurationsMap[containerNameStr].(map[string]any)
	if !ok {
		// Return empty list if container not found
		return types.DefaultTypeAdapter.NativeToValue([]map[string]any{})
	}

	result := makeEnvFromList(containerConfig, prefixStr, containerNameStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// makeConfigFileList flattens configs.files from all containers and returns a list of maps.
// Each map contains: name, mountPath, value, resourceName, and optionally remoteRef.
func makeConfigFileList(configMap map[string]any, prefix string) []map[string]any {
	if len(configMap) == 0 {
		return []map[string]any{}
	}

	result := make([]map[string]any, 0)

	for containerName, containerVal := range configMap {
		container, ok := containerVal.(map[string]any)
		if !ok {
			continue
		}
		configs, ok := container["configs"].(map[string]any)
		if !ok {
			continue
		}
		files, ok := configs["files"].([]any)
		if !ok {
			continue
		}

		for _, fileVal := range files {
			file, ok := fileVal.(map[string]any)
			if !ok {
				continue
			}

			name, _ := file["name"].(string)
			mountPath, _ := file["mountPath"].(string)
			value, _ := file["value"].(string)

			entry := map[string]any{
				"name":         name,
				"mountPath":    mountPath,
				"value":        value,
				"resourceName": generateConfigResourceName(prefix, containerName, name),
			}
			if remoteRef, ok := file["remoteRef"].(map[string]any); ok {
				entry["remoteRef"] = remoteRef
			}
			result = append(result, entry)
		}
	}
	return result
}

// makeSecretFileList flattens secrets.files from all containers and returns a list of maps.
// Each map contains: name, mountPath, value, resourceName, and optionally remoteRef.
func makeSecretFileList(configMap map[string]any, prefix string) []map[string]any {
	if len(configMap) == 0 {
		return []map[string]any{}
	}

	result := make([]map[string]any, 0)

	for containerName, containerVal := range configMap {
		container, ok := containerVal.(map[string]any)
		if !ok {
			continue
		}
		secrets, ok := container["secrets"].(map[string]any)
		if !ok {
			continue
		}
		files, ok := secrets["files"].([]any)
		if !ok {
			continue
		}

		for _, fileVal := range files {
			file, ok := fileVal.(map[string]any)
			if !ok {
				continue
			}

			name, _ := file["name"].(string)
			mountPath, _ := file["mountPath"].(string)
			value, _ := file["value"].(string)

			entry := map[string]any{
				"name":         name,
				"mountPath":    mountPath,
				"value":        value,
				"resourceName": generateSecretResourceName(prefix, containerName, name),
			}
			if remoteRef, ok := file["remoteRef"].(map[string]any); ok {
				entry["remoteRef"] = remoteRef
			}
			result = append(result, entry)
		}
	}
	return result
}

// generateConfigResourceName generates a Kubernetes-compliant resource name for a config file.
func generateConfigResourceName(prefix, container, filename string) string {
	return kubernetes.GenerateK8sNameWithLengthLimit(
		kubernetes.MaxResourceNameLength,
		prefix,
		container,
		"config",
		strings.ReplaceAll(filename, ".", "-"),
	)
}

// configurationsToContainerVolumeMountsFunction is the CEL binding for configurations.toContainerVolumeMounts(containerName).
// Returns a list of volumeMount entries based on a specific container's config files.
func configurationsToContainerVolumeMountsFunction(configurations, containerName ref.Val) ref.Val {
	containerNameStr, ok := containerName.Value().(string)
	if !ok {
		return types.NewErr("toContainerVolumeMounts: containerName must be a string")
	}

	// configurations is a map[string]any after JSON marshaling in ComponentContext.ToMap()
	configurationsMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toContainerVolumeMounts: expected map[string]any, got %T", configurations.Value())
	}

	// Get the specific container configuration
	containerConfig, ok := configurationsMap[containerNameStr].(map[string]any)
	if !ok {
		// Return empty list if container not found
		return types.DefaultTypeAdapter.NativeToValue([]map[string]any{})
	}

	result := makeVolumeMountsList(containerConfig, containerNameStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// configurationsToVolumesFunction is the CEL binding for configurations.toVolumes(prefix).
// Returns a list of volume entries for all containers' files.
func configurationsToVolumesFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toVolumes: prefix must be a string")
	}

	// configurations is a map[string]any after JSON marshaling in ComponentContext.ToMap()
	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toVolumes: expected map[string]any, got %T", configurations.Value())
	}
	result := makeVolumesList(configMap, prefixStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// makeEnvFromList generates envFrom entries for a single container configuration.
// Returns a list containing configMapRef and/or secretRef based on what envs are available.
func makeEnvFromList(containerConfig map[string]any, prefix, containerName string) []map[string]any {
	if len(containerConfig) == 0 {
		return []map[string]any{}
	}

	result := make([]map[string]any, 0)

	// Check for config environment variables
	if configs, ok := containerConfig["configs"].(map[string]any); ok {
		if envs, ok := configs["envs"].([]any); ok && len(envs) > 0 {
			configMapName := kubernetes.GenerateK8sNameWithLengthLimit(
				kubernetes.MaxResourceNameLength,
				prefix,
				containerName,
				"env-configs",
			)
			result = append(result, map[string]any{
				"configMapRef": map[string]any{
					"name": configMapName,
				},
			})
		}
	}

	// Check for secret environment variables
	if secrets, ok := containerConfig["secrets"].(map[string]any); ok {
		if envs, ok := secrets["envs"].([]any); ok && len(envs) > 0 {
			secretName := kubernetes.GenerateK8sNameWithLengthLimit(
				kubernetes.MaxResourceNameLength,
				prefix,
				containerName,
				"env-secrets",
			)
			result = append(result, map[string]any{
				"secretRef": map[string]any{
					"name": secretName,
				},
			})
		}
	}

	return result
}

// generateSecretResourceName generates a Kubernetes-compliant resource name for a secret file.
func generateSecretResourceName(prefix, container, filename string) string {
	return kubernetes.GenerateK8sNameWithLengthLimit(
		kubernetes.MaxResourceNameLength,
		prefix,
		container,
		"secret",
		strings.ReplaceAll(filename, ".", "-"),
	)
}

// makeVolumeMountsList generates volumeMount entries for a single container configuration.
// Returns a list containing volumeMount entries for all files (both config and secret files).
func makeVolumeMountsList(containerConfig map[string]any, containerName string) []map[string]any {
	if len(containerConfig) == 0 {
		return []map[string]any{}
	}

	result := make([]map[string]any, 0)

	// Process config files
	if configs, ok := containerConfig["configs"].(map[string]any); ok {
		if files, ok := configs["files"].([]any); ok {
			for _, fileVal := range files {
				if file, ok := fileVal.(map[string]any); ok {
					name, _ := file["name"].(string)
					mountPath, _ := file["mountPath"].(string)

					// Generate hash for volume name using the same pattern as the template
					volumeName := containerName + "-file-mount-" + generateVolumeHash(mountPath, name)

					result = append(result, map[string]any{
						"name":      volumeName,
						"mountPath": mountPath + "/" + name,
						"subPath":   name,
					})
				}
			}
		}
	}

	// Process secret files
	if secrets, ok := containerConfig["secrets"].(map[string]any); ok {
		if files, ok := secrets["files"].([]any); ok {
			for _, fileVal := range files {
				if file, ok := fileVal.(map[string]any); ok {
					name, _ := file["name"].(string)
					mountPath, _ := file["mountPath"].(string)

					// Generate hash for volume name using the same pattern as the template
					volumeName := containerName + "-file-mount-" + generateVolumeHash(mountPath, name)

					result = append(result, map[string]any{
						"name":      volumeName,
						"mountPath": mountPath + "/" + name,
						"subPath":   name,
					})
				}
			}
		}
	}

	return result
}

// makeVolumesList generates volume entries for all containers' files.
// Returns a list containing volume definitions for all files (both config and secret files) across all containers.
func makeVolumesList(configMap map[string]any, prefix string) []map[string]any {
	if len(configMap) == 0 {
		return []map[string]any{}
	}

	result := make([]map[string]any, 0)

	// Track volumes by name to avoid duplicates
	volumes := make(map[string]map[string]any)

	for containerName, containerVal := range configMap {
		container, ok := containerVal.(map[string]any)
		if !ok {
			continue
		}

		// Process config files
		if configs, ok := container["configs"].(map[string]any); ok {
			if files, ok := configs["files"].([]any); ok {
				for _, fileVal := range files {
					if file, ok := fileVal.(map[string]any); ok {
						name, _ := file["name"].(string)
						mountPath, _ := file["mountPath"].(string)

						// Generate hash for volume name and resource name
						volumeName := containerName + "-file-mount-" + generateVolumeHash(mountPath, name)
						configMapName := generateConfigResourceName(prefix, containerName, name)

						volumes[volumeName] = map[string]any{
							"name": volumeName,
							"configMap": map[string]any{
								"name": configMapName,
							},
						}
					}
				}
			}
		}

		// Process secret files
		if secrets, ok := container["secrets"].(map[string]any); ok {
			if files, ok := secrets["files"].([]any); ok {
				for _, fileVal := range files {
					if file, ok := fileVal.(map[string]any); ok {
						name, _ := file["name"].(string)
						mountPath, _ := file["mountPath"].(string)

						// Generate hash for volume name and resource name
						volumeName := containerName + "-file-mount-" + generateVolumeHash(mountPath, name)
						secretName := generateSecretResourceName(prefix, containerName, name)

						volumes[volumeName] = map[string]any{
							"name": volumeName,
							"secret": map[string]any{
								"secretName": secretName,
							},
						}
					}
				}
			}
		}
	}

	// Convert map to slice
	for _, volume := range volumes {
		result = append(result, volume)
	}

	return result
}

// configurationsToConfigEnvListFunction is the CEL binding for configurations.toConfigEnvList(prefix).
// Returns a list of objects with container, resourceName, and envs for each container with config envs.
func configurationsToConfigEnvListFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toConfigEnvList: prefix must be a string")
	}

	// configurations is a map[string]any after JSON marshaling in ComponentContext.ToMap()
	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toConfigEnvList: expected map[string]any, got %T", configurations.Value())
	}

	result := makeConfigEnvList(configMap, prefixStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// makeConfigEnvList creates a list of objects with container, resourceName, and envs.
// Each object represents a container that has config envs.
func makeConfigEnvList(configMap map[string]any, prefix string) []map[string]any {
	if len(configMap) == 0 {
		return []map[string]any{}
	}

	result := make([]map[string]any, 0)

	for containerName, containerVal := range configMap {
		container, ok := containerVal.(map[string]any)
		if !ok {
			continue
		}

		configs, ok := container["configs"].(map[string]any)
		if !ok {
			continue
		}

		envs, ok := configs["envs"].([]any)
		if !ok || len(envs) == 0 {
			continue
		}

		// Generate resource name for this container
		// Use kubernetes.GenerateK8sNameWithLengthLimit directly to avoid the extra "config" in the name
		resourceName := kubernetes.GenerateK8sNameWithLengthLimit(
			kubernetes.MaxResourceNameLength,
			prefix,
			containerName,
			"env-configs",
		)

		entry := map[string]any{
			"container":    containerName,
			"resourceName": resourceName,
			"envs":         envs,
		}
		result = append(result, entry)
	}

	return result
}

// configurationsToSecretEnvListFunction is the CEL binding for configurations.toSecretEnvList(prefix).
// Returns a list of objects with container, resourceName, and envs for each container with secret envs.
func configurationsToSecretEnvListFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toSecretEnvList: prefix must be a string")
	}

	// configurations is a map[string]any after JSON marshaling in ComponentContext.ToMap()
	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toSecretEnvList: expected map[string]any, got %T", configurations.Value())
	}

	result := makeSecretEnvList(configMap, prefixStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// makeSecretEnvList creates a list of objects with container, resourceName, and envs.
// Each object represents a container that has secret envs.
func makeSecretEnvList(configMap map[string]any, prefix string) []map[string]any {
	if len(configMap) == 0 {
		return []map[string]any{}
	}

	result := make([]map[string]any, 0)

	for containerName, containerVal := range configMap {
		container, ok := containerVal.(map[string]any)
		if !ok {
			continue
		}

		secrets, ok := container["secrets"].(map[string]any)
		if !ok {
			continue
		}

		envs, ok := secrets["envs"].([]any)
		if !ok || len(envs) == 0 {
			continue
		}

		// Generate resource name for this container
		// Use kubernetes.GenerateK8sNameWithLengthLimit directly for consistency
		resourceName := kubernetes.GenerateK8sNameWithLengthLimit(
			kubernetes.MaxResourceNameLength,
			prefix,
			containerName,
			"env-secrets",
		)

		entry := map[string]any{
			"container":    containerName,
			"resourceName": resourceName,
			"envs":         envs,
		}
		result = append(result, entry)
	}

	return result
}

// generateVolumeHash generates a hash for volume names to match the template pattern.
// This uses the same hash function as the oc_hash CEL function (FNV-1a 32-bit).
func generateVolumeHash(mountPath, filename string) string {
	input := mountPath + "/" + filename
	h := fnv.New32a()
	h.Write([]byte(input))
	return fmt.Sprintf("%08x", h.Sum32())
}
