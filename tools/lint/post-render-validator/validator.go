// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package postrendervalidator validates rendered Kubernetes resources. It
// turns a bundle of OpenChoreo documents into a rendering pipeline input,
// renders it with the real pipeline, and reports what the rendered objects
// look wrong.
package postrendervalidator

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/pipeline/component"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

// ValidateBundle renders the bundle and reports the post-render findings: the
// declared post-render validations that failed, the errors the rendering
// itself produced, and the built-in policy rules.
func ValidateBundle(ctx context.Context, b *Bundle) (ruleengine.Diagnostics, error) {
	var diags ruleengine.Diagnostics

	out, failures, err := RenderBundle(ctx, b)
	for _, f := range failures {
		diags = append(diags, ruleengine.Diagnostic{
			Severity: ruleengine.SeverityError,
			Code:     "post-render-validation-failed",
			Message:  formatFailure(f),
		})
	}
	if err != nil {
		diags = append(diags, ruleengine.Diagnostic{
			Severity: ruleengine.SeverityError,
			Code:     "post-render-error",
			Message:  err.Error(),
		})
		return diags, nil
	}
	if out == nil {
		return diags, nil
	}
	for _, r := range defaultRules() {
		diags = append(diags, r.Evaluate(out.Resources)...)
	}
	return diags, nil
}

// formatFailure describes one declared post-render failure. The label names
// the declaring source, the resource names the object the failure is about,
// and the iteration names the forEach item when the failure is per-item.
func formatFailure(f component.PostRenderFailure) string {
	msg := fmt.Sprintf("%s: %s", f.Label, f.Message)
	if f.Resource != "" {
		msg = fmt.Sprintf("%s: %s", f.Label, f.Resource)
		if f.Message != "" {
			msg = fmt.Sprintf("%s: %s", msg, f.Message)
		}
	}
	if f.Iteration != "" {
		msg = fmt.Sprintf("%s (%s)", msg, f.Iteration)
	}
	return msg
}

// ValidateDocuments groups the given documents into a bundle and validates it.
func ValidateDocuments(ctx context.Context, docs []*parser.DocumentNode) (ruleengine.Diagnostics, error) {
	b, err := BundleFromDocuments(docs)
	if err != nil {
		return nil, err
	}
	return ValidateBundle(ctx, b)
}

// Validate validates each of the given bundles and returns all the findings.
func Validate(ctx context.Context, bundles []*Bundle) (ruleengine.Diagnostics, error) {
	var all ruleengine.Diagnostics
	for _, b := range bundles {
		diags, err := ValidateBundle(ctx, b)
		if err != nil {
			return all, err
		}
		all = append(all, diags...)
	}
	return all, nil
}

// unused keeps the fmt import referenced when the build tags change.
var _ = fmt.Sprintf
