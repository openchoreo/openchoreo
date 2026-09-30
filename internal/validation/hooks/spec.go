// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package hooks validates Hook / ClusterHook specs and the HookBindings an
// Environment declares in spec.hooks. It is shared by the hook, clusterhook
// and environment admission webhooks.
package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// omitValue keeps large or sensitive values out of error messages.
var omitValue = field.OmitValueType{}

// DeploymentContextVariable is the root CEL variable a HookParameter.From expression
// is evaluated against. The gate populates it with the deployment context.
const DeploymentContextVariable = "deployment"

var (
	celEnvOnce sync.Once
	celEnv     *cel.Env
	celEnvErr  error
)

// fromCELEnv returns the CEL environment used to compile HookParameter.From
// expressions: the platform's base extensions plus the deployment context root.
func fromCELEnv() (*cel.Env, error) {
	celEnvOnce.Do(func() {
		opts := append(template.BaseCELExtensions(), cel.Variable(DeploymentContextVariable, cel.DynType))
		celEnv, celEnvErr = cel.NewEnv(opts...)
	})
	return celEnv, celEnvErr
}

// ValidateHookSpec validates a Hook or ClusterHook spec.
// isCluster restricts workflowRef to ClusterWorkflow, because a cluster-scoped
// hook has no namespace to resolve a namespaced Workflow in.
func ValidateHookSpec(spec *v1alpha1.HookSpec, isCluster bool, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if spec == nil {
		return append(allErrs, field.Required(path, "spec is required"))
	}

	if spec.Type != "" && spec.Type != v1alpha1.HookTypeWorkflow {
		allErrs = append(allErrs, field.NotSupported(path.Child("type"), spec.Type,
			[]string{string(v1alpha1.HookTypeWorkflow)}))
	}

	refPath := path.Child("workflowRef")
	if spec.WorkflowRef == nil {
		allErrs = append(allErrs, field.Required(refPath, "workflowRef is required for a Workflow hook"))
	} else {
		if spec.WorkflowRef.Name == "" {
			allErrs = append(allErrs, field.Required(refPath.Child("name"), "workflow name is required"))
		}
		switch spec.WorkflowRef.Kind {
		case "", v1alpha1.WorkflowRefKindClusterWorkflow:
		case v1alpha1.WorkflowRefKindWorkflow:
			if isCluster {
				allErrs = append(allErrs, field.Invalid(refPath.Child("kind"), spec.WorkflowRef.Kind,
					"a ClusterHook may only reference a ClusterWorkflow"))
			}
		default:
			allErrs = append(allErrs, field.NotSupported(refPath.Child("kind"), spec.WorkflowRef.Kind,
				[]string{string(v1alpha1.WorkflowRefKindWorkflow), string(v1alpha1.WorkflowRefKindClusterWorkflow)}))
		}
	}

	allErrs = append(allErrs, validateParameters(spec.Parameters, path.Child("parameters"))...)
	allErrs = append(allErrs, validateEnabledTo(spec.EnabledTo, path.Child("enabledTo"))...)
	return allErrs
}

func validateParameters(params []v1alpha1.HookParameter, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	seen := make(map[string]int, len(params))
	for i := range params {
		p := &params[i]
		pPath := path.Index(i)

		if p.Name == "" {
			allErrs = append(allErrs, field.Required(pPath.Child("name"), "parameter name is required"))
		} else if prev, dup := seen[p.Name]; dup {
			allErrs = append(allErrs, field.Duplicate(pPath.Child("name"),
				fmt.Sprintf("parameter %q is already declared at index %d", p.Name, prev)))
		} else {
			seen[p.Name] = i
		}

		allErrs = append(allErrs, validateParameterSource(p, pPath)...)

		if p.Schema != nil && len(p.Schema.Raw) > 0 {
			allErrs = append(allErrs, validateSchemaFragment(p.Schema.Raw, pPath.Child("schema"))...)
		}
	}
	return allErrs
}

// validateParameterSource enforces "exactly one source": the gate resolves a
// parameter by which field is set, so two sources would make precedence ambiguous
// and zero sources would leave the workflow input unset.
func validateParameterSource(p *v1alpha1.HookParameter, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	var sources []string
	if p.Value != nil {
		sources = append(sources, "value")
	}
	if p.From != "" {
		sources = append(sources, "from")
	}
	if p.Default != nil {
		sources = append(sources, "default")
	}
	if p.Required {
		sources = append(sources, "required")
	}

	switch len(sources) {
	case 0:
		allErrs = append(allErrs, field.Required(path,
			"exactly one of value, from, default or required must be set"))
	case 1:
	default:
		allErrs = append(allErrs, field.Invalid(path, omitValue,
			fmt.Sprintf("exactly one of value, from, default or required must be set, got %s",
				strings.Join(sources, " and "))))
	}

	if p.Overridable && p.From == "" {
		allErrs = append(allErrs, field.Invalid(path.Child("overridable"), p.Overridable,
			"overridable is only meaningful together with from"))
	}

	if p.From != "" {
		allErrs = append(allErrs, validateFromExpression(p.From, path.Child("from"))...)
	}
	return allErrs
}

// validateFromExpression requires at least one ${...} expression and compiles each
// against the deployment context, so a typo is caught at admission instead of on
// the first deployment into the environment.
func validateFromExpression(from string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	matches, err := template.FindCELExpressions(from)
	if err != nil {
		return append(allErrs, field.Invalid(path, from, fmt.Sprintf("invalid CEL expression: %v", err)))
	}
	if len(matches) == 0 {
		return append(allErrs, field.Invalid(path, from, "from must contain a ${...} CEL expression"))
	}

	env, err := fromCELEnv()
	if err != nil {
		return append(allErrs, field.InternalError(path, fmt.Errorf("failed to build CEL environment: %w", err)))
	}
	for _, m := range matches {
		if _, issues := env.Compile(m.InnerExpr); issues != nil && issues.Err() != nil {
			allErrs = append(allErrs, field.Invalid(path, m.FullExpr,
				fmt.Sprintf("CEL compilation failed: %v", issues.Err())))
		}
	}
	return allErrs
}

// validateSchemaFragment strict-decodes an OpenAPI v3 fragment so unknown keys
// ("types" for "type") are rejected instead of silently ignored.
func validateSchemaFragment(raw []byte, path *field.Path) field.ErrorList {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return field.ErrorList{field.Invalid(path, omitValue, fmt.Sprintf("schema must be a JSON object: %v", err))}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var props extv1.JSONSchemaProps
	if err := decoder.Decode(&props); err != nil {
		return field.ErrorList{field.Invalid(path, omitValue,
			fmt.Sprintf("schema contains unknown or invalid fields: %v", err))}
	}
	return nil
}

// validateEnabledTo checks the hook-level component type restriction: known
// kinds, non-empty names, no duplicates.
func validateEnabledTo(refs []v1alpha1.HookSubjectRef, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	seen := make(map[v1alpha1.HookSubjectRef]bool, len(refs))
	for i, r := range refs {
		rPath := path.Index(i)
		switch r.Kind {
		case v1alpha1.HookSubjectRefKindComponentType, v1alpha1.HookSubjectRefKindClusterComponentType:
		default:
			allErrs = append(allErrs, field.NotSupported(rPath.Child("kind"), r.Kind, []string{
				string(v1alpha1.HookSubjectRefKindComponentType),
				string(v1alpha1.HookSubjectRefKindClusterComponentType),
			}))
		}
		if r.Name == "" {
			allErrs = append(allErrs, field.Required(rPath.Child("name"), "type name is required"))
		}
		if seen[r] {
			allErrs = append(allErrs, field.Duplicate(rPath, r))
		}
		seen[r] = true
	}
	return allErrs
}
