// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// ResolveParameters maps every hook parameter to a string value.
//
// Precedence per parameter: a fixed value wins and cannot be overridden; a
// From expression is evaluated against the deployment context and may be
// replaced by the binding only when Overridable; otherwise the binding value,
// then Default. A Required parameter the binding does not supply is an error,
// as is a binding key that names a fixed parameter or one the hook does not
// declare. Parameters that resolve to nothing are omitted from the result.
func ResolveParameters(ctx context.Context, engine *template.Engine, hook *openchoreov1alpha1.HookSpec,
	binding openchoreov1alpha1.HookBinding, deployment map[string]any) (map[string]string, error) {
	supplied, err := BindingParameters(binding)
	if err != nil {
		return nil, err
	}
	declared := make(map[string]struct{}, len(hook.Parameters))
	out := make(map[string]string, len(hook.Parameters))

	for _, p := range hook.Parameters {
		declared[p.Name] = struct{}{}
		bound, hasBound := supplied[p.Name]

		switch {
		case p.Value != nil:
			if hasBound {
				return nil, fmt.Errorf("parameter %q is fixed by the hook and cannot be set by binding %q", p.Name, binding.Name)
			}
			out[p.Name] = *p.Value

		case p.From != "":
			if hasBound {
				if !p.Overridable {
					return nil, fmt.Errorf("parameter %q is computed by the hook and is not overridable", p.Name)
				}
				out[p.Name] = bound
				continue
			}
			v, err := engine.Render(ctx, p.From, deployment)
			if err != nil {
				return nil, fmt.Errorf("parameter %q: evaluating %q: %w", p.Name, p.From, err)
			}
			s, err := stringify(v)
			if err != nil {
				return nil, fmt.Errorf("parameter %q: %w", p.Name, err)
			}
			out[p.Name] = s

		case hasBound:
			out[p.Name] = bound

		case p.Default != nil:
			out[p.Name] = *p.Default

		case p.Required:
			return nil, fmt.Errorf("parameter %q is required but binding %q does not supply it", p.Name, binding.Name)
		}
	}

	undeclared := make([]string, 0)
	for k := range supplied {
		if _, ok := declared[k]; !ok {
			undeclared = append(undeclared, k)
		}
	}
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		return nil, fmt.Errorf("binding %q sets parameters the hook does not declare: %v", binding.Name, undeclared)
	}
	return out, nil
}

// BindingParameters decodes a binding's parameters into a flat string map.
// Scalars are rendered with their JSON text; objects and lists are kept as JSON.
func BindingParameters(binding openchoreov1alpha1.HookBinding) (map[string]string, error) {
	out := map[string]string{}
	if binding.Parameters == nil || len(binding.Parameters.Raw) == 0 {
		return out, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(binding.Parameters.Raw, &raw); err != nil {
		return nil, fmt.Errorf("binding %q: parameters must be a JSON object: %w", binding.Name, err)
	}
	for k, v := range raw {
		s, err := stringify(v)
		if err != nil {
			return nil, fmt.Errorf("binding %q parameter %q: %w", binding.Name, k, err)
		}
		out[k] = s
	}
	return out, nil
}

func stringify(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case int:
		return strconv.Itoa(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case uint64:
		return strconv.FormatUint(t, 10), nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("value of type %T cannot be passed as a string parameter: %w", v, err)
		}
		return string(b), nil
	}
}
