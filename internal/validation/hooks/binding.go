// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hooks

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Phase names a side of a HookSet.
type Phase string

const (
	PhasePreDeploy  Phase = "preDeploy"
	PhasePostDeploy Phase = "postDeploy"
)

// HookResolver looks up the spec a binding's hookRef points at.
// It returns (nil, nil) when the hook does not exist; the caller then warns
// instead of rejecting, so an environment can be applied before its hooks.
type HookResolver func(ref v1alpha1.HookRef) (*v1alpha1.HookSpec, error)

// ValidateHookSet validates the bindings of one environment. Errors reject
// the object; warnings are returned to the client as admission warnings.
func ValidateHookSet(set *v1alpha1.HookSet, path *field.Path, resolve HookResolver) (field.ErrorList, []string) {
	allErrs := field.ErrorList{}
	var warnings []string
	if set == nil {
		return allErrs, nil
	}

	// Binding names are the key of status.gate.preDeploy/postDeploy and of the
	// retry annotation, so they must be unique across both phases of an environment.
	seen := make(map[string]string)
	check := func(phase Phase, bindings []v1alpha1.HookBinding) {
		listPath := path.Child(string(phase))
		for i := range bindings {
			b := &bindings[i]
			bPath := listPath.Index(i)
			if b.Name != "" {
				if prev, dup := seen[b.Name]; dup {
					allErrs = append(allErrs, field.Duplicate(bPath.Child("name"),
						fmt.Sprintf("binding %q is already used at %s", b.Name, prev)))
				} else {
					seen[b.Name] = bPath.String()
				}
			}
			errs, warns := validateBinding(b, phase, bPath, resolve)
			allErrs = append(allErrs, errs...)
			warnings = append(warnings, warns...)
		}
	}
	check(PhasePreDeploy, set.PreDeploy)
	check(PhasePostDeploy, set.PostDeploy)
	return allErrs, warnings
}

func validateBinding(b *v1alpha1.HookBinding, phase Phase, path *field.Path, resolve HookResolver) (field.ErrorList, []string) {
	allErrs := field.ErrorList{}
	var warnings []string

	if b.Name == "" {
		allErrs = append(allErrs, field.Required(path.Child("name"), "binding name is required"))
	}

	refPath := path.Child("hookRef")
	if b.HookRef.Name == "" {
		allErrs = append(allErrs, field.Required(refPath.Child("name"), "hook name is required"))
	}
	switch b.HookRef.Kind {
	case "", v1alpha1.HookRefKindHook, v1alpha1.HookRefKindClusterHook:
	default:
		allErrs = append(allErrs, field.NotSupported(refPath.Child("kind"), b.HookRef.Kind,
			[]string{string(v1alpha1.HookRefKindHook), string(v1alpha1.HookRefKindClusterHook)}))
	}

	switch b.Mode {
	case "", v1alpha1.HookModeSync:
		allErrs = append(allErrs, validateSyncPolicy(b, phase, path)...)
	case v1alpha1.HookModeAsync:
		// An Async hook is never awaited, so a failure policy, timeout or retry
		// count could never take effect; rejecting them avoids a false sense of safety.
		if b.OnFailure != "" {
			allErrs = append(allErrs, field.Forbidden(path.Child("onFailure"), "onFailure has no effect on an Async hook"))
		}
		if b.Timeout != "" {
			allErrs = append(allErrs, field.Forbidden(path.Child("timeout"), "timeout has no effect on an Async hook"))
		}
		if b.Retries != 0 {
			allErrs = append(allErrs, field.Forbidden(path.Child("retries"), "retries has no effect on an Async hook"))
		}
	default:
		allErrs = append(allErrs, field.NotSupported(path.Child("mode"), b.Mode,
			[]string{string(v1alpha1.HookModeSync), string(v1alpha1.HookModeAsync)}))
	}

	for i, sel := range b.AppliesTo {
		sPath := path.Child("appliesTo").Index(i)
		switch sel.Kind {
		case v1alpha1.HookSubjectSelectorKindComponentType, v1alpha1.HookSubjectSelectorKindClusterComponentType:
		default:
			allErrs = append(allErrs, field.NotSupported(sPath.Child("kind"), sel.Kind, []string{
				string(v1alpha1.HookSubjectSelectorKindComponentType),
				string(v1alpha1.HookSubjectSelectorKindClusterComponentType),
			}))
		}
		if sel.Name == "" {
			allErrs = append(allErrs, field.Required(sPath.Child("name"), "type name is required"))
		}
	}

	params, paramErrs := decodeParameters(b.Parameters, path.Child("parameters"))
	allErrs = append(allErrs, paramErrs...)

	if resolve != nil && b.HookRef.Name != "" && len(paramErrs) == 0 {
		spec, err := resolve(b.HookRef)
		switch {
		case err != nil:
			warnings = append(warnings, fmt.Sprintf("%s: could not resolve %s %q: %v",
				refPath, kindOrDefault(b.HookRef.Kind), b.HookRef.Name, err))
		case spec == nil:
			warnings = append(warnings, fmt.Sprintf("%s: %s %q not found; the binding will fail at deployment until it exists",
				refPath, kindOrDefault(b.HookRef.Kind), b.HookRef.Name))
		default:
			allErrs = append(allErrs, validateBindingParameters(params, spec, path.Child("parameters"))...)
			allErrs = append(allErrs, validateAppliesToAgainstEnabledTo(b.AppliesTo, spec.EnabledTo, path.Child("appliesTo"))...)
		}
	}

	return allErrs, warnings
}

// validateSyncPolicy checks onFailure, timeout and retries for a Sync binding.
// Block only makes sense before the release is rendered; Alert only makes sense
// once something is running that can be degraded and later acknowledged.
func validateSyncPolicy(b *v1alpha1.HookBinding, phase Phase, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch b.OnFailure {
	case "", v1alpha1.HookFailurePolicyIgnore:
	case v1alpha1.HookFailurePolicyBlock:
		if phase != PhasePreDeploy {
			allErrs = append(allErrs, field.Invalid(path.Child("onFailure"), b.OnFailure,
				"Block is only valid for preDeploy hooks; the release is already rendered when a postDeploy hook fails"))
		}
	case v1alpha1.HookFailurePolicyAlert:
		if phase != PhasePostDeploy {
			allErrs = append(allErrs, field.Invalid(path.Child("onFailure"), b.OnFailure,
				"Alert is only valid for postDeploy hooks; use Block or Ignore for preDeploy hooks"))
		}
	default:
		allErrs = append(allErrs, field.NotSupported(path.Child("onFailure"), b.OnFailure, []string{
			string(v1alpha1.HookFailurePolicyBlock),
			string(v1alpha1.HookFailurePolicyIgnore),
			string(v1alpha1.HookFailurePolicyAlert),
		}))
	}

	if b.Timeout != "" {
		d, err := time.ParseDuration(b.Timeout)
		switch {
		case err != nil:
			allErrs = append(allErrs, field.Invalid(path.Child("timeout"), b.Timeout,
				fmt.Sprintf("timeout must be a Go duration such as 30m or 2h: %v", err)))
		case d <= 0:
			allErrs = append(allErrs, field.Invalid(path.Child("timeout"), b.Timeout, "timeout must be positive"))
		}
	}

	if b.Retries < 0 || b.Retries > 5 {
		allErrs = append(allErrs, field.Invalid(path.Child("retries"), b.Retries, "retries must be between 0 and 5"))
	}
	return allErrs
}

// decodeParameters decodes binding parameters into a string map. Values are
// strings because they feed workflow inputs verbatim.
func decodeParameters(raw *runtime.RawExtension, path *field.Path) (map[string]string, field.ErrorList) {
	params := map[string]string{}
	if raw == nil || len(raw.Raw) == 0 || string(raw.Raw) == "null" {
		return params, nil
	}
	if err := json.Unmarshal(raw.Raw, &params); err != nil {
		return nil, field.ErrorList{field.Invalid(path, omitValue,
			fmt.Sprintf("parameters must be an object of string values: %v", err))}
	}
	return params, nil
}

// validateBindingParameters checks binding-supplied values against the hook's
// declared parameters: a binding may only set keys the hook left open, and must
// set every key the hook marked required.
func validateBindingParameters(params map[string]string, spec *v1alpha1.HookSpec, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	declared := make(map[string]*v1alpha1.HookParameter, len(spec.Parameters))
	for i := range spec.Parameters {
		declared[spec.Parameters[i].Name] = &spec.Parameters[i]
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p, ok := declared[k]
		switch {
		case !ok:
			allErrs = append(allErrs, field.Invalid(path.Key(k), omitValue,
				fmt.Sprintf("parameter %q is not declared by the hook", k)))
		case p.Value != nil:
			allErrs = append(allErrs, field.Forbidden(path.Key(k),
				fmt.Sprintf("parameter %q is fixed by the hook and cannot be overridden", k)))
		case p.From != "" && !p.Overridable:
			allErrs = append(allErrs, field.Forbidden(path.Key(k),
				fmt.Sprintf("parameter %q is computed by the hook and is not overridable", k)))
		}
	}

	for _, p := range spec.Parameters {
		if p.Required {
			if _, ok := params[p.Name]; !ok {
				allErrs = append(allErrs, field.Required(path.Key(p.Name),
					fmt.Sprintf("parameter %q is required by the hook", p.Name)))
			}
		}
	}
	return allErrs
}

func kindOrDefault(k v1alpha1.HookRefKind) string {
	if k == "" {
		return string(v1alpha1.HookRefKindHook)
	}
	return string(k)
}

// validateAppliesToAgainstEnabledTo rejects a binding whose appliesTo selectors
// can never intersect the hook's enabledTo list: such a binding would be
// Skipped for every component, which is a binding mistake, not a policy.
// Both lists name component types, so the comparison is direct.
func validateAppliesToAgainstEnabledTo(applies []v1alpha1.HookSubjectSelector, enabled []v1alpha1.HookSubjectRef, path *field.Path) field.ErrorList {
	if len(enabled) == 0 {
		return nil
	}
	enabledSet := make(map[v1alpha1.HookSubjectRef]bool, len(enabled))
	for _, e := range enabled {
		enabledSet[e] = true
	}
	var allErrs field.ErrorList
	for i, sel := range applies {
		kind := v1alpha1.HookSubjectRefKind(sel.Kind)
		if !enabledSet[v1alpha1.HookSubjectRef{Kind: kind, Name: sel.Name}] {
			allErrs = append(allErrs, field.Invalid(path.Index(i), fmt.Sprintf("%s/%s", sel.Kind, sel.Name),
				"the referenced hook is not enabled for this component type (see the hook's spec.enabledTo)"))
		}
	}
	return allErrs
}
