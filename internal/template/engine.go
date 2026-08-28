// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter"
)

// Engine evaluates CEL backed templates that can contain inline expressions, map keys, and nested structures.
type Engine struct {
	cache                *EngineCache
	celExtensions        []cel.EnvOption
	costLimit            uint64
	cacheDisabled        bool
	programCacheDisabled bool
}

// NewEngine creates a new CEL template engine with default cache settings.
func NewEngine() *Engine {
	e := &Engine{
		cache: newEngineCache(false, false),
	}
	e.applyCostDefaults()
	return e
}

// NewEngineWithOptions creates a new CEL template engine with custom options.
// Use this for testing and benchmarking different caching strategies,
// or to add custom CEL extensions.
//
// Example:
//
//	// Disable all caching for baseline benchmark
//	engine := template.NewEngineWithOptions(template.DisableCache())
//
//	// Add custom CEL extensions
//	engine := template.NewEngineWithOptions(template.WithCELExtensions(context.CELExtensions()...))
func NewEngineWithOptions(opts ...EngineOption) *Engine {
	e := &Engine{}
	for _, opt := range opts {
		opt(e)
	}
	if e.cache == nil {
		e.cache = newEngineCache(e.cacheDisabled, e.programCacheDisabled)
	}
	e.applyCostDefaults()
	return e
}

// applyCostDefaults installs the built-in cost limit unless a caller supplied one.
// Every engine is bounded: a zero limit selects the default, it never means "unlimited".
func (e *Engine) applyCostDefaults() {
	if e.costLimit == 0 {
		e.costLimit = defaultCELCostLimit
	}
}

// WithCELExtensions adds custom CEL environment options to the engine.
// Use this to register custom functions, macros, and types.
func WithCELExtensions(extensions ...cel.EnvOption) EngineOption {
	return func(e *Engine) {
		e.celExtensions = append(e.celExtensions, extensions...)
	}
}

// renderState carries the per-Render context and cost budget through the recursive walk so
// every expression in one tree - values, list items, and dynamic map keys alike - draws from
// the same pool and observes the same cancellation.
type renderState struct {
	ctx    context.Context
	budget *CostBudget
}

// Render walks the provided structure and evaluates CEL expressions against the supplied inputs.
//
// Evaluation is bounded twice: each expression is capped by the engine's cost limit, and the
// whole walk is charged against the cost budget carried by ctx (see WithCostBudget). Callers
// without a budget on the context get one scoped to this Render. A cancelled or expired ctx
// interrupts evaluation, and the resulting error wraps context.Canceled or
// context.DeadlineExceeded so callers can classify it.
func (e *Engine) Render(ctx context.Context, data any, inputs map[string]any) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	budget := CostBudgetFrom(ctx)
	if budget == nil {
		budget = NewCostBudget(e.renderBudgetDefault())
	}
	return e.render(&renderState{ctx: ctx, budget: budget}, data, inputs)
}

func (e *Engine) render(rs *renderState, data any, inputs map[string]any) (any, error) {
	// Every value in the tree passes through here, so this is where a walk notices that the
	// render deadline expired or the reconcile was cancelled. Checking only in renderString
	// is not enough: a list of numbers or booleans reaches none of the expression paths and
	// none of the string path either, so a cancelled render would walk it to the end. The
	// check costs a field read per value and charges nothing against the cost budget - no
	// CEL work is being paid for.
	if ctxErr := rs.ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("template render interrupted: %w", ctxErr)
	}

	switch v := data.(type) {
	case string:
		return e.renderString(rs, v, inputs)
	case map[string]any:
		result := make(map[string]any, len(v))
		// Walk keys in sorted order. Go randomizes map iteration, and this walk both charges
		// the cost budget and stops at the first failing expression, so an unordered walk makes
		// "which expression is named in the error" vary run to run. Reconcilers put that text
		// in a status condition; a message that changes on every reconcile rewrites the
		// condition, and each rewrite is a watch event that immediately re-triggers the
		// reconcile. The rendered result is a map, so ordering the walk changes no output.
		for _, key := range slices.Sorted(maps.Keys(v)) {
			value := v[key]
			renderedKey, err := e.renderString(rs, key, inputs)
			if err != nil {
				return nil, err
			}
			evaluatedKey := key
			if keyStr, ok := renderedKey.(string); ok {
				evaluatedKey = keyStr
			} else if renderedKey != key {
				// Dynamic key expression evaluated to non-string
				return nil, fmt.Errorf("dynamic map key '%s' must evaluate to a string, got %T: %v", TruncateFragment(key), renderedKey, FormatFragment(renderedKey))
			}

			renderedValue, err := e.render(rs, value, inputs)
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
			rendered, err := e.render(rs, item, inputs)
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
func (e *Engine) renderString(rs *renderState, str string, inputs map[string]any) (any, error) {
	// Map keys reach here without passing through render, so the walk's cancellation check
	// is repeated for them. The expression paths below are interrupted by ContextEval, but a
	// template that is almost entirely static never reaches an evaluation and would otherwise
	// run the whole traversal out under an expired render deadline.
	if ctxErr := rs.ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("template render interrupted: %w", ctxErr)
	}

	spans, err := findCELSpans(str)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return str, nil
	}

	// Standalone expression: return native type (e.g., ${spec.replicas} returns int, not "3")
	trimmed := strings.TrimSpace(str)
	if len(spans) == 1 && str[spans[0].start:spans[0].end] == trimmed {
		result, err := e.evaluateCEL(rs, spans[0].inner, inputs)
		return normalizeCELResult(result, err)
	}

	// Interpolation mode: substitute all expressions into the string.
	//
	// This is a single left-to-right pass over str rather than one strings.Replace per
	// expression. Replace rebuilds the whole string every time, so a template holding many
	// expressions copied the accumulated result once per expression - quadratic in the
	// template size, and invisible to the cost guards because a constant expression like
	// ${"x"} evaluates for almost nothing. Walking the spans in order costs one pass.
	var b strings.Builder
	b.Grow(len(str))
	cursor := 0
	for _, span := range spans {
		value, err := e.evaluateCEL(rs, span.inner, inputs)
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

		b.WriteString(str[cursor:span.start])
		b.WriteString(replacement)
		cursor = span.end
	}
	b.WriteString(str[cursor:])

	return b.String(), nil
}

// CELMatch represents a CEL expression found in a template string.
type CELMatch struct {
	FullExpr  string // The complete ${...} expression including delimiters
	InnerExpr string // The CEL expression content without ${ and }
}

// ErrNestedExpression is returned when nested CEL expressions are found.
var ErrNestedExpression = errors.New("nested CEL expressions must be quoted")

// FindCELExpressions locates all ${...} expression markers within a string.
//
// This function performs brace-balanced parsing to handle nested curly braces correctly.
// For example, in "${merge({a: 1}, {b: 2})}", the parser counts opening and closing braces
// to identify the complete expression boundary.
//
// The algorithm uses a brace counter that increments on '{' and decrements on '}'.
// When the counter returns to zero, we've found the matching closing brace.
//
// Returns:
//   - FullExpr: the complete ${...} expression including delimiters
//   - InnerExpr: the CEL expression content without ${ and }
//
// Example:
//   - Input: "image:${spec.image}:${spec.tag}"
//   - Output: [{FullExpr: "${spec.image}", InnerExpr: "spec.image"},
//     {FullExpr: "${spec.tag}", InnerExpr: "spec.tag"}]
func FindCELExpressions(str string) ([]CELMatch, error) {
	spans, err := findCELSpans(str)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, nil
	}
	matches := make([]CELMatch, len(spans))
	for i, span := range spans {
		matches[i] = CELMatch{
			FullExpr:  str[span.start:span.end],
			InnerExpr: span.inner,
		}
	}
	return matches, nil
}

// celSpan is a CEL expression located within its source string: the byte range the
// ${...} marker occupies, plus the expression text inside it. Interpolation substitutes
// at these offsets, which is what keeps it to a single pass over the string.
type celSpan struct {
	start int // index of the '$' opening the marker
	end   int // index one past the '}' closing it
	inner string
}

// findCELSpans is the scanner behind FindCELExpressions, returning the located form.
func findCELSpans(str string) ([]celSpan, error) {
	var matches []celSpan
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
		inSingleQuote := false
		inDoubleQuote := false
		escaped := false
		for pos < len(str) && brace > 0 {
			switch str[pos] {
			case '\\':
				if inSingleQuote || inDoubleQuote {
					escaped = !escaped
				}
			case '\'':
				if !inDoubleQuote && !escaped {
					inSingleQuote = !inSingleQuote
				}
				escaped = false
			case '"':
				if !inSingleQuote && !escaped {
					inDoubleQuote = !inDoubleQuote
				}
				escaped = false
			case '{':
				if !inSingleQuote && !inDoubleQuote {
					brace++
				}
				escaped = false
			case '}':
				if !inSingleQuote && !inDoubleQuote {
					brace--
				}
				escaped = false
			case '$':
				if !inSingleQuote && !inDoubleQuote && pos+1 < len(str) && str[pos+1] == '{' {
					return nil, fmt.Errorf("%w: %s", ErrNestedExpression, TruncateFragment(str[start:pos+2]))
				}
				escaped = false
			default:
				escaped = false
			}
			pos++
		}

		if brace == 0 {
			matches = append(matches, celSpan{
				start: start,
				end:   pos,
				inner: str[start+2 : pos-1],
			})
			i = pos
		} else {
			// Unclosed brace - stop parsing
			break
		}
	}
	return matches, nil
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

func (e *Engine) evaluateCEL(rs *renderState, expression string, inputs map[string]any) (any, error) {
	// Refuse before doing any work when the allowance is already spent. cel-go reports what
	// an evaluation cost only once it returns, so the budget is always charged after the
	// fact; without this check the overshoot is unbounded. Two callers make that concrete:
	// the error aggregators in the pipelines keep evaluating past a failed expression, and
	// an expression that fails for its own reason returns that reason rather than the
	// breach - so nothing would ever stop the sweep, and each remaining expression would
	// burn up to the full per-expression limit. Checking here bounds the overshoot to the
	// one expression that crossed the line.
	if budgetErr := rs.budget.exceeded(); budgetErr != nil {
		return nil, fmt.Errorf("CEL evaluation skipped for expression '%s': %w", TruncateFragment(expression), budgetErr)
	}

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
			return nil, fmt.Errorf("CEL compilation error in expression '%s': %w", TruncateFragment(expression), issues.Err())
		}

		// The cost limit is frozen into the cached program; changing it requires a controller restart.
		// CustomFunctionCostOptions feeds the same cost tracker the limit bounds, so our oc_*
		// overloads are metered by the data they traverse instead of cel-go's O(1) default.
		programOptions := append([]cel.ProgramOption{
			cel.CostLimit(e.costLimit),
			cel.InterruptCheckFrequency(interruptCheckFrequency),
		}, CustomFunctionCostOptions()...)
		program, err = env.Program(ast, programOptions...)
		if err != nil {
			return nil, fmt.Errorf("CEL program creation error for expression '%s': %w", TruncateFragment(expression), err)
		}

		// Store in cache for future use
		e.cache.SetProgram(envKey, expression, program)
	}

	// ContextEval, rather than Eval, so a cancelled or expired context interrupts a long
	// running comprehension instead of running it to completion.
	result, details, err := program.ContextEval(rs.ctx, inputs)

	// Read the cost before any early return so omit() and failed evaluations are still accounted for.
	var actual uint64
	if details != nil {
		if ac := details.ActualCost(); ac != nil {
			actual = *ac
		}
	}
	// Charge the budget on every path, including the failing ones: work that was performed
	// counts even when its result is discarded. A concrete evaluation failure is reported
	// ahead of the budget breach because the underlying cause names what to fix. Deferring
	// the breach that way is safe because the spend is recorded regardless, and the check at
	// the top of this function turns it into a refusal the moment anything asks to evaluate
	// again.
	budgetErr := rs.budget.add(actual)

	if err != nil {
		if err.Error() == omitErrMsg {
			if budgetErr != nil {
				return nil, fmt.Errorf("CEL evaluation error in expression '%s': %w", TruncateFragment(expression), budgetErr)
			}
			return omitSentinel, nil
		}
		// cel-go signals an interrupted evaluation without consistently wrapping the context's
		// own error, so wrap it here: callers classify cancellation and deadlines with
		// errors.Is against context.Canceled / context.DeadlineExceeded.
		if ctxErr := rs.ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("CEL evaluation interrupted in expression '%s': %w: %w", TruncateFragment(expression), ctxErr, err)
		}
		var cancelled interpreter.EvalCancelledError
		if errors.As(err, &cancelled) && cancelled.Cause == interpreter.CostLimitExceeded {
			return nil, fmt.Errorf("CEL evaluation error in expression '%s': %w: %w", TruncateFragment(expression), ErrCostLimitExceeded, err)
		}
		return nil, fmt.Errorf("CEL evaluation error in expression '%s': %w", TruncateFragment(expression), err)
	}

	if budgetErr != nil {
		return nil, fmt.Errorf("CEL evaluation error in expression '%s': %w", TruncateFragment(expression), budgetErr)
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
	env, err := buildEnv(inputs, e.celExtensions)
	if err != nil {
		return nil, err
	}

	// Store in cache
	e.cache.SetEnv(cacheKey, env)
	return env, nil
}

// buildEnv wires up CEL with the helper surface area expected by our templating story so authors
// can reuse common snippets like `omit`, `merge`, and `sanitizeK8sResourceName`.
func buildEnv(inputs map[string]any, celExtensions []cel.EnvOption) (*cel.Env, error) {
	envOptions := BaseCELExtensions()

	// Add variables for all inputs
	for key := range inputs {
		envOptions = append(envOptions, cel.Variable(key, cel.DynType))
	}

	// Add custom CEL extensions (e.g., configuration helpers from context package)
	envOptions = append(envOptions, celExtensions...)

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
