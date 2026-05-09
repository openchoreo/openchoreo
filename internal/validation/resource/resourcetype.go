// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
)

// celWrapPattern matches a single ${...}-wrapped CEL expression. The CRD
// schema enforces this pattern on IncludeWhen and ReadyWhen, so the inner
// extraction here is defensive — a non-matching value indicates the CRD layer
// failed and we should surface a clean error instead of crashing.
var celWrapPattern = regexp.MustCompile(`^\$\{([\s\S]+)\}\s*$`)

// ValidateResourceTypeSpec performs CEL-aware validation on a ResourceTypeSpec.
// Returned errors carry full field paths under basePath; callers compose the
// list across multiple specs (e.g. when re-validating a ResourceRelease snapshot).
//
// What this validates:
//   - Parameters / EnvironmentConfigs schemas parse as well-formed OpenAPI v3.
//   - Each resources[].template CEL expression compiles against the base env
//     (no applied.* in scope).
//   - Each resources[].includeWhen returns bool against the base env.
//   - Each resources[].readyWhen returns bool against the env-with-applied,
//     and any applied.<id> reference matches a declared resources[].id.
//   - Each outputs[] CEL expression (value / secretKeyRef.{name,key} /
//     configMapKeyRef.{name,key}) compiles against env-with-applied with the
//     same applied.<id> declaration check.
func ValidateResourceTypeSpec(spec *v1alpha1.ResourceTypeSpec, basePath *field.Path) field.ErrorList {
	if spec == nil {
		return nil
	}
	return validateResourceTypeSpecCommon(
		spec.Parameters,
		spec.EnvironmentConfigs,
		spec.Resources,
		spec.Outputs,
		basePath,
	)
}

// ValidateClusterResourceTypeSpec is the cluster-scoped sibling. The spec
// shape is currently identical to ResourceTypeSpec; this delegates through
// the shared inner function so future divergence (e.g. cluster-only fields)
// has a single place to extend.
func ValidateClusterResourceTypeSpec(spec *v1alpha1.ClusterResourceTypeSpec, basePath *field.Path) field.ErrorList {
	if spec == nil {
		return nil
	}
	return validateResourceTypeSpecCommon(
		spec.Parameters,
		spec.EnvironmentConfigs,
		spec.Resources,
		spec.Outputs,
		basePath,
	)
}

func validateResourceTypeSpecCommon(
	parameters *v1alpha1.SchemaSection,
	environmentConfigs *v1alpha1.SchemaSection,
	resources []v1alpha1.ResourceTypeManifest,
	outputs []v1alpha1.ResourceTypeOutput,
	basePath *field.Path,
) field.ErrorList {
	var allErrs field.ErrorList

	parametersSchema, envConfigsSchema, schemaErrs := schemautil.ExtractAndValidateSchemas(
		parameters, environmentConfigs, basePath,
	)
	allErrs = append(allErrs, schemaErrs...)

	baseEnv, err := buildResourceCELEnv(SchemaOptions{
		ParametersSchema:         parametersSchema,
		EnvironmentConfigsSchema: envConfigsSchema,
	})
	if err != nil {
		return append(allErrs, field.InternalError(basePath, fmt.Errorf("build CEL env: %w", err)))
	}

	appliedEnv, err := extendEnvWithApplied(baseEnv)
	if err != nil {
		return append(allErrs, field.InternalError(basePath, fmt.Errorf("extend CEL env with applied: %w", err)))
	}

	declaredIDs := make(map[string]bool, len(resources))
	for _, r := range resources {
		if r.ID != "" {
			declaredIDs[r.ID] = true
		}
	}

	resourcesPath := basePath.Child("resources")
	for i := range resources {
		allErrs = append(allErrs, validateResourceManifest(
			&resources[i], baseEnv, appliedEnv, declaredIDs, resourcesPath.Index(i),
		)...)
	}

	outputsPath := basePath.Child("outputs")
	for i := range outputs {
		allErrs = append(allErrs, validateResourceOutput(
			&outputs[i], appliedEnv, declaredIDs, outputsPath.Index(i),
		)...)
	}

	return allErrs
}

// validateResourceManifest validates a single resources[] entry: its template,
// includeWhen, and readyWhen.
func validateResourceManifest(
	manifest *v1alpha1.ResourceTypeManifest,
	baseEnv, appliedEnv *cel.Env,
	declaredIDs map[string]bool,
	path *field.Path,
) field.ErrorList {
	var allErrs field.ErrorList

	if manifest.IncludeWhen != "" {
		allErrs = append(allErrs, validateBoolCELField(
			manifest.IncludeWhen, baseEnv, declaredIDs, path.Child("includeWhen"),
			false, // applied.* not allowed during render-phase
		)...)
	}

	if manifest.ReadyWhen != "" {
		allErrs = append(allErrs, validateBoolCELField(
			manifest.ReadyWhen, appliedEnv, declaredIDs, path.Child("readyWhen"),
			true, // applied.<id> allowed; must match declared ids
		)...)
	}

	if manifest.Template != nil {
		allErrs = append(allErrs, validateTemplateBody(
			manifest.Template, baseEnv, declaredIDs, path.Child("template"),
		)...)
	}

	return allErrs
}

// validateResourceOutput validates a single outputs[] entry: each CEL field
// in the value / secretKeyRef / configMapKeyRef branch against the
// env-with-applied. The XOR among the three branches is enforced by a CRD
// XValidation marker (resourcetype_types.go:51); this function trusts that.
func validateResourceOutput(
	out *v1alpha1.ResourceTypeOutput,
	appliedEnv *cel.Env,
	declaredIDs map[string]bool,
	path *field.Path,
) field.ErrorList {
	var allErrs field.ErrorList

	if out.Value != "" {
		allErrs = append(allErrs, validateCELString(
			out.Value, appliedEnv, declaredIDs, path.Child("value"),
		)...)
	}

	if out.SecretKeyRef != nil {
		ref := out.SecretKeyRef
		refPath := path.Child("secretKeyRef")
		if ref.Name != "" {
			allErrs = append(allErrs, validateCELString(ref.Name, appliedEnv, declaredIDs, refPath.Child("name"))...)
		}
		if ref.Key != "" {
			allErrs = append(allErrs, validateCELString(ref.Key, appliedEnv, declaredIDs, refPath.Child("key"))...)
		}
	}

	if out.ConfigMapKeyRef != nil {
		ref := out.ConfigMapKeyRef
		refPath := path.Child("configMapKeyRef")
		if ref.Name != "" {
			allErrs = append(allErrs, validateCELString(ref.Name, appliedEnv, declaredIDs, refPath.Child("name"))...)
		}
		if ref.Key != "" {
			allErrs = append(allErrs, validateCELString(ref.Key, appliedEnv, declaredIDs, refPath.Child("key"))...)
		}
	}

	return allErrs
}

// validateBoolCELField validates a single ${...}-wrapped CEL expression that
// must return bool. Used for IncludeWhen and ReadyWhen.
//
// allowApplied controls whether applied.<id> references are permitted; when
// true, the env passed in already has applied in scope and undeclared-id
// references are flagged via AST walk.
func validateBoolCELField(
	wrapped string,
	env *cel.Env,
	declaredIDs map[string]bool,
	path *field.Path,
	allowApplied bool,
) field.ErrorList {
	inner, ok := unwrapCEL(wrapped)
	if !ok {
		return field.ErrorList{field.Invalid(path, wrapped,
			"expression must be wrapped as ${...}")}
	}

	parsed, issues := env.Parse(inner)
	if issues != nil && issues.Err() != nil {
		return field.ErrorList{field.Invalid(path, wrapped,
			fmt.Sprintf("CEL parse error: %s", issues.Err()))}
	}

	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return field.ErrorList{field.Invalid(path, wrapped,
			fmt.Sprintf("CEL type check error: %s", issues.Err()))}
	}

	output := checked.OutputType()
	if !output.IsExactType(cel.BoolType) && output != cel.DynType {
		return field.ErrorList{field.Invalid(path, wrapped,
			fmt.Sprintf("expression must return bool, got %s", output))}
	}

	if allowApplied {
		if errs := validateAppliedReferences(checked.NativeRep().Expr(), declaredIDs, path, wrapped); len(errs) > 0 {
			return errs
		}
	}

	return nil
}

// validateCELString validates every ${...} expression in a string against the
// supplied env. The string may contain zero or more expressions
// (e.g. interpolated names like "${metadata.resourceName}-conn").
//
// declaredIDs governs whether applied.<id> references match a declared
// resources[].id. The env is expected to be the env-with-applied — callers
// pass baseEnv when applied.* should be rejected outright.
func validateCELString(
	value string,
	env *cel.Env,
	declaredIDs map[string]bool,
	path *field.Path,
) field.ErrorList {
	matches, err := template.FindCELExpressions(value)
	if err != nil {
		return field.ErrorList{field.Invalid(path, value,
			fmt.Sprintf("failed to parse CEL expressions: %v", err))}
	}

	var allErrs field.ErrorList
	for _, m := range matches {
		parsed, issues := env.Parse(m.InnerExpr)
		if issues != nil && issues.Err() != nil {
			allErrs = append(allErrs, field.Invalid(path, value,
				fmt.Sprintf("invalid CEL expression %q: parse: %s", m.InnerExpr, issues.Err())))
			continue
		}
		checked, issues := env.Check(parsed)
		if issues != nil && issues.Err() != nil {
			allErrs = append(allErrs, field.Invalid(path, value,
				fmt.Sprintf("invalid CEL expression %q: type check: %s", m.InnerExpr, issues.Err())))
			continue
		}
		allErrs = append(allErrs, validateAppliedReferences(
			checked.NativeRep().Expr(), declaredIDs, path, m.InnerExpr,
		)...)
	}
	return allErrs
}

// validateTemplateBody walks a JSON-serialized template body and validates
// every CEL expression found against baseEnv (applied.* rejected outright by
// the env's lack of registration).
func validateTemplateBody(
	body *runtime.RawExtension,
	baseEnv *cel.Env,
	declaredIDs map[string]bool,
	path *field.Path,
) field.ErrorList {
	if body == nil || len(body.Raw) == 0 {
		return nil
	}

	var data any
	if err := json.Unmarshal(body.Raw, &data); err != nil {
		return field.ErrorList{field.Invalid(path, string(body.Raw),
			fmt.Sprintf("invalid JSON: %v", err))}
	}

	return walkAndValidateCEL(data, baseEnv, declaredIDs, path)
}

// walkAndValidateCEL recursively traverses a JSON-decoded structure, finds
// CEL expressions in string nodes, and validates each against env. Mirrors
// internal/validation/component.walkAndValidateCEL but operates against the
// resource-side env and applies the applied.<id> check uniformly.
func walkAndValidateCEL(
	data any,
	env *cel.Env,
	declaredIDs map[string]bool,
	path *field.Path,
) field.ErrorList {
	var errs field.ErrorList
	switch v := data.(type) {
	case string:
		errs = append(errs, validateCELString(v, env, declaredIDs, path)...)
	case map[string]any:
		// Sort keys so error ordering is stable across runs (map iteration is
		// unordered in Go and bare iteration produces flaky test output).
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if strings.Contains(k, "${") {
				errs = append(errs, validateCELString(k, env, declaredIDs, path.Key(k))...)
			}
			errs = append(errs, walkAndValidateCEL(v[k], env, declaredIDs, path.Key(k))...)
		}
	case []any:
		for i, item := range v {
			errs = append(errs, walkAndValidateCEL(item, env, declaredIDs, path.Index(i))...)
		}
	}
	return errs
}

// validateAppliedReferences walks a checked CEL AST and reports any
// applied.<id> reference whose <id> is not in declaredIDs. Both
// applied.<ident> (SelectKind) and applied["<lit>"] (CallKind with the index
// operator) are recognized.
//
// fieldValue and pathValue are used only for error wrapping — they describe
// the source field to the caller.
func validateAppliedReferences(
	expr celast.Expr,
	declaredIDs map[string]bool,
	path *field.Path,
	fieldValue string,
) field.ErrorList {
	if expr == nil {
		return nil
	}

	var undeclared []string
	seen := make(map[string]bool)

	visitor := celast.NewExprVisitor(func(e celast.Expr) {
		var id string

		switch e.Kind() {
		case celast.SelectKind:
			sel := e.AsSelect()
			operand := sel.Operand()
			if operand.Kind() == celast.IdentKind && operand.AsIdent() == "applied" {
				id = sel.FieldName()
			}
		case celast.CallKind:
			call := e.AsCall()
			if call.FunctionName() != operators.Index {
				return
			}
			args := call.Args()
			if len(args) != 2 {
				return
			}
			target := args[0]
			key := args[1]
			if target.Kind() != celast.IdentKind || target.AsIdent() != "applied" {
				return
			}
			if key.Kind() != celast.LiteralKind {
				// applied[someVar] — can't validate at compile time; skip.
				return
			}
			lit, ok := key.AsLiteral().Value().(string)
			if !ok {
				return
			}
			id = lit
		default:
			return
		}

		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		if !declaredIDs[id] {
			undeclared = append(undeclared, id)
		}
	})
	celast.PostOrderVisit(expr, visitor)

	if len(undeclared) == 0 {
		return nil
	}

	sort.Strings(undeclared)
	return field.ErrorList{field.Invalid(path, fieldValue,
		fmt.Sprintf("applied.<id> references unknown ids %v; declared ids: %v",
			undeclared, sortedKeys(declaredIDs)))}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unwrapCEL(s string) (string, bool) {
	m := celWrapPattern.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}
