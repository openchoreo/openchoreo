// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestEvalForEach_EvalError(t *testing.T) {
	engine := template.NewEngine()

	_, err := EvalForEach(engine, "${nonexistent.list}", "item", map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestEvalForEach_ConversionError(t *testing.T) {
	engine := template.NewEngine()

	// forEach expression evaluates to an integer, which is not iterable
	_, err := EvalForEach(engine, "${parameters.count}", "item", map[string]any{
		"parameters": map[string]any{
			"count": 42,
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "array or map", "error should mention expected types")
}

func TestEvalForEach_DefaultVarName(t *testing.T) {
	engine := template.NewEngine()

	ctx := map[string]any{
		"items": []any{"a", "b"},
	}
	// Pass empty varName to use the default "item"
	contexts, err := EvalForEach(engine, "${items}", "", ctx)
	require.NoError(t, err)
	require.Len(t, contexts, 2)

	// Verify the default variable name "item" is used in the context
	assert.Equal(t, "a", contexts[0]["item"])
	assert.Equal(t, "b", contexts[1]["item"])
}

func TestTruncateRule_LongRule(t *testing.T) {
	// Create a rule string longer than 120 characters
	longRule := "${" + strings.Repeat("a", 200) + "}"
	result := truncateRule(longRule, 120)
	assert.Len(t, []rune(result), 123, "truncated result should be maxLen runes + 3 for '...'")
	assert.True(t, strings.HasSuffix(result, "..."), "truncated result should end with '...'")
	assert.True(t, strings.HasPrefix(result, "${"+strings.Repeat("a", 118)), "truncated result should preserve the original prefix")
}

func TestTruncateRule_ShortRule(t *testing.T) {
	shortRule := "${parameters.replicas > 0}"
	result := truncateRule(shortRule, 120)
	assert.Equal(t, shortRule, result, "short rule should be returned as-is")
}
