// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/parser"

	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// CELExtensions returns CEL environment options for configuration helpers.
// These include:
//   - Macro: configurations.toConfigFileList(prefix) -> configurationsToConfigFileList(configurations, prefix)
//   - Function: configurationsToConfigFileList
func CELExtensions() []cel.EnvOption {
	return []cel.EnvOption{
		// Register the macro
		cel.Macros(toConfigFileListMacro),
		// Register the function
		cel.Function("configurationsToConfigFileList",
			cel.Overload("configurationsToConfigFileList_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.DynType),
				cel.BinaryBinding(configurationsToConfigFileListFunction),
			),
		),
	}
}

// toConfigFileListMacro transforms configurations.toConfigFileList(prefix) into
// configurationsToConfigFileList(configurations, prefix) at compile time.
var toConfigFileListMacro = cel.ReceiverMacro("toConfigFileList", 1,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == "configurations" {
			return eh.NewCall("configurationsToConfigFileList", target, args[0]), nil
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
