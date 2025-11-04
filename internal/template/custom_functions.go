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
//   - oc_omit(): Returns a sentinel value that causes the field to be removed from output
//   - oc_merge(map1, map2, ...mapN): Merges multiple maps, with later maps overriding earlier ones
//   - oc_generate_name(...strings): Generates a valid K8s resource name with hash suffix for uniqueness
//
// All custom functions use the "oc_" prefix to avoid potential conflicts with upstream CEL-go.
func CustomFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Macros(generateNameMacro, mergeMacro),
		cel.Function("oc_omit",
			cel.Overload("oc_omit", []*cel.Type{}, cel.DynType,
				cel.FunctionBinding(func(values ...ref.Val) ref.Val {
					return omitCEL
				}),
			),
		),
		cel.Function("oc_merge",
			cel.Overload("oc_merge_map_map",
				[]*cel.Type{cel.MapType(cel.StringType, cel.DynType), cel.MapType(cel.StringType, cel.DynType)},
				cel.MapType(cel.StringType, cel.DynType),
				cel.BinaryBinding(mergeMapFunction),
			),
		),
		cel.Function("oc_generate_name",
			cel.Overload("oc_generate_name_string",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					return generateK8sNameFromStrings([]string{arg.Value().(string)})
				}),
			),
			cel.Overload("oc_generate_name_list",
				[]*cel.Type{cel.ListType(cel.StringType)},
				cel.StringType,
				cel.UnaryBinding(generateK8sName),
			),
		),
	}
}

// mergeMapFunction implements the oc_merge() CEL function.
// It performs a shallow merge of two maps, with values from the second map
// overriding values from the first map.
//
// The macro expansion allows variadic usage:
//   - oc_merge(a, b) → direct binary merge
//   - oc_merge(a, b, c) → oc_merge(oc_merge(a, b), c)
//   - oc_merge(a, b, c, d) → oc_merge(oc_merge(oc_merge(a, b), c), d)
//
// Example usage in templates:
//
//	${oc_merge(defaults, overrides)}
//	${oc_merge({replicas: 1}, spec.resources, env.overrides)}
//	${oc_merge(base, layer1, layer2, layer3)}
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

// generateK8sNameFromStrings generates a valid Kubernetes resource name from arbitrary strings.
//
// This function uses GenerateK8sNameWithLengthLimit to ensure the generated name:
//   - Follows DNS subdomain rules (lowercase alphanumeric, hyphens, dots)
//   - Is truncated to fit within 253 characters (default K8s limit)
//   - Includes a hash suffix for uniqueness
//   - Starts and ends with alphanumeric characters
//
// This function enables templates to construct resource names from user input or component
// metadata without manual sanitization. For example:
//   - "My App!", "v2" -> "my-app-v2-a1b2c3d4"
//   - "payment-service", "prod" -> "payment-service-prod-e5f6g7h8"
func generateK8sNameFromStrings(parts []string) ref.Val {
	result := kubernetes.GenerateK8sNameWithLengthLimit(kubernetes.MaxResourceNameLength, parts...)
	return types.String(result)
}

// generateK8sName is the CEL binding for oc_generate_name().
// It handles multiple input formats from CEL expressions to provide flexible name generation.
//
// Supported input types:
//   - Single string: oc_generate_name("My App")
//   - List of strings: oc_generate_name(["my", "app", "v2"])
//   - Variadic args: oc_generate_name("my", "app", "v2") (via macro expansion)
//
// Non-string list items are silently ignored, allowing mixed-type lists to be processed.
func generateK8sName(arg ref.Val) ref.Val {
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

	return generateK8sNameFromStrings(parts)
}

// generateNameMacro enables variadic syntax for oc_generate_name in templates.
//
// This macro transforms variadic calls into list-based calls that the runtime function can handle:
//   - oc_generate_name("a", "b", "c") → oc_generate_name(["a", "b", "c"])
//   - oc_generate_name() → oc_generate_name([])
//   - oc_generate_name("single") → passes through unchanged (no macro expansion needed)
//
// This allows template authors to use natural syntax like ${oc_generate_name(component.name, "-", environment)}
// instead of the more verbose ${oc_generate_name([component.name, "-", environment])}.
var generateNameMacro = cel.GlobalVarArgMacro("oc_generate_name",
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		switch len(args) {
		case 0:
			// No args: convert to empty list
			return eh.NewCall("oc_generate_name", eh.NewList()), nil
		case 1:
			// Single arg: no macro expansion needed, pass through to function
			return nil, nil
		default:
			// Multiple args: wrap in list for function to process
			return eh.NewCall("oc_generate_name", eh.NewList(args...)), nil
		}
	})

// mergeMacro enables variadic syntax for oc_merge in templates.
//
// This macro transforms variadic calls into nested binary calls that chain the merges:
//   - oc_merge(a, b) → passes through unchanged (binary function handles it)
//   - oc_merge(a, b, c) → oc_merge(oc_merge(a, b), c)
//   - oc_merge(a, b, c, d) → oc_merge(oc_merge(oc_merge(a, b), c), d)
//
// This allows template authors to merge multiple maps in a single call:
//
//	${oc_merge(defaults, component.spec, env.overrides)}
//
// The merge is left-associative, meaning later arguments override earlier ones.
var mergeMacro = cel.GlobalVarArgMacro("oc_merge",
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		switch len(args) {
		case 0, 1:
			// Need at least 2 arguments for merge
			return nil, &common.Error{
				Message: "oc_merge requires at least 2 arguments",
			}
		case 2:
			// Binary call: no macro expansion needed, pass through to function
			return nil, nil
		default:
			// Variadic call: chain merges left-to-right
			// oc_merge(a, b, c, d) becomes oc_merge(oc_merge(oc_merge(a, b), c), d)
			result := eh.NewCall("oc_merge", args[0], args[1])
			for i := 2; i < len(args); i++ {
				result = eh.NewCall("oc_merge", result, args[i])
			}
			return result, nil
		}
	})
