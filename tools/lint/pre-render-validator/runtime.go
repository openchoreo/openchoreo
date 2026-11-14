// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/template"
	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
)

// Runtime checks are deliberately restricted to roots whose concrete values
// are available. No metadata, secrets, workload or dependency placeholders.
type runtimeValidator struct{}

func (runtimeValidator) Supports(k string) bool {
	return k == "Component" || k == valueComponentRelease || k == "Project" || k == valueProjectRelease
}
func (runtimeValidator) Validate(ctx context.Context, r Resource, c *ValidationContext) []Finding {
	if r.Document.Kind == valueProjectRelease {
		return evaluateRulesAt(ctx, r, nodeAt(r.Document.Spec, "projectType", "spec"), nodeAt(r.Document.Spec, valueParameters), nil, false, valueProjectType)
	}
	if r.Document.Kind == "Project" {
		ref := nodeAt(r.Document.Spec, "type")
		kind := scalar(nodeAt(ref, "kind"))
		if kind == "" {
			kind = valueProjectType
		}
		ns := r.Document.Metadata.Namespace
		if kind == "ClusterProjectType" {
			ns = ""
		}
		defs := c.Resolve(ResourceKey{r.Document.APIVersion, kind, ns, scalar(nodeAt(ref, "name"))})
		if len(defs) != 1 {
			return nil
		}
		return evaluateRulesAt(ctx, r, defs[0].Document.Spec, nodeAt(r.Document.Spec, valueParameters), nil, false, kind)
	}
	if r.Document.Kind == valueComponentRelease {
		def := nodeAt(r.Document.Spec, "componentType", "spec")
		return evaluateRulesAt(ctx, r, def, nodeAt(r.Document.Spec, "componentProfile", valueParameters), nil, false, valueComponentType)
	}
	var comp v1alpha1.Component
	if decodeTyped(r.Document, &comp) != nil {
		return nil
	}
	name := comp.Spec.ComponentType.Name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	kind := string(comp.Spec.ComponentType.Kind)
	if kind == "" {
		kind = valueComponentType
	}
	namespace := comp.Namespace
	if kind == valueClusterComponentType {
		namespace = ""
	}
	defs := c.Resolve(ResourceKey{r.Document.APIVersion, kind, namespace, name})
	if len(defs) != 1 {
		return nil
	}
	def := defs[0].Document.Spec
	findings := evaluateRulesAt(ctx, r, def, nodeAt(r.Document.Spec, valueParameters), nil, false, kind)
	findings = append(findings, validateEmbeddedBindings(ctx, r, def, nodeAt(r.Document.Spec, valueParameters), nil, false, func(k, n string) []*yaml.Node {
		ns := comp.Namespace
		if k == valueClusterTrait {
			ns = ""
		}
		matches := c.Resolve(ResourceKey{r.Document.APIVersion, k, ns, n})
		specs := make([]*yaml.Node, 0, len(matches))
		for _, m := range matches {
			specs = append(specs, m.Document.Spec)
		}
		return specs
	})...)

	for i, instance := range comp.Spec.Traits {
		kind := string(instance.Kind)
		if kind == "" {
			kind = valueTrait
		}
		namespace := comp.Namespace
		if kind == valueClusterTrait {
			namespace = ""
		}
		defs := c.Resolve(ResourceKey{r.Document.APIVersion, kind, namespace, instance.Name})
		if len(defs) != 1 {
			continue
		}
		traits := nodeAt(r.Document.Spec, valueTraits)
		if traits == nil || i >= len(traits.Content) {
			continue
		}
		findings = append(findings, evaluateRulesAt(ctx, r, defs[0].Document.Spec, nodeAt(traits.Content[i], valueParameters), nil, false, kind)...)
	}
	return findings
}
func evaluateRulesAt(ctx context.Context, r Resource, definition, values, env *yaml.Node, envAvailable bool, kind string) []Finding {
	if definition == nil {
		return nil
	}
	// Existing CEL checker owns syntax/type/reference errors. Runtime never
	// repeats them; snapshot static checking is performed by snapshotValidator.
	pair, err := celvalidator.ExtractSchemas(definition)
	if err != nil {
		return nil
	}
	view := specView(r, kind, definition)
	static, err := celvalidator.ValidateDocument(view.Document, pair)
	if err != nil || len(static) > 0 {
		return nil
	}
	rules := nodeAt(definition, "preRenderValidations")
	if rules == nil {
		rules = nodeAt(definition, "validations")
	}
	if rules == nil || rules.Kind != yaml.SequenceNode {
		return nil
	}
	params, ok := runtimeValues(nodeAt(definition, valueParameters), values)
	if !ok {
		return nil
	}
	roots := map[string]bool{valueParameters: true}
	activation := map[string]any{valueParameters: params}
	if envAvailable {
		configs, ok := runtimeValues(nodeAt(definition, valueEnvironmentConfigs), env)
		if !ok {
			return nil
		}
		roots[valueEnvironmentConfigs] = true
		activation[valueEnvironmentConfigs] = configs
	}
	engine := template.NewEngineWithOptions(template.WithCELExtensions(pipelinecontext.CELExtensions()...))
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	parser, _ := cel.NewEnv()
	var findings []Finding
	for _, entry := range rules.Content {
		rule := scalar(nodeAt(entry, "rule"))
		expr := strings.TrimSpace(rule)
		if !strings.HasPrefix(expr, "${") || !strings.HasSuffix(expr, "}") {
			continue
		}
		ast, issues := parser.Parse(expr[2 : len(expr)-1])
		if issues != nil && issues.Err() != nil {
			continue
		}
		parsed, err := cel.AstToParsedExpr(ast)
		if err != nil {
			continue
		}
		if !availableExpr(parsed.GetExpr(), roots) {
			continue
		}
		err = renderer.EvaluateValidationRules(ctx, engine, []v1alpha1.ValidationRule{{Rule: rule, Message: scalar(nodeAt(entry, "message"))}}, activation)
		if err != nil {
			target := values
			switch r.Document.Kind {
			case "ReleaseBinding":
				target = nodeAt(r.Document.Spec, "componentTypeEnvironmentConfigs")
			case valueProjectReleaseBinding:
				target = nodeAt(r.Document.Spec, valueEnvironmentConfigs)
			}
			if precise := ruleValueLocation(parsed.GetExpr(), values, env); precise != nil {
				target = precise
			}
			findings = append(findings, nodeFinding(r, target, "pre-render-validation-failed", err.Error()))
		}
	}
	return findings
}
func runtimeValues(section, values *yaml.Node) (map[string]any, bool) {
	def, err := sectionNode(section)
	if err != nil {
		return nil, false
	}
	raw, err := rawNode(values)
	if err != nil {
		return nil, false
	}
	result := map[string]any{}
	if raw != nil {
		if json.Unmarshal(raw.Raw, &result) != nil || result == nil {
			return nil, false
		}
	}
	if def != nil {
		structural, jsonSchema, err := schema.ResolveSectionToBundle(def)
		if err != nil {
			return nil, false
		}
		result = schema.ApplyDefaults(result, structural)
		if schema.ValidateWithJSONSchema(result, jsonSchema) != nil {
			return nil, false
		}
	}
	return result, true
}
func availableExpr(e *exprpb.Expr, scope map[string]bool) bool {
	if e == nil {
		return true
	}
	switch x := e.ExprKind.(type) {
	case *exprpb.Expr_IdentExpr:
		return scope[x.IdentExpr.Name]
	case *exprpb.Expr_SelectExpr:
		return availableExpr(x.SelectExpr.Operand, scope)
	case *exprpb.Expr_CallExpr:
		if !availableExpr(x.CallExpr.Target, scope) {
			return false
		}
		for _, a := range x.CallExpr.Args {
			if !availableExpr(a, scope) {
				return false
			}
		}
	case *exprpb.Expr_ListExpr:
		for _, a := range x.ListExpr.Elements {
			if !availableExpr(a, scope) {
				return false
			}
		}
	case *exprpb.Expr_StructExpr:
		for _, a := range x.StructExpr.Entries {
			if !availableExpr(a.GetMapKey(), scope) || !availableExpr(a.Value, scope) {
				return false
			}
		}
	case *exprpb.Expr_ComprehensionExpr:
		a := x.ComprehensionExpr
		if !availableExpr(a.IterRange, scope) || !availableExpr(a.AccuInit, scope) {
			return false
		}
		local := map[string]bool{}
		for k, v := range scope {
			local[k] = v
		}
		local[a.IterVar] = true
		local[a.AccuVar] = true
		return availableExpr(a.LoopCondition, local) && availableExpr(a.LoopStep, local) && availableExpr(a.Result, local)
	}
	return true
}

// ruleValueLocation narrows a failed rule to a supplied value only when all
// input references identify one path. Multi-input rules retain their container
// location rather than claiming one of several values caused the failure.
func ruleValueLocation(expr *exprpb.Expr, parameters, configs *yaml.Node) *yaml.Node {
	paths := map[string][]string{}
	var visit func(*exprpb.Expr)
	visit = func(e *exprpb.Expr) {
		if e == nil {
			return
		}
		if path := ruleInputPath(e); len(path) > 0 {
			paths[strings.Join(path, "\x00")] = path
			return
		}
		switch x := e.ExprKind.(type) {
		case *exprpb.Expr_SelectExpr:
			visit(x.SelectExpr.Operand)
		case *exprpb.Expr_CallExpr:
			visit(x.CallExpr.Target)
			for _, arg := range x.CallExpr.Args {
				visit(arg)
			}
		case *exprpb.Expr_ListExpr:
			for _, item := range x.ListExpr.Elements {
				visit(item)
			}
		case *exprpb.Expr_StructExpr:
			for _, item := range x.StructExpr.Entries {
				visit(item.GetMapKey())
				visit(item.Value)
			}
		case *exprpb.Expr_ComprehensionExpr:
			c := x.ComprehensionExpr
			visit(c.IterRange)
			visit(c.AccuInit)
			visit(c.LoopCondition)
			visit(c.LoopStep)
			visit(c.Result)
		}
	}
	visit(expr)
	if len(paths) != 1 {
		return nil
	}
	for _, path := range paths {
		if len(path) < 2 {
			return nil
		}
		root := parameters
		if path[0] == valueEnvironmentConfigs {
			root = configs
		}
		return nodeAt(root, path[1:]...)
	}
	return nil
}

func ruleInputPath(e *exprpb.Expr) []string {
	if e == nil {
		return nil
	}
	switch x := e.ExprKind.(type) {
	case *exprpb.Expr_IdentExpr:
		if x.IdentExpr.Name == valueParameters || x.IdentExpr.Name == valueEnvironmentConfigs {
			return []string{x.IdentExpr.Name}
		}
	case *exprpb.Expr_SelectExpr:
		if p := ruleInputPath(x.SelectExpr.Operand); p != nil {
			return append(p, x.SelectExpr.Field)
		}
	}
	return nil
}
