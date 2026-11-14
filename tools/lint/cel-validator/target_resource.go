// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"
	structural "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel/model"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/apiserver/pkg/cel/common"

	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
)

func withTargetResource(env *cel.Env, target *yaml.Node) *cel.Env {
	return withResolvedTargetResource(env, target, targetschema.Default())
}

func withResolvedTargetResource(env *cel.Env, target *yaml.Node, resolver targetschema.Resolver) *cel.Env {
	if env == nil || target == nil {
		return withResource(env)
	}
	group, version, kind := mappingValue(target, "group"), mappingValue(target, "version"), mappingValue(target, "kind")
	if group == nil || version == nil || kind == nil {
		return withResource(env)
	}
	if group.Kind != yaml.ScalarNode || version.Kind != yaml.ScalarNode || kind.Kind != yaml.ScalarNode || group.Tag != valueStr || version.Tag != valueStr || kind.Tag != valueStr {
		return withResource(env)
	}
	key, ok := targetschema.LiteralGVK(group.Value, version.Value, kind.Value)
	if !ok {
		return withResource(env)
	}
	if resolver == nil {
		resolver = targetschema.Default()
	}
	s, err := resolver.Resolve(key)
	if err != nil || s == nil {
		return withResource(env)
	}
	decl := common.SchemaDeclType(targetModel(normalizeForCEL(targetschema.WithResourceEnvelope(s))), false)
	if decl == nil {
		return withResource(env)
	}
	decl = decl.MaybeAssignTypeName("TargetResource")
	provider := apiservercel.NewDeclTypeProvider(decl)
	opts, err := provider.EnvOptions(env.CELTypeProvider())
	if err != nil {
		return withResource(env)
	}
	opts = append(opts, cel.Variable("resource", decl.CelType()))
	typed, err := env.Extend(opts...)
	if err != nil {
		return withResource(env)
	}
	return typed
}

// The Kubernetes CRD-rule model intentionally hides preserved fields. Linting
// rendered resources must allow them, so expose only those subtrees as dyn.
// All type construction remains in the existing Kubernetes CEL model.
type resourceSchema struct{ *model.Structural }

func targetModel(s *structural.Structural) *resourceSchema {
	return &resourceSchema{&model.Structural{Structural: s}}
}
func (s *resourceSchema) IsXIntOrString() bool {
	n := s.Structural.Structural
	if n.XIntOrString || n.XPreserveUnknownFields || n.Type == "" {
		return true
	}
	if n.Type != "object" {
		return false
	}
	if n.AdditionalProperties != nil && n.AdditionalProperties.Bool && (n.AdditionalProperties.Structural == nil || len(n.Properties) > 0) {
		return true
	}
	return len(n.Properties) == 0 && n.AdditionalProperties == nil
}
func (s *resourceSchema) Properties() map[string]common.Schema {
	out := map[string]common.Schema{}
	for name, value := range s.Structural.Structural.Properties {
		copyValue := value
		out[name] = targetModel(&copyValue)
	}
	return out
}
func (s *resourceSchema) Items() common.Schema {
	if child := s.Structural.Structural.Items; child != nil {
		return targetModel(child)
	}
	return nil
}
func (s *resourceSchema) AdditionalProperties() common.SchemaOrBool {
	if ap := s.Structural.Structural.AdditionalProperties; ap != nil {
		return &resourceSchemaOrBool{ap}
	}
	return nil
}
func (s *resourceSchema) WithTypeAndObjectMeta() common.Schema {
	return targetModel(model.WithTypeAndObjectMeta(s.Structural.Structural))
}

type resourceSchemaOrBool struct{ value *structural.StructuralOrBool }

func (s *resourceSchemaOrBool) Allows() bool { return s.value.Bool }
func (s *resourceSchemaOrBool) Schema() common.Schema {
	if s.value.Structural == nil {
		return nil
	}
	return targetModel(s.value.Structural)
}

// Rendered resources are unstructured JSON-like maps. Formats such as byte and
// date-time remain strings at runtime, unlike CRD admission's typed adapter.
func (s *resourceSchema) Format() string {
	if s.Structural.Structural.Type == "string" {
		return ""
	}
	return s.Structural.Format()
}
