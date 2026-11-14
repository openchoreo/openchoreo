// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ruleengine

import (
	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// Diagnostic codes seen when the CEL rule itself cannot run: the document's
// openAPIV3Schema section failed to resolve, or the schema-aware environment
// could not be built. These are distinct from the per-expression codes the
// validator emits (cel-parse-error, cel-type-error, cel-unknown-name,
// cel-unknown-function).
const (
	// CodeCELSchemaInvalid reports a malformed spec.parameters /
	// spec.environmentConfigs.openAPIV3Schema that could not be resolved.
	CodeCELSchemaInvalid = "cel-schema-invalid"
	// CodeCELEnvError reports an internal failure building the CEL environment.
	CodeCELEnvError = "cel-env-error"
)

// CELRules type-checks every ${...} expression in a CEL-bearing kind
// (ComponentType, Trait, ResourceType and their cluster variants) against the
// schema-aware environment, mirroring the webhook validation in
// internal/validation. For every other kind Evaluate is a no-op.
type CELRules struct{}

func (r *CELRules) Name() string { return "cel-rules" }

// Evaluate extracts the parameters / environmentConfigs schemas from the
// document and runs schema-aware CEL validation over its template expressions.
func (r *CELRules) Evaluate(doc *parser.DocumentNode) Diagnostics {
	pair, err := celvalidator.ExtractSchemas(doc.Spec)
	if err != nil {
		return Diagnostics{{
			Range:    doc.Range,
			Severity: SeverityError,
			Code:     CodeCELSchemaInvalid,
			Message:  err.Error(),
		}}
	}

	findings, err := celvalidator.ValidateDocument(doc, pair)
	if err != nil {
		return Diagnostics{{
			Range:    doc.Range,
			Severity: SeverityError,
			Code:     CodeCELEnvError,
			Message:  err.Error(),
		}}
	}

	diags := make(Diagnostics, 0, len(findings))
	for _, f := range findings {
		diags = append(diags, Diagnostic{
			Range:    f.Range,
			Severity: SeverityError,
			Code:     f.Code,
			Message:  f.Message,
		})
	}
	return diags
}
