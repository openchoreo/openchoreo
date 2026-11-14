// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ruleengine

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// EnumRules validates that fields with fixed sets of allowed values
// only contain those values.
type EnumRules struct{}

func (r *EnumRules) Name() string { return "enum" }

func (r *EnumRules) Evaluate(doc *parser.DocumentNode) Diagnostics {
	var diags Diagnostics

	diags = append(diags, r.validateAPIVersion(doc)...)
	diags = append(diags, r.validateKind(doc)...)
	diags = append(diags, r.validateSpecEnums(doc)...)

	return diags
}

func (r *EnumRules) validateAPIVersion(doc *parser.DocumentNode) Diagnostics {
	if doc.APIVersion == "" {
		return nil
	}

	if !parser.ValidAPIVersions[doc.APIVersion] {
		f := doc.GetField("apiVersion")
		rr := doc.Range
		if f != nil {
			rr = nodeRange(f.ValueNode)
		}
		return Diagnostics{{
			Range:    rr,
			Severity: SeverityError,
			Code:     "invalid-apiVersion",
			Message:  fmt.Sprintf("apiVersion %q is not recognized; expected one of: openchoreo.dev/v1alpha1", doc.APIVersion),
		}}
	}
	return nil
}

func (r *EnumRules) validateKind(doc *parser.DocumentNode) Diagnostics {
	if doc.Kind == "" {
		return nil
	}

	if !parser.KnownKinds[doc.Kind] {
		f := doc.GetField("kind")
		rr := doc.Range
		if f != nil {
			rr = nodeRange(f.ValueNode)
		}
		return Diagnostics{{
			Range:    rr,
			Severity: SeverityError,
			Code:     "unknown-kind",
			Message:  fmt.Sprintf("kind %q is not a recognized OpenChoreo resource kind", doc.Kind),
		}}
	}
	return nil
}

// knownImagePullPolicies lists valid values for the imagePullPolicy field.
var knownImagePullPolicies = map[string]bool{
	"Always":       true,
	"IfNotPresent": true,
	"Never":        true,
}

func (r *EnumRules) validateSpecEnums(doc *parser.DocumentNode) Diagnostics {
	if doc.Spec == nil || doc.Spec.Kind != yaml.MappingNode {
		return nil
	}

	var diags Diagnostics

	for i := 0; i+1 < len(doc.Spec.Content); i += 2 {
		key := doc.Spec.Content[i]
		val := doc.Spec.Content[i+1]

		if key.Value == "imagePullPolicy" && val.Kind == yaml.ScalarNode {
			if !knownImagePullPolicies[val.Value] {
				diags = append(diags, Diagnostic{
					Range:    nodeRange(val),
					Severity: SeverityError,
					Code:     "invalid-imagePullPolicy",
					Message:  fmt.Sprintf("imagePullPolicy %q is not valid; expected one of: Always, IfNotPresent, Never", val.Value),
				})
			}
		}
	}

	return diags
}
