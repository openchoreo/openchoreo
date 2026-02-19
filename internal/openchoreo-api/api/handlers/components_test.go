// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestToModelCreateComponentRequest(t *testing.T) {
	tests := []struct {
		name              string
		input             *gen.CreateComponentRequest
		wantNil           bool
		wantComponentType bool
		wantKind          string
		wantCTName        string
	}{
		{
			name:    "Nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "Non-nil input with ComponentType string",
			input: &gen.CreateComponentRequest{
				Name:          "my-comp",
				ComponentType: ptr.To("deployment/web-app"),
			},
			wantNil:           false,
			wantComponentType: true,
			wantKind:          "ComponentType",
			wantCTName:        "deployment/web-app",
		},
		{
			name: "Non-nil input with nil ComponentType",
			input: &gen.CreateComponentRequest{
				Name: "my-comp",
			},
			wantNil:           false,
			wantComponentType: false,
		},
		{
			name: "Non-nil input with empty string ComponentType",
			input: &gen.CreateComponentRequest{
				Name:          "my-comp",
				ComponentType: ptr.To(""),
			},
			wantNil:           false,
			wantComponentType: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toModelCreateComponentRequest(tt.input)

			if tt.wantNil {
				if result != nil {
					t.Errorf("toModelCreateComponentRequest() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatal("toModelCreateComponentRequest() returned nil, want non-nil")
			}

			if tt.wantComponentType {
				if result.ComponentType == nil {
					t.Fatal("ComponentType is nil, want non-nil")
				}
				if result.ComponentType.Kind != tt.wantKind {
					t.Errorf("ComponentType.Kind = %q, want %q", result.ComponentType.Kind, tt.wantKind)
				}
				if result.ComponentType.Name != tt.wantCTName {
					t.Errorf("ComponentType.Name = %q, want %q", result.ComponentType.Name, tt.wantCTName)
				}
			} else {
				if result.ComponentType != nil {
					t.Errorf("ComponentType = %v, want nil", result.ComponentType)
				}
			}
		})
	}
}

func TestToModelTraits(t *testing.T) {
	traitKind := gen.ComponentTraitInputKindTrait
	clusterTraitKind := gen.ComponentTraitInputKindClusterTrait

	tests := []struct {
		name      string
		input     *[]gen.ComponentTraitInput
		wantNil   bool
		wantCount int
		wantKinds []string
		wantNames []string
	}{
		{
			name:    "Nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name:    "Empty slice returns nil",
			input:   &[]gen.ComponentTraitInput{},
			wantNil: true,
		},
		{
			name: "Traits without kind default to empty string",
			input: &[]gen.ComponentTraitInput{
				{Name: "logging", InstanceName: "app-logging"},
			},
			wantCount: 1,
			wantKinds: []string{""},
			wantNames: []string{"logging"},
		},
		{
			name: "Trait with kind=Trait",
			input: &[]gen.ComponentTraitInput{
				{Name: "logging", InstanceName: "app-logging", Kind: &traitKind},
			},
			wantCount: 1,
			wantKinds: []string{"Trait"},
			wantNames: []string{"logging"},
		},
		{
			name: "Trait with kind=ClusterTrait",
			input: &[]gen.ComponentTraitInput{
				{Name: "global-logger", InstanceName: "my-logger", Kind: &clusterTraitKind},
			},
			wantCount: 1,
			wantKinds: []string{"ClusterTrait"},
			wantNames: []string{"global-logger"},
		},
		{
			name: "Mixed kinds",
			input: &[]gen.ComponentTraitInput{
				{Name: "logging", InstanceName: "app-logging", Kind: &traitKind},
				{Name: "global-logger", InstanceName: "my-logger", Kind: &clusterTraitKind},
				{Name: "autoscaler", InstanceName: "my-scaler"},
			},
			wantCount: 3,
			wantKinds: []string{"Trait", "ClusterTrait", ""},
			wantNames: []string{"logging", "global-logger", "autoscaler"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toModelTraits(tt.input)

			if tt.wantNil {
				if result != nil {
					t.Errorf("toModelTraits() = %v, want nil", result)
				}
				return
			}

			if len(result) != tt.wantCount {
				t.Fatalf("toModelTraits() returned %d traits, want %d", len(result), tt.wantCount)
			}

			for i, trait := range result {
				if trait.Kind != tt.wantKinds[i] {
					t.Errorf("trait[%d].Kind = %q, want %q", i, trait.Kind, tt.wantKinds[i])
				}
				if trait.Name != tt.wantNames[i] {
					t.Errorf("trait[%d].Name = %q, want %q", i, trait.Name, tt.wantNames[i])
				}
			}
		})
	}
}
