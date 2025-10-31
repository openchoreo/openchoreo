// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/json"
	"fmt"
	"maps"
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
			if ptr, ok := renderedValue.(*omitValue); ok && ptr == omitSentinel {
				continue
			}
			result[evaluatedKey] = renderedValue
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			rendered, err := e.Render(item, inputs)
			if err != nil {
				return nil, err
			}
			result[i] = rendered
		}
		return result, nil
	default:
		return v, nil
	}
}

func (e *Engine) renderString(str string, inputs map[string]any) (any, error) {
	expressions := findCELExpressions(str)
	if len(expressions) == 0 {
		return str, nil
	}

	trimmed := strings.TrimSpace(str)
	if len(expressions) == 1 && expressions[0].fullExpr == trimmed {
		result, err := e.evaluateCEL(expressions[0].innerExpr, inputs)
		return normalizeCELResult(result, err)
	}

	rendered := str
	for _, match := range expressions {
		value, err := e.evaluateCEL(match.innerExpr, inputs)
		if err != nil {
			return nil, err
		}

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

func findCELExpressions(str string) []celMatch {
	var matches []celMatch
	i := 0
	for i < len(str) {
		start := strings.Index(str[i:], "${")
		if start == -1 {
			break
		}
		start += i

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
			break
		}
	}
	return matches
}

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

// sanitizeK8sNameFromStrings collapses fragments into a DNS-ish identifier so templates can stitch
// together names without worrying about illegal characters.
func sanitizeK8sNameFromStrings(parts []string) ref.Val {
	cleanedNames := make([]string, len(parts))
	for i, name := range parts {
		cleanedNames[i] = strings.ReplaceAll(name, ".", "-")
	}
	return types.String(kubernetes.GenerateK8sNameWithLengthLimit(63, cleanedNames...))
}

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

// sanitizeK8sResourceNameMacro keeps `${sanitizeK8sResourceName(...)}` available so templates can
// normalize resource names with a single call.
var sanitizeK8sResourceNameMacro = cel.GlobalVarArgMacro("sanitizeK8sResourceName",
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		switch len(args) {
		case 0:
			return eh.NewCall("sanitizeK8sResourceName", eh.NewList()), nil
		case 1:
			return nil, nil
		default:
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
					return types.NewErr(omitErrMsg)
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

// convertCELValue collapses CEL's dynamic values into native Go types so the rendered objects line
// up with standard JSON/YAML expectations.
func convertCELValue(val ref.Val) any {
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
		switch list := val.Value().(type) {
		case []ref.Val:
			result := make([]any, len(list))
			for i, item := range list {
				result[i] = convertCELValue(item)
			}
			return result
		case []any:
			result := make([]any, len(list))
			for i, item := range list {
				switch t := item.(type) {
				case ref.Val:
					result[i] = convertCELValue(t)
				case map[ref.Val]ref.Val:
					m := make(map[string]any)
					for k, v := range t {
						keyStr := fmt.Sprintf("%v", k.Value())
						m[keyStr] = convertCELValue(v)
					}
					result[i] = m
				default:
					result[i] = item
				}
			}
			return result
		default:
			return val.Value()
		}
	case types.MapType:
		switch m := val.Value().(type) {
		case map[ref.Val]ref.Val:
			result := make(map[string]any)
			for k, v := range m {
				result[fmt.Sprintf("%v", k.Value())] = convertCELValue(v)
			}
			return result
		case map[string]any:
			result := make(map[string]any)
			for k, v := range m {
				switch nested := v.(type) {
				case ref.Val:
					result[k] = convertCELValue(nested)
				default:
					result[k] = v
				}
			}
			return result
		default:
			return val.Value()
		}
	default:
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
			if ptr, ok := value.(*omitValue); ok && ptr == omitSentinel {
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
