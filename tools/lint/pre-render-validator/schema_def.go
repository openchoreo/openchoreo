// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func validateSchemaDefinitions(index *index) []Finding {
	var findings []Finding

	for _, ct := range index.componentTypes {
		if ct.doc.Spec.Parameters != nil && ct.doc.Spec.Parameters.OpenAPIV3Schema != nil {
		}
		_, _, errs := schemautil.ExtractAndValidateSchemas(ct.doc.Spec.Parameters, ct.doc.Spec.EnvironmentConfigs, field.NewPath("spec"))
		findings = append(findings, mapSchemaDefErrors(errs, ct.file, ct.pdoc, "ComponentType", ct.doc.Name)...)
	}
	for _, cct := range index.clusterComponentTypes {
		_, _, errs := schemautil.ExtractAndValidateSchemas(cct.doc.Spec.Parameters, cct.doc.Spec.EnvironmentConfigs, field.NewPath("spec"))
		findings = append(findings, mapSchemaDefErrors(errs, cct.file, cct.pdoc, "ClusterComponentType", cct.doc.Name)...)
	}
	for _, t := range index.traits {
		_, _, errs := schemautil.ExtractAndValidateSchemas(t.doc.Spec.Parameters, t.doc.Spec.EnvironmentConfigs, field.NewPath("spec"))
		findings = append(findings, mapSchemaDefErrors(errs, t.file, t.pdoc, "Trait", t.doc.Name)...)
	}
	for _, ct2 := range index.clusterTraits {
		_, _, errs := schemautil.ExtractAndValidateSchemas(ct2.doc.Spec.Parameters, ct2.doc.Spec.EnvironmentConfigs, field.NewPath("spec"))
		findings = append(findings, mapSchemaDefErrors(errs, ct2.file, ct2.pdoc, "ClusterTrait", ct2.doc.Name)...)
	}

	return findings
}

func mapSchemaDefErrors(errs field.ErrorList, file string, doc *parser.DocumentNode, kind, name string) []Finding {
	var findings []Finding
	for _, err := range errs {
		if err == nil {
			continue
		}
		line, column := fallbackLineColumn()
		if doc != nil {
			node := doc.Root
			for _, part := range strings.Split(err.Field, ".") {
				child := findMappingValue(node, part)
				if child == nil {
					break
				}
				node = child
			}
			if schemaNode := findMappingValue(node, "openAPIV3Schema"); schemaNode != nil {
				node = schemaNode
			}
			// Strict JSON decoding reports a field name rather than its full
			// schema path. Use it only when it identifies one source key.
			const marker = `unknown field "`
			if start := strings.Index(err.Error(), marker); start >= 0 {
				rest := err.Error()[start+len(marker):]
				if end := strings.Index(rest, `"`); end >= 0 {
					keys := schemaKeys(node, rest[:end])
					if len(keys) == 1 {
						node = keys[0]
					}
				}
			}
			line, column = position(node)
		}

		code := "schema-definition-invalid"
		msg := err.Error()
		if strings.Contains(err.Error(), "failed to parse") {
			code = "schema-definition-parse-error"
		} else if strings.Contains(err.Error(), "unknown or invalid fields") {
			code = "schema-definition-invalid-fields"
		}

		findings = append(findings, Finding{
			Path:     file,
			Severity: SeverityError,
			Code:     code,
			Message:  fmt.Sprintf("%s %q: %s", kind, name, msg),
			Line:     line,
			Column:   column,
		})
	}
	return findings
}

var _ *v1alpha1.ComponentType

// schemaKeys preserves source identity; ambiguous repeated keys deliberately
// fall back to their schema section rather than highlighting the wrong field.
func schemaKeys(node *yaml.Node, name string) []*yaml.Node {
	if node == nil {
		return nil
	}
	var keys []*yaml.Node
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == name {
				keys = append(keys, node.Content[i])
			}
			keys = append(keys, schemaKeys(node.Content[i+1], name)...)
		}
	} else {
		for _, child := range node.Content {
			keys = append(keys, schemaKeys(child, name)...)
		}
	}
	return keys
}
