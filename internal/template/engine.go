// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
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

// Engine evaluates CEL backed templates that can contain inline expressions, map keys, and nested structures.
type Engine struct {
	cache *EngineCache
}

// NewEngine creates a new CEL template engine with default cache settings.
func NewEngine() *Engine {
	return &Engine{
		cache: NewEngineCache(),
	}
}

// NewEngineWithOptions creates a new CEL template engine with custom cache options.
// Use this for testing and benchmarking different caching strategies.
//
// Example:
//
//	// Disable all caching for baseline benchmark
//	engine := template.NewEngineWithOptions(template.DisableCache())
//
//	// Disable only program cache to measure its impact
//	engine := template.NewEngineWithOptions(template.DisableProgramCacheOnly())
func NewEngineWithOptions(opts ...EngineOption) *Engine {
	return &Engine{
		cache: NewEngineCacheWithOptions(opts...),
	}
}

// Render walks the provided structure and evaluates CEL expressions against the supplied inputs.
func (e *Engine) Render(data any, inputs map[string]any) (any, error) {
	switch v := data.(type) {
	case string:
		return e.renderString(v, inputs)
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, value := range v {
			renderedKey, err := e.renderString(key, inputs)
			if err != nil {
				return nil, err
			}
			evaluatedKey := key
			if keyStr, ok := renderedKey.(string); ok {
				evaluatedKey = keyStr
			} else if renderedKey != key {
				// Dynamic key expression evaluated to non-string
				return nil, fmt.Errorf("dynamic map key '%s' must evaluate to a string, got %T: %v", key, renderedKey, renderedKey)
			}

			renderedValue, err := e.Render(value, inputs)
			if err != nil {
				return nil, err
			}
			if renderedValue == omitSentinel {
				continue
			}
			result[evaluatedKey] = renderedValue
		}
		return result, nil
	case []any:
		result := make([]any, 0, len(v))
		for _, item := range v {
			rendered, err := e.Render(item, inputs)
			if err != nil {
				return nil, err
			}
			if rendered == omitSentinel {
				continue
			}
			result = append(result, rendered)
		}
		return result, nil
	default:
		return v, nil
	}
}

// renderString evaluates CEL expressions within a string value.
//
// This function handles two distinct rendering modes:
//
//  1. Standalone expression mode: When the string contains a single expression that occupies
//     the entire string (after trimming), the expression's native type is returned directly.
//     Example: "  ${spec.replicas}  " evaluates to integer 3, not string "3"
//
//  2. Interpolation mode: When the string contains multiple expressions or text mixed with
//     expressions, all expressions are evaluated and converted to strings for interpolation.
//     Example: "image:${spec.name}:${spec.tag}" becomes "image:myapp:v1.0"
//
// Type conversion in interpolation mode:
//   - Strings: used as-is
//   - Numbers: formatted with minimal precision (%d for integers, %g for floats)
//   - Booleans: formatted as "true" or "false"
//   - Objects/arrays: JSON-marshaled, falling back to %v formatting on error
func (e *Engine) renderString(str string, inputs map[string]any) (any, error) {
	expressions := findCELExpressions(str)
	if len(expressions) == 0 {
		return str, nil
	}

	// Standalone expression: return native type (e.g., ${spec.replicas} returns int, not "3")
	trimmed := strings.TrimSpace(str)
	if len(expressions) == 1 && expressions[0].fullExpr == trimmed {
		result, err := e.evaluateCEL(expressions[0].innerExpr, inputs)
		return normalizeCELResult(result, err)
	}

	// Interpolation mode: substitute all expressions into the string
	rendered := str
	for _, match := range expressions {
		value, err := e.evaluateCEL(match.innerExpr, inputs)
		if err != nil {
			return nil, err
		}

		// Convert CEL result to string for interpolation
		var replacement string
		switch typed := value.(type) {
		case string:
			replacement = typed
		case int64:
			replacement = fmt.Sprintf("%d", typed)
		case float64:
			replacement = fmt.Sprintf("%g", typed)
		case bool:
			replacement = fmt.Sprintf("%t", typed)
		default:
			// Complex types: try JSON marshaling for clean output
			bytes, err := json.Marshal(typed)
			if err != nil {
				replacement = fmt.Sprintf("%v", typed)
			} else {
				replacement = string(bytes)
			}
		}

		rendered = strings.Replace(rendered, match.fullExpr, replacement, 1)
	}

	return rendered, nil
}

type celMatch struct {
	fullExpr  string
	innerExpr string
}

// findCELExpressions locates all ${...} expression markers within a string.
//
// This function performs brace-balanced parsing to handle nested curly braces correctly.
// For example, in "${merge({a: 1}, {b: 2})}", the parser counts opening and closing braces
// to identify the complete expression boundary.
//
// The algorithm uses a brace counter that increments on '{' and decrements on '}'.
// When the counter returns to zero, we've found the matching closing brace.
//
// Returns:
//   - fullExpr: the complete ${...} expression including delimiters
//   - innerExpr: the CEL expression content without ${ and }
//
// Example:
//   - Input: "image:${spec.image}:${spec.tag}"
//   - Output: [{fullExpr: "${spec.image}", innerExpr: "spec.image"},
//     {fullExpr: "${spec.tag}", innerExpr: "spec.tag"}]
func findCELExpressions(str string) []celMatch {
	var matches []celMatch
	i := 0
	for i < len(str) {
		start := strings.Index(str[i:], "${")
		if start == -1 {
			break
		}
		start += i

		// Use brace counter to handle nested curly braces in CEL expressions
		// e.g., ${merge({a: 1}, {b: 2})} requires counting to find the correct closing brace
		brace := 1
		pos := start + 2
		for pos < len(str) && brace > 0 {
			switch str[pos] {
			case '{':
				brace++
			case '}':
				brace--
			}
			pos++
		}

		if brace == 0 {
			matches = append(matches, celMatch{
				fullExpr:  str[start:pos],
				innerExpr: str[start+2 : pos-1],
			})
			i = pos
		} else {
			// Unclosed brace - stop parsing
			break
		}
	}
	return matches
}

// normalizeCELResult processes evaluation results to handle the special omit sentinel value.
// The omit sentinel is used to mark fields that should be removed from the rendered output,
// allowing templates to conditionally exclude fields using the omit() helper function.
//
// This function ensures both pointer and value comparisons work correctly for omit detection.
func normalizeCELResult(result any, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	if result == omitSentinel {
		return omitSentinel, nil
	}
	if val, ok := result.(*omitValue); ok && val == omitSentinel {
		return omitSentinel, nil
	}
	return result, nil
}

func (e *Engine) evaluateCEL(expression string, inputs map[string]any) (any, error) {
	env, err := e.getOrCreateEnv(inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to build CEL environment: %w", err)
	}

	// Try to get compiled program from cache
	envKey := envCacheKey(inputs)

	var program cel.Program
	if cached, ok := e.cache.GetProgram(envKey, expression); ok {
		program = cached
	} else {
		// Compile and cache the program
		ast, issues := env.Compile(expression)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("CEL compilation error in expression '%s': %w", expression, issues.Err())
		}

		program, err = env.Program(ast)
		if err != nil {
			return nil, fmt.Errorf("CEL program creation error for expression '%s': %w", expression, err)
		}

		// Store in cache for future use
		e.cache.SetProgram(envKey, expression, program)
	}

	result, _, err := program.Eval(inputs)
	if err != nil {
		if err.Error() == omitErrMsg {
			return omitSentinel, nil
		}
		return nil, fmt.Errorf("CEL evaluation error in expression '%s': %w", expression, err)
	}

	return convertCELValue(result), nil
}

func (e *Engine) getOrCreateEnv(inputs map[string]any) (*cel.Env, error) {
	cacheKey := envCacheKey(inputs)

	// Try to get from cache
	if cached, ok := e.cache.GetEnv(cacheKey); ok {
		return cached, nil
	}

	// Build new environment
	env, err := buildEnv(inputs)
	if err != nil {
		return nil, err
	}

	// Store in cache
	e.cache.SetEnv(cacheKey, env)
	return env, nil
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

// buildEnv wires up CEL with the helper surface area expected by our templating story so authors
// can reuse common snippets like `omit`, `merge`, and `sanitizeK8sResourceName`.
func buildEnv(inputs map[string]any) (*cel.Env, error) {
	envOptions := []cel.EnvOption{
		cel.OptionalTypes(),
	}

	for key := range inputs {
		envOptions = append(envOptions, cel.Variable(key, cel.DynType))
	}

	envOptions = append(envOptions,
		ext.Strings(),
		ext.Encoders(),
		ext.Math(),
		ext.Lists(),
		ext.Sets(),
		ext.TwoVarComprehensions(),
		cel.Macros(sanitizeK8sResourceNameMacro),
		cel.Function("omit",
			cel.Overload("omit", []*cel.Type{}, cel.DynType,
				cel.FunctionBinding(func(values ...ref.Val) ref.Val {
					return omitCEL
				}),
			),
		),
		cel.Function("merge",
			cel.Overload("merge_map_map", []*cel.Type{cel.MapType(cel.StringType, cel.DynType), cel.MapType(cel.StringType, cel.DynType)}, cel.MapType(cel.StringType, cel.DynType),
				cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
					baseVal := lhs.Value()
					overrideVal := rhs.Value()

					baseMap := make(map[string]any)
					overrideMap := make(map[string]any)

					switch b := baseVal.(type) {
					case map[string]any:
						baseMap = b
					case map[ref.Val]ref.Val:
						for k, v := range b {
							baseMap[string(k.(types.String))] = v.Value()
						}
					}

					switch o := overrideVal.(type) {
					case map[string]any:
						overrideMap = o
					case map[ref.Val]ref.Val:
						for k, v := range o {
							overrideMap[string(k.(types.String))] = v.Value()
						}
					}

					result := make(map[string]any)
					maps.Copy(result, baseMap)
					maps.Copy(result, overrideMap)

					celResult := make(map[ref.Val]ref.Val)
					for k, v := range result {
						celResult[types.String(k)] = types.DefaultTypeAdapter.NativeToValue(v)
					}

					return types.NewDynamicMap(types.DefaultTypeAdapter, celResult)
				}),
			),
		),
		cel.Function("sanitizeK8sResourceName",
			cel.Overload("sanitize_k8s_resource_name_string", []*cel.Type{cel.StringType}, cel.StringType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					return sanitizeK8sNameFromStrings([]string{arg.Value().(string)})
				}),
			),
			cel.Overload("sanitize_k8s_resource_name_list", []*cel.Type{cel.ListType(cel.StringType)}, cel.StringType,
				cel.UnaryBinding(sanitizeK8sName),
			),
		),
	)

	return cel.NewEnv(envOptions...)
}

// convertCELList converts a CEL list value to a native Go slice, filtering out omit markers.
func convertCELList(list any) any {
	switch l := list.(type) {
	case []ref.Val:
		result := make([]any, 0, len(l))
		for _, item := range l {
			converted := convertCELValue(item)
			if converted == omitSentinel {
				continue
			}
			result = append(result, converted)
		}
		return result
	case []any:
		return convertAnyList(l)
	default:
		return list
	}
}

// convertAnyList converts a []any list, handling ref.Val items and maps.
func convertAnyList(list []any) []any {
	result := make([]any, 0, len(list))
	for _, item := range list {
		switch t := item.(type) {
		case ref.Val:
			converted := convertCELValue(t)
			if converted == omitSentinel {
				continue
			}
			result = append(result, converted)
		case map[ref.Val]ref.Val:
			m := convertRefValMap(t)
			result = append(result, m)
		default:
			result = append(result, item)
		}
	}
	return result
}

// convertRefValMap converts a map[ref.Val]ref.Val to map[string]any, filtering out omit markers.
func convertRefValMap(m map[ref.Val]ref.Val) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		converted := convertCELValue(v)
		if converted == omitSentinel {
			continue
		}
		keyStr := fmt.Sprintf("%v", k.Value())
		result[keyStr] = converted
	}
	return result
}

// convertStringAnyMap converts a map[string]any, handling ref.Val values.
func convertStringAnyMap(m map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		switch nested := v.(type) {
		case ref.Val:
			converted := convertCELValue(nested)
			if converted == omitSentinel {
				continue
			}
			result[k] = converted
		default:
			result[k] = v
		}
	}
	return result
}

// convertCELValue converts CEL's internal value types to native Go types.
//
// CEL uses its own value representation (ref.Val) to support rich type checking and
// cross-language compatibility. This function unwraps these values into standard Go types
// that can be easily marshaled to JSON/YAML.
//
// Special handling:
//   - omitCELValue: Returns omitSentinel to mark fields for removal
//   - Lists and maps: Recursively converted, filtering out omit sentinels
//   - Nested ref.Val: Recursively unwrapped until native types are reached
//
// Type conversions:
//   - CEL strings/ints/bools → Go string/int64/bool
//   - CEL lists → Go []any (with omit filtering)
//   - CEL maps → Go map[string]any (with omit filtering)
func convertCELValue(val ref.Val) any {
	// Check if this is an omit marker
	if _, ok := val.(*omitCELValue); ok {
		return omitSentinel
	}

	// Legacy error-based omit check (kept for backwards compatibility)
	if types.IsError(val) {
		if err, ok := val.Value().(error); ok && err.Error() == omitErrMsg {
			return omitSentinel
		}
	}

	switch val.Type() {
	case types.StringType:
		return val.Value().(string)
	case types.IntType:
		return val.Value().(int64)
	case types.UintType:
		return val.Value().(uint64)
	case types.DoubleType:
		return val.Value().(float64)
	case types.BoolType:
		return val.Value().(bool)
	case types.ListType:
		return convertCELList(val.Value())
	case types.MapType:
		switch m := val.Value().(type) {
		case map[ref.Val]ref.Val:
			return convertRefValMap(m)
		case map[string]any:
			return convertStringAnyMap(m)
		default:
			return val.Value()
		}
	default:
		// Handle wrapped ref.Val or unknown types
		switch typed := val.Value().(type) {
		case ref.Val:
			return convertCELValue(typed)
		default:
			return typed
		}
	}
}

// RemoveOmittedFields walks the rendered tree after CEL evaluation and strips the omit() sentinel.
// Templates using the reusable `omit()` helper stay compatible with the rendering pipeline's pruning semantics.
func RemoveOmittedFields(data any) any {
	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, value := range v {
			if value == omitSentinel {
				continue
			}
			cleaned := RemoveOmittedFields(value)
			if cleaned != omitSentinel {
				result[key] = cleaned
			}
		}
		return result
	case []any:
		result := make([]any, 0, len(v))
		for _, item := range v {
			if item == omitSentinel {
				continue
			}
			cleaned := RemoveOmittedFields(item)
			if cleaned != omitSentinel {
				result = append(result, cleaned)
			}
		}
		return result
	default:
		return v
	}
}

// IsMissingDataError checks if an error indicates missing data during CEL evaluation.
// This handles CEL runtime errors for missing keys and compile-time errors for
// undefined variables. These errors are used for graceful degradation in optional
// contexts like includeWhen and where clauses.
//
// CEL returns:
//   - "no such key: <key>" for missing map keys/fields at runtime
//   - "undeclared reference to '<var>'" for undefined variables at compile time
func IsMissingDataError(err error) bool {
	if err == nil {
		return false
	}

	// Check for actual CEL error messages
	msg := err.Error()
	return strings.Contains(msg, "no such key") ||
		strings.Contains(msg, "undeclared reference")
}
