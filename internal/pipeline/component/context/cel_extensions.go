// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
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
	protocolTCP              = "TCP"
	protocolUDP              = "UDP"
)

// CELExtensions returns CEL environment options for configuration helpers.
// These include:
//   - Macro: configurations.toConfigFileList() -> configurationsToConfigFileList(configurations, prefix)
//   - Function: configurationsToConfigFileList
//   - Macro: configurations.toSecretFileList() -> configurationsToSecretFileList(configurations, prefix)
//   - Function: configurationsToSecretFileList
//   - Macro: configurations.toContainerEnvFrom(containerName) -> configurationsToContainerEnvFrom(configurations, containerName, prefix)
//   - Function: configurationsToContainerEnvFrom
//   - Macro: configurations.toContainerVolumeMounts(containerName) -> configurationsToContainerVolumeMounts(configurations, containerName)
//   - Function: configurationsToContainerVolumeMounts
//   - Macro: configurations.toVolumes() -> configurationsToVolumes(configurations, prefix)
//   - Function: configurationsToVolumes
//   - Macro: configurations.toConfigEnvsByContainer() -> configurationsToConfigEnvsByContainer(configurations, prefix)
//   - Function: configurationsToConfigEnvsByContainer
//   - Macro: configurations.toSecretEnvsByContainer() -> configurationsToSecretEnvsByContainer(configurations, prefix)
//   - Function: configurationsToSecretEnvsByContainer
//   - Macro: workload.toServicePorts() -> workloadToServicePorts(workload)
//   - Function: workloadToServicePorts
//
// Where prefix = metadata.componentName + "-" + metadata.environmentName (automatically injected by macros)
func CELExtensions() []cel.EnvOption {
	return []cel.EnvOption{
		// Register the macros
		cel.Macros(toConfigFileListMacro, toSecretFileListMacro, toContainerEnvFromMacro, toContainerVolumeMountsMacro, toVolumesMacro, toConfigEnvsByContainerMacro, toSecretEnvsByContainerMacro, toServicePortsMacro),
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
		cel.Function("configurationsToConfigEnvsByContainer",
			cel.Overload("configurationsToConfigEnvsByContainer_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToConfigEnvsByContainerFunction),
			),
		),
		cel.Function("configurationsToSecretEnvsByContainer",
			cel.Overload("configurationsToSecretEnvsByContainer_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToSecretEnvsByContainerFunction),
			),
		),
		cel.Function("workloadToServicePorts",
			cel.Overload("workloadToServicePorts_dyn",
				[]*cel.Type{cel.DynType}, cel.ListType(cel.DynType),
				cel.UnaryBinding(workloadToServicePortsFunction),
			),
		),
	}
}

// buildPrefixExpr creates an AST expression for: metadata.componentName + "-" + metadata.environmentName
func buildPrefixExpr(eh parser.ExprHelper) ast.Expr {
	componentNameExpr := eh.NewSelect(eh.NewIdent("metadata"), "componentName")
	environmentNameExpr := eh.NewSelect(eh.NewIdent("metadata"), "environmentName")
	componentWithDash := eh.NewCall("_+_", componentNameExpr, eh.NewLiteral(types.String("-")))
	// metadata.componentName + "-" + metadata.environmentName
	return eh.NewCall("_+_", componentWithDash, environmentNameExpr)
}

// toConfigFileListMacro transforms configurations.toConfigFileList() into
// configurationsToConfigFileList(configurations, prefix) at compile time.
var toConfigFileListMacro = cel.ReceiverMacro("toConfigFileList", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			prefixExpr := buildPrefixExpr(eh)
			return eh.NewCall("configurationsToConfigFileList", target, prefixExpr), nil
		}
		return nil, nil
	})

// toSecretFileListMacro transforms configurations.toSecretFileList() into
// configurationsToSecretFileList(configurations, prefix) at compile time.
var toSecretFileListMacro = cel.ReceiverMacro("toSecretFileList", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			prefixExpr := buildPrefixExpr(eh)
			return eh.NewCall("configurationsToSecretFileList", target, prefixExpr), nil
		}
		return nil, nil
	})

// toContainerEnvFromMacro transforms configurations.toContainerEnvFrom(containerName) into
// configurationsToContainerEnvFrom(configurations, containerName, prefix) at compile time.
var toContainerEnvFromMacro = cel.ReceiverMacro("toContainerEnvFrom", 1,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			prefixExpr := buildPrefixExpr(eh)
			return eh.NewCall("configurationsToContainerEnvFrom", target, args[0], prefixExpr), nil
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

// toVolumesMacro transforms configurations.toVolumes() into
// configurationsToVolumes(configurations, prefix) at compile time.
var toVolumesMacro = cel.ReceiverMacro("toVolumes", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			prefixExpr := buildPrefixExpr(eh)
			return eh.NewCall("configurationsToVolumes", target, prefixExpr), nil
		}
		return nil, nil
	})

// toConfigEnvsByContainerMacro transforms configurations.toConfigEnvsByContainer() into
// configurationsToConfigEnvsByContainer(configurations, prefix) at compile time.
var toConfigEnvsByContainerMacro = cel.ReceiverMacro("toConfigEnvsByContainer", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			prefixExpr := buildPrefixExpr(eh)
			return eh.NewCall("configurationsToConfigEnvsByContainer", target, prefixExpr), nil
		}
		return nil, nil
	})

// toSecretEnvsByContainerMacro transforms configurations.toSecretEnvsByContainer() into
// configurationsToSecretEnvsByContainer(configurations, prefix) at compile time.
var toSecretEnvsByContainerMacro = cel.ReceiverMacro("toSecretEnvsByContainer", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			prefixExpr := buildPrefixExpr(eh)
			return eh.NewCall("configurationsToSecretEnvsByContainer", target, prefixExpr), nil
		}
		return nil, nil
	})

// toServicePortsMacro transforms workload.toServicePorts() into
// workloadToServicePorts(workload) at compile time.
var toServicePortsMacro = cel.ReceiverMacro("toServicePorts", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		// Check if target is workload
		if target.Kind() == ast.IdentKind && target.AsIdent() == "workload" {
			return eh.NewCall("workloadToServicePorts", target), nil
		}
		return nil, nil
	})

// configurationsToConfigFileListFunction is the CEL binding for configurations.toConfigFileList().
// The macro automatically injects prefix (metadata.componentName + "-" + metadata.environmentName) as the prefix parameter.
// Returns a list of maps, each containing: name, mountPath, value, resourceName, and optionally remoteRef.
func configurationsToConfigFileListFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toConfigFileList: prefix must be a string")
	}

	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toConfigFileList: expected map[string]any, got %T", configurations.Value())
	}
	result := makeConfigFileList(configMap, prefixStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// configurationsToSecretFileListFunction is the CEL binding for configurations.toSecretFileList().
// The macro automatically injects prefix (metadata.componentName + "-" + metadata.environmentName) as the prefix parameter.
// Returns a list of maps, each containing: name, mountPath, resourceName, and remoteRef.
// Note: Secret files do not have inline values, only remoteRef for external secret references.
func configurationsToSecretFileListFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toSecretFileList: prefix must be a string")
	}

	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toSecretFileList: expected map[string]any, got %T", configurations.Value())
	}
	result := makeSecretFileList(configMap, prefixStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// configurationsToContainerEnvFromFunction is the CEL binding for configurations.toContainerEnvFrom(containerName).
// The macro automatically injects prefix (metadata.componentName + "-" + metadata.environmentName) as the third parameter.
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

	configurationsMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toContainerEnvFrom: expected map[string]any, got %T", configurations.Value())
	}

	containerConfig, exists := configurationsMap[containerNameStr]
	if !exists {
		return types.NewErr("toContainerEnvFrom: container '%s' not found in configurations", containerNameStr)
	}

	containerConfigMap, ok := containerConfig.(map[string]any)
	if !ok {
		return types.NewErr("toContainerEnvFrom: expected map[string]any for container '%s', got %T", containerNameStr, containerConfig)
	}

	result := makeEnvFromList(containerConfigMap, prefixStr, containerNameStr)
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

			entry := map[string]any{
				"name":         name,
				"mountPath":    mountPath,
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

	configurationsMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toContainerVolumeMounts: expected map[string]any, got %T", configurations.Value())
	}

	containerConfig, exists := configurationsMap[containerNameStr]
	if !exists {
		return types.NewErr("toContainerVolumeMounts: container '%s' not found in configurations", containerNameStr)
	}

	containerConfigMap, ok := containerConfig.(map[string]any)
	if !ok {
		return types.NewErr("toContainerVolumeMounts: expected map[string]any for container '%s', got %T", containerNameStr, containerConfig)
	}

	result := makeVolumeMountsList(containerConfigMap, containerNameStr)
	return types.DefaultTypeAdapter.NativeToValue(result)
}

// configurationsToVolumesFunction is the CEL binding for configurations.toVolumes().
// The macro automatically injects prefix (metadata.componentName + "-" + metadata.environmentName) as the prefix parameter.
// Returns a list of volume entries for all containers' files.
func configurationsToVolumesFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toVolumes: prefix must be a string")
	}

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
			configMapName := generateEnvResourceName(prefix, containerName, "env-configs")
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
			secretName := generateEnvResourceName(prefix, containerName, "env-secrets")
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

	for _, volume := range volumes {
		result = append(result, volume)
	}

	return result
}

// configurationsToConfigEnvsByContainerFunction is the CEL binding for configurations.toConfigEnvsByContainer().
// The macro automatically injects prefix (metadata.componentName + "-" + metadata.environmentName) as the prefix parameter.
// Returns a list of objects with container, resourceName, and envs for each container with config envs.
func configurationsToConfigEnvsByContainerFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toConfigEnvsByContainer: prefix must be a string")
	}

	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toConfigEnvsByContainer: expected map[string]any, got %T", configurations.Value())
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

		resourceName := generateEnvResourceName(prefix, containerName, "env-configs")

		entry := map[string]any{
			"container":    containerName,
			"resourceName": resourceName,
			"envs":         envs,
		}
		result = append(result, entry)
	}

	return result
}

// configurationsToSecretEnvsByContainerFunction is the CEL binding for configurations.toSecretEnvsByContainer().
// The macro automatically injects prefix (metadata.componentName + "-" + metadata.environmentName) as the prefix parameter.
// Returns a list of objects with container, resourceName, and envs for each container with secret envs.
func configurationsToSecretEnvsByContainerFunction(configurations, prefix ref.Val) ref.Val {
	prefixStr, ok := prefix.Value().(string)
	if !ok {
		return types.NewErr("toSecretEnvsByContainer: prefix must be a string")
	}

	configMap, ok := configurations.Value().(map[string]any)
	if !ok {
		return types.NewErr("toSecretEnvsByContainer: expected map[string]any, got %T", configurations.Value())
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

		resourceName := generateEnvResourceName(prefix, containerName, "env-secrets")

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

func generateEnvResourceName(prefix, container, suffix string) string {
	return kubernetes.GenerateK8sNameWithLengthLimit(
		kubernetes.MaxResourceNameLength,
		prefix,
		container,
		suffix,
	)
}

// workloadToServicePortsFunction is the CEL binding for workload.toServicePorts().
// Returns a list of Service port definitions, each containing: name, port, targetPort, protocol.
func workloadToServicePortsFunction(workload ref.Val) ref.Val {
	workloadMap, ok := workload.Value().(map[string]any)
	if !ok {
		return types.NewErr("toServicePorts: expected workload to be a map, got %T", workload.Value())
	}

	// Extract endpoints from workload
	endpointsVal, hasEndpoints := workloadMap["endpoints"]
	if !hasEndpoints {
		return types.DefaultTypeAdapter.NativeToValue([]map[string]any{})
	}

	endpointsMap, ok := endpointsVal.(map[string]any)
	if !ok {
		return types.NewErr("toServicePorts: workload.endpoints must be a map, got %T", endpointsVal)
	}

	if len(endpointsMap) == 0 {
		return types.DefaultTypeAdapter.NativeToValue([]map[string]any{})
	}

	// Sort endpoint names for deterministic output
	endpointNames := make([]string, 0, len(endpointsMap))
	for endpointName := range endpointsMap {
		endpointNames = append(endpointNames, endpointName)
	}
	sort.Strings(endpointNames)

	result := make([]map[string]any, 0, len(endpointsMap))
	usedNames := make(map[string]bool)

	for _, endpointName := range endpointNames {
		endpointVal := endpointsMap[endpointName]
		endpoint, ok := endpointVal.(map[string]any)
		if !ok {
			return types.NewErr("toServicePorts: endpoint '%s' must be an object, got %T", endpointName, endpointVal)
		}

		portVal := endpoint["port"]
		if portVal == nil {
			return types.NewErr("toServicePorts: endpoint '%s' is missing required 'port' field", endpointName)
		}
		var port int64
		switch p := portVal.(type) {
		case int64:
			port = p
		case float64:
			if math.Trunc(p) != p {
				return types.NewErr("toServicePorts: endpoint '%s' port must be an integer, got %v", endpointName, p)
			}
			port = int64(p)
		case int:
			port = int64(p)
		case int32:
			port = int64(p)
		default:
			return types.NewErr("toServicePorts: endpoint '%s' must have a numeric port, got %T", endpointName, portVal)
		}

		endpointType, _ := endpoint["type"].(string)
		protocol := mapEndpointTypeToProtocol(endpointType)

		// Sanitize endpoint name for Kubernetes port naming
		sanitizedName := sanitizePortName(endpointName)

		// If sanitization resulted in empty string, use port number as fallback
		if sanitizedName == "" {
			sanitizedName = fmt.Sprintf("port-%d", port)
		}

		// Ensure uniqueness by adding suffix if needed
		finalName := sanitizedName
		counter := 2
		for usedNames[finalName] {
			finalName = fmt.Sprintf("%s-%d", sanitizedName, counter)
			// Ensure the final name doesn't exceed 15 characters
			if len(finalName) > 15 {
				// Truncate the base name to make room for suffix
				maxBaseLen := 15 - len(fmt.Sprintf("-%d", counter))
				if maxBaseLen > 0 {
					finalName = fmt.Sprintf("%s-%d", sanitizedName[:maxBaseLen], counter)
				} else {
					// If counter is too large, use minimal name
					finalName = fmt.Sprintf("p-%d", counter)
				}
			}
			counter++
		}
		usedNames[finalName] = true

		result = append(result, map[string]any{
			"name":       finalName,
			"port":       port, // External port uses endpoint port
			"targetPort": port, // Target port uses endpoint port
			"protocol":   protocol,
		})
	}

	return types.DefaultTypeAdapter.NativeToValue(result)
}

// mapEndpointTypeToProtocol maps WorkloadEndpoint.Type to Kubernetes Service protocol.
func mapEndpointTypeToProtocol(endpointType string) string {
	switch endpointType {
	case protocolTCP:
		return protocolTCP
	case protocolUDP:
		return protocolUDP
	default:
		// HTTP, REST, gRPC, GraphQL, Websocket all use TCP
		return protocolTCP
	}
}

// sanitizePortName sanitizes endpoint names for use as Kubernetes port names.
// Kubernetes port names must be:
// - lowercase alphanumeric + hyphens
// - start and end with alphanumeric (not hyphen)
// - max 15 characters (IANA service name limit)
// Returns empty string if name cannot be sanitized.
func sanitizePortName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)
	// Replace underscores with hyphens
	name = strings.ReplaceAll(name, "_", "-")

	// Remove invalid characters (keep only alphanumeric and hyphens)
	var result strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			result.WriteRune(ch)
		}
	}
	sanitized := result.String()

	// Trim leading and trailing hyphens (Kubernetes requirement)
	sanitized = strings.Trim(sanitized, "-")

	// If empty after sanitization, return empty (caller must handle)
	if len(sanitized) == 0 {
		return ""
	}

	// Limit to 15 characters (IANA service name limit)
	if len(sanitized) > 15 {
		sanitized = sanitized[:15]
		// Trim trailing hyphen if we cut in the middle of a word
		sanitized = strings.TrimRight(sanitized, "-")
	}

	return sanitized
}
