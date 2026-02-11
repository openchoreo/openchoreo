// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

func TestEvaluateValidationRules(t *testing.T) {
	engine := template.NewEngine()

	context := map[string]any{
		"parameters": map[string]any{
			"replicas": int64(3),
			"expose":   true,
			"name":     "my-app",
		},
	}

	tests := []struct {
		name    string
		rules   []v1alpha1.ValidationRule
		context map[string]any
		wantErr bool
		errMsgs []string
	}{
		{
			name:    "no rules returns nil",
			rules:   nil,
			context: context,
			wantErr: false,
		},
		{
			name:    "empty rules returns nil",
			rules:   []v1alpha1.ValidationRule{},
			context: context,
			wantErr: false,
		},
		{
			name: "all rules pass",
			rules: []v1alpha1.ValidationRule{
				{Rule: "${parameters.replicas > 0}", Message: "replicas must be positive"},
				{Rule: "${parameters.expose == true}", Message: "must be exposed"},
			},
			context: context,
			wantErr: false,
		},
		{
			name: "single rule fails with index and rule text",
			rules: []v1alpha1.ValidationRule{
				{Rule: "${parameters.replicas > 10}", Message: "replicas must be greater than 10"},
			},
			context: context,
			wantErr: true,
			errMsgs: []string{
				`rule[0]`,
				`${parameters.replicas > 10}`,
				"evaluated to false",
				"replicas must be greater than 10",
			},
		},
		{
			name: "multiple rules fail without short-circuiting",
			rules: []v1alpha1.ValidationRule{
				{Rule: "${parameters.replicas > 10}", Message: "replicas must be greater than 10"},
				{Rule: "${parameters.expose == false}", Message: "must not be exposed"},
				{Rule: "${parameters.replicas > 0}", Message: "replicas must be positive"},
			},
			context: context,
			wantErr: true,
			errMsgs: []string{
				"rule[0]",
				"replicas must be greater than 10",
				"rule[1]",
				"must not be exposed",
			},
		},
		{
			name: "rule evaluation error includes index and rule text",
			rules: []v1alpha1.ValidationRule{
				{Rule: "${nonexistent.field}", Message: "should not reach"},
			},
			context: context,
			wantErr: true,
			errMsgs: []string{
				"rule[0]",
				"${nonexistent.field}",
				"evaluation error",
			},
		},
		{
			name: "rule returning non-boolean includes index and rule text",
			rules: []v1alpha1.ValidationRule{
				{Rule: "${parameters.name}", Message: "should not reach"},
			},
			context: context,
			wantErr: true,
			errMsgs: []string{
				"rule[0]",
				"${parameters.name}",
				"must evaluate to boolean",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateValidationRules(engine, tt.rules, tt.context)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				for _, msg := range tt.errMsgs {
					if !strings.Contains(err.Error(), msg) {
						t.Errorf("expected error to contain %q, got %q", msg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error but got: %v", err)
				}
			}
		})
	}
}
