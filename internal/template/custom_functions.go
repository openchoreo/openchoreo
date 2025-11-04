// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"maps"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/parser"

	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// omitValue is a sentinel used to mark values that should be pruned after rendering.
type omitValue struct{}

var omitSentinel = &omitValue{}

const omitErrMsg = "__OC_RENDERER_OMIT__"

// omitCELValue is a CEL value type that represents an omitted value.
// This allows omit() to return a valid CEL value that can be used inside
// map literals and arrays, rather than an error that propagates up.
type omitCELValue struct{}

var (
	omitCEL     = &omitCELValue{}
	omitTypeVal = cel.ObjectType("omit")
)

// CEL ref.Val interface implementation for omitCELValue
func (o *omitCELValue) ConvertToNative(typeDesc reflect.Type) (interface{}, error) {
	return omitSentinel, nil
}

func (o *omitCELValue) ConvertToType(typeVal ref.Type) ref.Val {
	return o
}

func (o *omitCELValue) Equal(other ref.Val) ref.Val {
	if _, ok := other.(*omitCELValue); ok {
		return types.True
	}
	return types.False
}

func (o *omitCELValue) Type() ref.Type {
	return omitTypeVal
}

func (o *omitCELValue) Value() interface{} {
	return omitSentinel
}

// CustomFunctions returns the CEL environment options for custom template functions.
// These functions provide additional capabilities beyond the standard CEL-go extensions.
//
// Available custom functions:
//   - omit(): Returns a sentinel value that causes the field to be removed from output
//   - merge(map1, map2): Merges two maps, with map2 values overriding map1
//   - sanitizeK8sResourceName(...strings): Converts strings to valid K8s resource names
func CustomFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Macros(sanitizeK8sResourceNameMacro),
		cel.Function("omit",
			cel.Overload("omit", []*cel.Type{}, cel.DynType,
				cel.FunctionBinding(func(values ...ref.Val) ref.Val {
					return omitCEL
				}),
			),
		),
		cel.Function("merge",
			cel.Overload("merge_map_map",
				[]*cel.Type{cel.MapType(cel.StringType, cel.DynType), cel.MapType(cel.StringType, cel.DynType)},
				cel.MapType(cel.StringType, cel.DynType),
				cel.BinaryBinding(mergeMapFunction),
			),
		),
		cel.Function("sanitizeK8sResourceName",
			cel.Overload("sanitize_k8s_resource_name_string",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					return sanitizeK8sNameFromStrings([]string{arg.Value().(string)})
				}),
			),
			cel.Overload("sanitize_k8s_resource_name_list",
				[]*cel.Type{cel.ListType(cel.StringType)},
				cel.StringType,
				cel.UnaryBinding(sanitizeK8sName),
			),
		),
	}
}

// mergeMapFunction implements the merge() CEL function.
// It performs a shallow merge of two maps, with values from the second map
// overriding values from the first map.
//
// Example usage in templates:
//
//	${merge(defaults, overrides)}
//	${merge({replicas: 1, memory: "256Mi"}, spec.resources)}
func mergeMapFunction(lhs, rhs ref.Val) ref.Val {
	baseVal := lhs.Value()
	overrideVal := rhs.Value()

	baseMap := make(map[string]any)
	overrideMap := make(map[string]any)

	// Convert base map from CEL types to Go types
	switch b := baseVal.(type) {
	case map[string]any:
		baseMap = b
	case map[ref.Val]ref.Val:
		for k, v := range b {
			baseMap[string(k.(types.String))] = v.Value()
		}
	}

	// Convert override map from CEL types to Go types
	switch o := overrideVal.(type) {
	case map[string]any:
		overrideMap = o
	case map[ref.Val]ref.Val:
		for k, v := range o {
			overrideMap[string(k.(types.String))] = v.Value()
		}
	}

	// Merge maps
	result := make(map[string]any)
	maps.Copy(result, baseMap)
	maps.Copy(result, overrideMap)

	// Convert back to CEL map type
	celResult := make(map[ref.Val]ref.Val)
	for k, v := range result {
		celResult[types.String(k)] = types.DefaultTypeAdapter.NativeToValue(v)
	}

	return types.NewDynamicMap(types.DefaultTypeAdapter, celResult)
}

// sanitizeK8sNameFromStrings converts arbitrary strings into valid Kubernetes resource names.
//
// This function uses GenerateK8sNameWithLengthLimit to ensure the generated name:
//   - Follows DNS subdomain rules (lowercase alphanumeric, hyphens, dots)
//   - Is truncated to fit within 253 characters (default K8s limit)
//   - Includes a hash suffix for uniqueness when names are long
//   - Starts and ends with alphanumeric characters
//
// This function enables templates to construct resource names from user input or component
// metadata without manual sanitization. For example:
//   - "My App!", "v2" -> "my-app-v2-a1b2c3d4"
//   - "payment-service", "prod" -> "payment-service-prod-e5f6g7h8"
func sanitizeK8sNameFromStrings(parts []string) ref.Val {
	result := kubernetes.GenerateK8sNameWithLengthLimit(kubernetes.MaxResourceNameLength, parts...)
	return types.String(result)
}

// sanitizeK8sName is the CEL binding for sanitizeK8sResourceName().
// It handles multiple input formats from CEL expressions to provide flexible name sanitization.
//
// Supported input types:
//   - Single string: sanitizeK8sResourceName("My App")
//   - List of strings: sanitizeK8sResourceName(["my", "app", "v2"])
//   - Variadic args: sanitizeK8sResourceName("my", "app", "v2") (via macro expansion)
//
// Non-string list items are silently ignored, allowing mixed-type lists to be processed.
func sanitizeK8sName(arg ref.Val) ref.Val {
	// CEL callers can hand us either a list (`["foo", "-", "bar"]`) or a dynamic list of ref.Val.
	// Accept all of them so reusable template helpers keep working unchanged.
	parts := []string{}

	// Handle different input types
	switch v := arg.Value().(type) {
	case string:
		parts = append(parts, v)
	case []ref.Val:
		for _, item := range v {
			if str, ok := item.Value().(string); ok {
				parts = append(parts, str)
			}
		}
	case []any:
		for _, item := range v {
			if str, ok := item.(string); ok {
				parts = append(parts, str)
			}
		}
	}

	return sanitizeK8sNameFromStrings(parts)
}

// sanitizeK8sResourceNameMacro enables variadic syntax for sanitizeK8sResourceName in templates.
//
// This macro transforms variadic calls into list-based calls that the runtime function can handle:
//   - sanitizeK8sResourceName("a", "b", "c") → sanitizeK8sResourceName(["a", "b", "c"])
//   - sanitizeK8sResourceName() → sanitizeK8sResourceName([])
//   - sanitizeK8sResourceName("single") → passes through unchanged (no macro expansion needed)
//
// This allows template authors to use natural syntax like ${sanitizeK8sResourceName(component.name, "-", environment)}
// instead of the more verbose ${sanitizeK8sResourceName([component.name, "-", environment])}.
var sanitizeK8sResourceNameMacro = cel.GlobalVarArgMacro("sanitizeK8sResourceName",
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		switch len(args) {
		case 0:
			// No args: convert to empty list
			return eh.NewCall("sanitizeK8sResourceName", eh.NewList()), nil
		case 1:
			// Single arg: no macro expansion needed, pass through to function
			return nil, nil
		default:
			// Multiple args: wrap in list for function to process
			return eh.NewCall("sanitizeK8sResourceName", eh.NewList(args...)), nil
		}
	})
