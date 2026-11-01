// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package schemavalidator

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/template"
	"gopkg.in/yaml.v3"
)

// validateEmbeddedBindings resolves only bindings with concrete, available
// inputs through the production resolver. It never manufactures context roots.
func validateEmbeddedBindings(ctx context.Context, r Resource, definition, parameters, configs *yaml.Node, envAvailable bool, lookup func(string, string) []*yaml.Node) []Finding {
	bindings := nodeAt(definition, "traits")
	if bindings == nil || bindings.Kind != yaml.SequenceNode {
		return nil
	}
	params, ok := runtimeValues(nodeAt(definition, "parameters"), parameters)
	if !ok {
		return nil
	}
	roots := map[string]bool{"parameters": true}
	activation := map[string]any{"parameters": params}
	if envAvailable {
		env, ok := runtimeValues(nodeAt(definition, "environmentConfigs"), configs)
		if !ok {
			return nil
		}
		roots["environmentConfigs"] = true
		activation["environmentConfigs"] = env
	}
	engine := template.NewEngineWithOptions(template.WithCELExtensions(pipelinecontext.CELExtensions()...))
	var findings []Finding
	for _, binding := range bindings.Content {
		kind := scalar(nodeAt(binding, "kind"))
		if kind == "" {
			kind = "Trait"
		}
		name := scalar(nodeAt(binding, "name"))
		defs := lookup(kind, name)
		if len(defs) != 1 {
			if len(defs) > 1 {
				findings = append(findings, nodeFinding(r, binding, "pre-render-ambiguous-definition", "ambiguous embedded trait "+name))
			}
			continue
		}
		if !bindingsAvailable(binding, roots) {
			continue
		}
		raw, err := rawNode(binding)
		if err != nil {
			continue
		}
		var embedded v1alpha1.ComponentTypeTrait
		if json.Unmarshal(raw.Raw, &embedded) != nil {
			continue
		}
		resolved, env, err := pipelinecontext.ResolveEmbeddedTraitBindings(ctx, engine, embedded, activation)
		if err != nil {
			findings = append(findings, nodeFinding(r, binding, "pre-render-validation-failed", err.Error()))
			continue
		}
		for _, section := range []struct {
			name   string
			values map[string]any
			code   string
		}{{"parameters", resolved, "schema-parameters-mismatch"}, {"environmentConfigs", env, "schema-environment-configs-mismatch"}} {
			schema, err := sectionNode(nodeAt(defs[0], section.name))
			if err != nil {
				continue
			}
			n := mapValueNode(section.values)
			found := validateValueNode(r, schema, n, "embedded trait "+name)
			for i := range found {
				found[i].Code = section.code
				found[i].Line, found[i].Column = position(nodeAt(binding, section.name))
				if found[i].Line < 1 {
					found[i].Line, found[i].Column = position(binding)
				}
			}
			findings = append(findings, found...)
		}
		runtime := evaluateRulesAt(ctx, r, defs[0], mapValueNode(resolved), mapValueNode(env), true, kind)
		for i := range runtime {
			runtime[i].Line, runtime[i].Column = position(binding)
		}
		findings = append(findings, runtime...)
	}
	return findings
}
func mapValueNode(values map[string]any) *yaml.Node {
	if values == nil {
		values = map[string]any{}
	}
	raw, _ := json.Marshal(values)
	var node yaml.Node
	if yaml.Unmarshal(raw, &node) != nil || len(node.Content) == 0 {
		return nil
	}
	return node.Content[0]
}
func bindingsAvailable(node *yaml.Node, roots map[string]bool) bool {
	if node == nil {
		return true
	}
	if node.Kind == yaml.ScalarNode && strings.Contains(node.Value, "${") {
		// Whole-value CEL can be dependency checked without approximating template parsing.
		expression := strings.TrimSpace(node.Value)
		if !strings.HasPrefix(expression, "${") || !strings.HasSuffix(expression, "}") {
			return false
		}
		env, _ := cel.NewEnv()
		ast, issues := env.Parse(expression[2 : len(expression)-1])
		if issues != nil && issues.Err() != nil {
			return false
		}
		return availableExpr(ast.Expr(), roots)
	}
	for _, child := range node.Content {
		if !bindingsAvailable(child, roots) {
			return false
		}
	}
	return true
}
