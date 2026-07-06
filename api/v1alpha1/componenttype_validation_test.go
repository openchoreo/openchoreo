// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "testing"

func TestComponentTypeEffectivePreRenderValidations(t *testing.T) {
	legacy := []ValidationRule{{Rule: "${1 == 1}", Message: "legacy"}}
	fresh := []ValidationRule{{Rule: "${2 == 2}", Message: "fresh"}}

	tests := []struct {
		name string
		spec ComponentTypeSpec
		want string // Message of the single expected rule, or "" for none
	}{
		{name: "neither set", spec: ComponentTypeSpec{}, want: ""},
		{name: "only legacy validations", spec: ComponentTypeSpec{Validations: legacy}, want: "legacy"},
		{name: "only preRenderValidations", spec: ComponentTypeSpec{PreRenderValidations: fresh}, want: "fresh"},
		{name: "preRender wins when both set", spec: ComponentTypeSpec{Validations: legacy, PreRenderValidations: fresh}, want: "fresh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.EffectivePreRenderValidations()
			if tt.want == "" {
				if len(got) != 0 {
					t.Fatalf("expected no rules, got %d", len(got))
				}
				return
			}
			if len(got) != 1 || got[0].Message != tt.want {
				t.Fatalf("expected rule %q, got %+v", tt.want, got)
			}
		})
	}
}

func TestClusterComponentTypeEffectivePreRenderValidations(t *testing.T) {
	legacy := []ValidationRule{{Rule: "${1 == 1}", Message: "legacy"}}
	fresh := []ValidationRule{{Rule: "${2 == 2}", Message: "fresh"}}

	if got := (&ClusterComponentTypeSpec{}).EffectivePreRenderValidations(); len(got) != 0 {
		t.Fatalf("expected no rules for empty spec, got %d", len(got))
	}
	if got := (&ClusterComponentTypeSpec{Validations: legacy}).EffectivePreRenderValidations(); len(got) != 1 || got[0].Message != "legacy" {
		t.Fatalf("expected legacy fallback, got %+v", got)
	}
	if got := (&ClusterComponentTypeSpec{Validations: legacy, PreRenderValidations: fresh}).EffectivePreRenderValidations(); len(got) != 1 || got[0].Message != "fresh" {
		t.Fatalf("expected preRender to win, got %+v", got)
	}
}
