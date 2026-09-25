// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hooks

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func workflowWith(schema string) *v1alpha1.WorkflowSpec {
	if schema == "" {
		return &v1alpha1.WorkflowSpec{}
	}
	return &v1alpha1.WorkflowSpec{Parameters: &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(schema)}}}
}

func TestValidateParametersAgainstWorkflow(t *testing.T) {
	// The workflow declares image (required) and severity (optional) in the
	// standard OpenAPI v3 form Workflow authors write.
	schema := `{"type":"object","properties":{"image":{"type":"string"},"severity":{"type":"string","default":"HIGH"}},"required":["image"]}`

	tests := []struct {
		name     string
		params   []v1alpha1.HookParameter
		wf       *v1alpha1.WorkflowSpec
		wantErrs []string
	}{
		{
			name:   "accepts parameters that cover the required input and name declared inputs",
			params: []v1alpha1.HookParameter{{Name: "image", From: "${deployment.release}"}, {Name: "severity", Default: strPtr("LOW")}},
			wf:     workflowWith(schema),
		},
		{
			name:     "rejects a parameter the workflow does not declare because the run would ignore it",
			params:   []v1alpha1.HookParameter{{Name: "image", Required: true}, {Name: "shade", Required: true}},
			wf:       workflowWith(schema),
			wantErrs: []string{"spec.parameters[1].name", `does not declare an input named "shade"`},
		},
		{
			name:     "rejects an unmapped required input because the run would be rejected later",
			params:   []v1alpha1.HookParameter{{Name: "severity", Default: strPtr("LOW")}},
			wf:       workflowWith(schema),
			wantErrs: []string{`workflow input "image" is required but not mapped`},
		},
		{
			name:     "rejects every parameter when the workflow declares no inputs",
			params:   []v1alpha1.HookParameter{{Name: "image", Required: true}},
			wf:       workflowWith(""),
			wantErrs: []string{`does not declare an input named "image"`},
		},
		{
			name:   "accepts no parameters when the workflow requires none",
			params: nil,
			wf:     workflowWith(`{"type":"object","properties":{"severity":{"type":"string"}}}`),
		},
		{
			name:   "skips the check when the workflow is unknown",
			params: []v1alpha1.HookParameter{{Name: "anything", Required: true}},
			wf:     nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateParametersAgainstWorkflow(tc.params, "scan", tc.wf, field.NewPath("spec", "parameters"))
			assertErrs(t, errs, tc.wantErrs)
		})
	}
}
