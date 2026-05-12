// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

import (
	"testing"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation/field"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// testCELEnv builds a CEL environment with both "resource" and "principal" map variables
// so extractDottedAccesses tests can use either root without depending on GetCELEnv.
func testCELEnv(t *testing.T) *cel.Env {
	t.Helper()
	env, err := cel.NewEnv(
		cel.Variable("resource", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("user", cel.MapType(cel.StringType, cel.DynType)),
	)
	require.NoError(t, err)
	return env
}

// getCelExpr compiles expr using the test CEL env and returns the root celast.Expr.
func getCelExpr(t *testing.T, expr string) celast.Expr {
	t.Helper()
	ast, issues := testCELEnv(t).Compile(expr)
	require.Nil(t, issues.Err(), "compile error: %v", issues.Err())
	return ast.NativeRep().Expr()
}

func TestExtractDottedAccesses(t *testing.T) {
	roots := authzcore.KnownCELRoots

	tests := []struct {
		name       string
		expression string
		roots      map[string]bool
		want       []string
	}{
		{
			name:       "single known root access",
			expression: `resource.environment == "prod"`,
			roots:      roots,
			want:       []string{"resource.environment"},
		},
		{
			name:       "same attribute referenced multiple times",
			expression: `resource.environment == "prod" || resource.environment == "staging"`,
			roots:      roots,
			want:       []string{"resource.environment"},
		},
		{
			name:       "no select expression — pure literal comparison",
			expression: `"prod" == "prod"`,
			roots:      roots,
			want:       nil,
		},
		{
			name:       "unknown root is not collected",
			expression: `resource.environment == "prod"`,
			roots:      map[string]bool{"other": true},
			want:       nil,
		},
		{
			name:       "empty roots map yields nothing",
			expression: `resource.environment == "prod"`,
			roots:      map[string]bool{},
			want:       nil,
		},
		{
			name:       "complex expression with multiple conditions and roots",
			expression: `(resource.environment == "prod") || (resource.componentType in ["db"] && user.group == "devs")`,
			roots:      map[string]bool{"resource": true, "user": true, "componentType": true},
			want:       []string{"resource.environment", "resource.componentType", "user.group"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := getCelExpr(t, tt.expression)
			got := extractDottedAccesses(expr, tt.roots)
			if tt.want == nil {
				require.Nil(t, got)
			} else {
				require.ElementsMatch(t, tt.want, got)
			}
		})
	}
}

func TestValidateCondition(t *testing.T) {
	tests := []struct {
		name    string
		cond    openchoreodevv1alpha1.AuthzCondition
		wantErr string
	}{
		{
			name: "valid condition",
			cond: openchoreodevv1alpha1.AuthzCondition{
				Actions:    []string{authzcore.ActionCreateReleaseBinding},
				Expression: `resource.environment == "prod"`,
			},
		},
		{
			name: "valid condition for resourcereleasebinding",
			cond: openchoreodevv1alpha1.AuthzCondition{
				Actions:    []string{authzcore.ActionCreateResourceReleaseBinding},
				Expression: `resource.environment == "prod"`,
			},
		},
		{
			name: "empty actions",
			cond: openchoreodevv1alpha1.AuthzCondition{
				Actions:    []string{},
				Expression: `resource.environment == "prod"`,
			},
			wantErr: "actions must be non-empty",
		},
		{
			name: "nil actions",
			cond: openchoreodevv1alpha1.AuthzCondition{
				Actions:    nil,
				Expression: `resource.environment == "prod"`,
			},
			wantErr: "actions must be non-empty",
		},
		{
			name: "empty expression",
			cond: openchoreodevv1alpha1.AuthzCondition{
				Actions:    []string{authzcore.ActionCreateReleaseBinding},
				Expression: "",
			},
			wantErr: "expression must be non-empty",
		},
		{
			name: "expression with no attribute access",
			cond: openchoreodevv1alpha1.AuthzCondition{
				Actions:    []string{authzcore.ActionCreateReleaseBinding},
				Expression: `true`,
			},
			wantErr: "expression must reference at least one allowed attribute",
		},
		{
			name: "invalid CEL syntax",
			cond: openchoreodevv1alpha1.AuthzCondition{
				Actions:    []string{authzcore.ActionCreateReleaseBinding},
				Expression: `resource.environment ==`,
			},
			wantErr: "CEL compile error",
		},
		{
			name: "expression returns non-bool",
			cond: openchoreodevv1alpha1.AuthzCondition{
				Actions:    []string{authzcore.ActionCreateReleaseBinding},
				Expression: `resource.environment`,
			},
			wantErr: "expression must return bool",
		},
		{
			name: "action set with empty attribute intersection",
			cond: openchoreodevv1alpha1.AuthzCondition{
				// ActionViewLogs has resource.environment; ActionViewProject does not — intersection is empty.
				Actions:    []string{authzcore.ActionViewLogs, authzcore.ActionViewProject},
				Expression: `resource.environment == "prod"`,
			},
			wantErr: "conditions are not supported for the specified action set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateCondition(tt.cond, field.NewPath("spec").Child("roleMappings").Index(0).Child("conditions").Index(0))
			if tt.wantErr == "" {
				require.Empty(t, errs)
			} else {
				require.NotEmpty(t, errs)
				require.ErrorContains(t, errs.ToAggregate(), tt.wantErr)
			}
		})
	}
}

func TestValidateRoleMappings(t *testing.T) {
	validCond := openchoreodevv1alpha1.AuthzCondition{
		Actions:    []string{authzcore.ActionCreateReleaseBinding},
		Expression: `resource.environment == "prod"`,
	}
	emptyActionsCond := openchoreodevv1alpha1.AuthzCondition{
		Actions:    []string{},
		Expression: `resource.environment == "prod"`,
	}
	emptyExprCond := openchoreodevv1alpha1.AuthzCondition{
		Actions:    []string{authzcore.ActionCreateReleaseBinding},
		Expression: "",
	}

	tests := []struct {
		name     string
		mappings []openchoreodevv1alpha1.RoleMapping
		wantErrs []string
	}{
		{
			name:     "nil mappings",
			mappings: nil,
		},
		{
			name:     "mapping with no conditions",
			mappings: []openchoreodevv1alpha1.RoleMapping{{Conditions: nil}},
		},
		{
			name: "mapping with valid condition",
			mappings: []openchoreodevv1alpha1.RoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCond}},
			},
		},
		{
			name: "error path includes correct indices",
			mappings: []openchoreodevv1alpha1.RoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCond}},
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCond, emptyActionsCond}},
			},
			wantErrs: []string{"spec.roleMappings[1].conditions[1]"},
		},
		{
			name: "multiple invalid conditions across mappings are all reported",
			mappings: []openchoreodevv1alpha1.RoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{emptyActionsCond}},
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCond, emptyExprCond}},
			},
			wantErrs: []string{
				"spec.roleMappings[0].conditions[0]",
				"spec.roleMappings[1].conditions[1]",
			},
		},
		{
			name: "multiple invalid conditions within same mapping are all reported",
			mappings: []openchoreodevv1alpha1.RoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{emptyActionsCond, emptyExprCond}},
			},
			wantErrs: []string{
				"spec.roleMappings[0].conditions[0]",
				"spec.roleMappings[0].conditions[1]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateRoleMappings(tt.mappings)
			if len(tt.wantErrs) == 0 {
				require.Empty(t, errs)
			} else {
				require.Len(t, errs, len(tt.wantErrs))
				aggregate := errs.ToAggregate().Error()
				for _, wantErr := range tt.wantErrs {
					require.Contains(t, aggregate, wantErr)
				}
			}
		})
	}
}
