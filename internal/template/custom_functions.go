// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"fmt"
	"hash/fnv"
	"maps"
	"math"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"
	"github.com/google/cel-go/interpreter"
	"github.com/google/cel-go/parser"

	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// maxListsRangeSize bounds lists.range so a single call cannot materialize more than
// ~maxListsRangeSize x 24B of slice memory. Set to 10_000 because cel-go's cost model is
// a poor proxy for CPU once a list grows past ~10k elements: a measured
// lists.range(10000).map(...) costs 150k and runs in 85ms, while the same expression at
// 100k costs 1.5M and runs in 6.4s — 10x the cost for 75x the time. No sample uses
// lists.range, so the tightening costs nothing in practice.
//
// This closes one route to a huge list, not all of them: split() on a large
// tenant-supplied string is another, and is unaffected by this cap
// (v.split(",").map(...) over an 800KB value burns ~9.7s before the cost limit fires).
// The per-expression cost limit therefore bounds memory, not wall-clock. The controls for
// the residual routes are the cumulative reconcile budget and the opt-in render timeout.
const maxListsRangeSize int64 = 10_000

// BaseCELExtensions returns the CEL extensions used across OpenChoreo.
// This includes optional types, common utility extensions for strings, encoding,
// math, lists, sets, two-variable comprehensions, and OpenChoreo custom functions.
func BaseCELExtensions() []cel.EnvOption {
	opts := []cel.EnvOption{
		cel.OptionalTypes(),
		ext.Strings(),
		ext.Encoders(),
		ext.Math(),
		ext.Lists(ext.ListsMaxRangeSize(maxListsRangeSize)),
		ext.Sets(),
		ext.TwoVarComprehensions(),
	}
	return append(opts, CustomFunctions()...)
}

// omitValue is a sentinel used to mark values that should be pruned after rendering.
// The template engine recognizes this sentinel and removes the containing field from output.
type omitValue struct{}

var omitSentinel = &omitValue{}

const omitErrMsg = "__OC_RENDERER_OMIT__"

// omitCELValue is a CEL value type that represents an omitted value.
//
// This internal type allows oc_omit() to return a valid CEL value (rather than an error)
// that can be safely used inside map literals and arrays. The template engine's post-processing
// phase detects the omitSentinel and removes the containing field, map key, or array element
// from the final rendered output.
//
// Implementation notes:
//   - ConvertToNative returns omitSentinel which the pruning logic recognizes
//   - Type() returns a custom "omit" type to distinguish from other CEL values
//   - Equal() only returns true when comparing two omitCELValue instances
//
// See CustomFunctions() documentation for usage examples.
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
//
// These functions provide additional capabilities beyond the standard CEL-go extensions,
// designed for use in CEL-based templates throughout OpenChoreo. All custom functions use
// the "oc_" prefix to avoid potential conflicts with upstream CEL-go.
//
// # Available Functions
//
// oc_omit() - Remove fields, map keys, or array items from rendered output
//
// oc_merge(map1, map2, ...mapN) - Shallow merge of multiple maps
//
// oc_generate_name(...strings) - Generate valid Kubernetes resource names (≤253 chars)
//
// oc_dns_label(...strings) - Generate valid Kubernetes DNS label names (≤63 chars)
//
// oc_hash(string) - Generate 8-character hash from input string
//
// # oc_omit() - Conditional Omission
//
// Returns a sentinel value that is removed during post-processing. Supports two use cases:
//
// Use Case 1: Remove entire fields from YAML/JSON structure
//
//	metadata:
//	  annotations: ${has(spec.annotations) ? spec.annotations : oc_omit()}
//	  labels:
//	    version: ${has(spec.version) ? spec.version : oc_omit()}
//
// Result when spec.annotations and spec.version are undefined:
//
//	metadata:
//	  labels: {}
//
// Use Case 2: Remove map keys or array items within CEL expressions
//
//	# Conditional map keys
//	labels: ${{"app": metadata.name, "env": has(spec.env) ? spec.env : oc_omit()}}
//
//	# Conditional array items
//	args: ${["--port=8080", spec.debug ? "--debug" : oc_omit(), "--log=info"]}
//
// # oc_merge() - Shallow Map Merge
//
// Merges multiple maps left-to-right, with later maps overriding earlier ones.
// IMPORTANT: This is a shallow merge - nested maps are replaced, not merged recursively.
//
//	# Basic merge
//	env: ${oc_merge(defaults, spec.env, environmentConfigs)}
//
//	# Inline map literals
//	resources: ${oc_merge({cpu: "100m", memory: "128Mi"}, spec.resources)}
//
//	# Variadic merge (3+ maps)
//	config: ${oc_merge(base, layer1, layer2, layer3)}
//
// Shallow merge behavior:
//
//	base = {resources: {cpu: "100m", memory: "128Mi"}, replicas: 1}
//	override = {resources: {cpu: "200m"}}
//	result = {resources: {cpu: "200m"}, replicas: 1}
//	# Note: memory is LOST because resources map was replaced entirely
//
// # oc_generate_name() - Kubernetes Name Generation
//
// Generates valid Kubernetes DNS subdomain names from arbitrary strings.
// Names are sanitized, truncated to 253 characters, and include an 8-character
// hash suffix for uniqueness.
//
//	# Variadic arguments
//	name: ${oc_generate_name(component.name, environment, "cache")}
//	# "payment-service", "prod", "cache" -> "payment-service-prod-cache-a1b2c3d4"
//
//	# Array input
//	name: ${oc_generate_name([metadata.namespace, metadata.name, "worker"])}
//
//	# Single string (sanitized)
//	name: ${oc_generate_name("My App!")}
//	# "My App!" -> "my-app-e5f6g7h8"
//
// Hash suffix ensures uniqueness even when inputs sanitize to the same string:
//
//	oc_generate_name("my-app")   -> "my-app-abc12345"
//	oc_generate_name("My App!")  -> "my-app-def67890"  # Different hash
//
// # oc_dns_label() - Kubernetes DNS Label Name Generation
//
// Same as oc_generate_name() but enforces a ≤63 character limit, suitable for
// Kubernetes DNS label names (e.g., hostname subdomain labels).
//
//	# Webapp hostname subdomain (≤63 chars)
//	hostnames:
//	  - ${oc_dns_label(endpointName, metadata.componentName, metadata.environmentName, metadata.componentNamespace)}.example.com
//
// # oc_hash() - String Hashing
//
// Generates an 8-character hexadecimal hash from an input string using the FNV-32a
// algorithm. Useful for creating stable, deterministic identifiers or suffixes.
//
// The hash is deterministic - the same input always produces the same output:
//
//	oc_hash("test")  -> "4fdcca5d"  # Always produces this hash
//	oc_hash("test")  -> "4fdcca5d"  # Same input, same output
//
// All custom functions use the "oc_" prefix to avoid potential conflicts with upstream CEL-go.
//
// Any new oc_* function that does size-proportional work must also get a CallCost case in
// customFunctionCostEstimator below; otherwise cel-go meters it at a flat one cost unit and
// the per-expression limit and reconcile budget stay blind to whatever it traverses.
func CustomFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Macros(generateNameMacro, dnslabelMacro, mergeMacro),
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
		cel.Function("oc_dns_label",
			cel.Overload("oc_dns_label_string",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					return generateK8sDNSLabelFromStrings([]string{arg.Value().(string)})
				}),
			),
			cel.Overload("oc_dns_label_list",
				[]*cel.Type{cel.ListType(cel.StringType)},
				cel.StringType,
				cel.UnaryBinding(generateK8sDNSLabel),
			),
		),
		cel.Function("oc_hash",
			cel.Overload("oc_hash_string", []*cel.Type{cel.StringType}, cel.StringType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					input := arg.Value().(string)
					h := fnv.New32a()
					h.Write([]byte(input))
					return types.String(fmt.Sprintf("%08x", h.Sum32()))
				}),
			),
		),
	}
}

// mergeMapFunction implements the binary oc_merge() CEL function.
//
// Performs a shallow merge of two maps, with values from rhs overriding values from lhs.
// Nested maps are replaced entirely, not merged recursively.
//
// The mergeMacro expands variadic calls into nested binary calls:
//   - oc_merge(a, b, c) → oc_merge(oc_merge(a, b), c)
//
// See CustomFunctions() for detailed usage examples.
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
// Sanitizes input to follow DNS subdomain rules (lowercase alphanumeric, hyphens, dots),
// truncates to 253 characters, and appends an 8-character hash suffix for uniqueness.
//
// See CustomFunctions() for detailed usage examples.
func generateK8sNameFromStrings(parts []string) ref.Val {
	result := kubernetes.GenerateK8sNameWithLengthLimit(kubernetes.MaxResourceNameLength, parts...)
	return types.String(result)
}

func generateK8sDNSLabelFromStrings(parts []string) ref.Val {
	result := kubernetes.GenerateK8sNameWithLengthLimit(kubernetes.MaxLabelNameLength, parts...)
	return types.String(result)
}

// generateK8sDNSLabel is the CEL binding for oc_dns_label().
// Same as generateK8sName but enforces a ≤63 character limit.
func generateK8sDNSLabel(arg ref.Val) ref.Val {
	parts := []string{}
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
	return generateK8sDNSLabelFromStrings(parts)
}

// generateK8sName is the CEL binding for oc_generate_name().
//
// Handles multiple input formats (single string, array, variadic via macro).
// Non-string list items are silently ignored, allowing mixed-type lists.
//
// See CustomFunctions() for detailed usage examples.
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

// dnslabelMacro enables variadic syntax for oc_dns_label in templates.
// Same expansion logic as generateNameMacro but targets oc_dns_label.
var dnslabelMacro = cel.GlobalVarArgMacro("oc_dns_label",
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		switch len(args) {
		case 0:
			return eh.NewCall("oc_dns_label", eh.NewList()), nil
		case 1:
			return nil, nil
		default:
			return eh.NewCall("oc_dns_label", eh.NewList(args...)), nil
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

// customFunctionCostEstimator charges runtime cost for the oc_* functions registered by
// CustomFunctions.
//
// cel-go charges one cost unit for any overload it does not recognize, so without this
// estimator every oc_* call is metered as O(1) no matter how much data it walks. That makes
// the per-expression cost limit and the reconcile budget blind to the one class of work a
// template author fully controls: `${oc_hash(hugeParameter)}` repeated across a large
// template traverses gigabytes while spending a handful of cost units.
//
// Each case below charges for the input the function actually traverses, using cel-go's own
// conventions so a custom call and an equivalent built-in call cost the same order of
// magnitude:
//
//   - String traversal is charged at common.StringTraversalCostFactor (0.1 per byte), the
//     same factor cel-go applies to startsWith, string conversion, and concatenation.
//   - Map traversal is charged one unit per entry, the same convention cel-go applies to
//     list containment (`in`).
//
// Dispatch is by function name rather than overload ID. Template inputs are declared as
// cel.DynType, so the checker cannot always pick a single overload for a function that
// declares more than one — oc_generate_name and oc_dns_label each declare a string and a
// list form — and an overload-keyed tracker silently misses those calls. The function name
// is always known, and the argument's own type tells the two forms apart.
//
// oc_omit takes no arguments and does no proportional work, so it falls through to cel-go's
// default cost of one.
type customFunctionCostEstimator struct{}

// CallCost implements interpreter.ActualCostEstimator. A nil return means "no estimate",
// which falls back to cel-go's O(1) default — the blind spot this exists to close — so
// every function handled here returns a real cost.
func (customFunctionCostEstimator) CallCost(function, _ string, args []ref.Val, _ ref.Val) *uint64 {
	cost := uint64(1)

	switch function {
	case "oc_hash":
		cost = saturatingAdd(cost, traversalCost(stringBytes(args[0])))

	case "oc_generate_name", "oc_dns_label":
		cost = saturatingAdd(cost, nameInputCost(args[0]))

	case "oc_merge":
		for _, arg := range args {
			cost = saturatingAdd(cost, collectionSize(arg))
		}

	default:
		return nil
	}

	return &cost
}

// CustomFunctionCostOptions returns the program options that install the estimator.
//
// cel.CostTracking feeds the same cost tracker that cel.CostLimit bounds, so a breach
// interrupts evaluation exactly as a built-in overload's cost would.
func CustomFunctionCostOptions() []cel.ProgramOption {
	return []cel.ProgramOption{cel.CostTracking(customFunctionCostEstimator{})}
}

// traversalCost converts a byte count into cost units at cel-go's string traversal factor.
func traversalCost(size uint64) uint64 {
	return uint64(math.Ceil(float64(size) * common.StringTraversalCostFactor))
}

// Lists cost by both entry count and bytes: even empty strings require iteration and allocation.
func nameInputCost(val ref.Val) uint64 {
	if lister, ok := val.(traits.Lister); ok {
		n := collectionLen(lister)
		var bytes uint64
		for i := int64(0); i < n; i++ {
			bytes = saturatingAdd(bytes, stringBytes(lister.Get(types.Int(i))))
		}
		return saturatingAdd(collectionSize(lister), traversalCost(bytes))
	}
	return traversalCost(stringBytes(val))
}

// CEL's string size is a rune count; these functions traverse bytes, so use the native string.
func stringBytes(val ref.Val) uint64 {
	s, _ := val.Value().(string)
	return uint64(len(s))
}

func collectionSize(val ref.Val) uint64 {
	n := collectionLen(val)
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// collectionLen reports a collection's entry count as CEL's own int64 size, so it indexes a
// Lister without a further conversion. Anything that cannot be sized reports -1, which reads
// as an empty collection to both the loop above and collectionSize.
func collectionLen(val ref.Val) int64 {
	sz, ok := val.(traits.Sizer)
	if !ok {
		return -1
	}
	n, ok := sz.Size().(types.Int)
	if !ok {
		return -1
	}
	return int64(n)
}

// saturatingAdd adds two costs without wrapping. A wrapped total would read as a tiny cost
// and let the very evaluation the guard exists to stop run unmetered.
func saturatingAdd(a, b uint64) uint64 {
	if a > math.MaxUint64-b {
		return math.MaxUint64
	}
	return a + b
}

var _ interpreter.ActualCostEstimator = customFunctionCostEstimator{}
