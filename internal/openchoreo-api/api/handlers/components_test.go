// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"
	"time"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
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

func TestToGenComponentType(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name                string
		input               *models.ComponentTypeResponse
		wantAllowedTraits   *[]string
		wantName            string
		wantAllowedWFs      bool
		wantAllowedWFsCount int
	}{
		{
			name: "No allowed traits",
			input: &models.ComponentTypeResponse{
				Name:         "service",
				WorkloadType: "deployment",
				CreatedAt:    now,
			},
			wantName:          "service",
			wantAllowedTraits: nil,
		},
		{
			name: "Allowed traits with default Trait kind uses name only",
			input: &models.ComponentTypeResponse{
				Name:         "service",
				WorkloadType: "deployment",
				CreatedAt:    now,
				AllowedTraits: []models.AllowedTraitResponse{
					{Kind: "Trait", Name: "logging"},
					{Kind: "Trait", Name: "autoscaler"},
				},
			},
			wantName:          "service",
			wantAllowedTraits: ptr.To([]string{"logging", "autoscaler"}),
		},
		{
			name: "Allowed traits with empty kind uses name only",
			input: &models.ComponentTypeResponse{
				Name:         "worker",
				WorkloadType: "deployment",
				CreatedAt:    now,
				AllowedTraits: []models.AllowedTraitResponse{
					{Kind: "", Name: "logging"},
				},
			},
			wantName:          "worker",
			wantAllowedTraits: ptr.To([]string{"logging"}),
		},
		{
			name: "Allowed traits with ClusterTrait kind uses kind:name format",
			input: &models.ComponentTypeResponse{
				Name:         "service",
				WorkloadType: "deployment",
				CreatedAt:    now,
				AllowedTraits: []models.AllowedTraitResponse{
					{Kind: "ClusterTrait", Name: "global-logger"},
				},
			},
			wantName:          "service",
			wantAllowedTraits: ptr.To([]string{"ClusterTrait:global-logger"}),
		},
		{
			name: "Mixed trait kinds",
			input: &models.ComponentTypeResponse{
				Name:         "service",
				WorkloadType: "deployment",
				CreatedAt:    now,
				AllowedTraits: []models.AllowedTraitResponse{
					{Kind: "Trait", Name: "logging"},
					{Kind: "ClusterTrait", Name: "global-autoscaler"},
					{Kind: "", Name: "simple-trait"},
				},
			},
			wantName:          "service",
			wantAllowedTraits: ptr.To([]string{"logging", "ClusterTrait:global-autoscaler", "simple-trait"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toGenComponentType(tt.input)

			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}

			if tt.wantAllowedTraits == nil {
				if result.AllowedTraits != nil {
					t.Errorf("AllowedTraits = %v, want nil", result.AllowedTraits)
				}
				return
			}

			if result.AllowedTraits == nil {
				t.Fatal("AllowedTraits is nil, want non-nil")
			}

			got := *result.AllowedTraits
			want := *tt.wantAllowedTraits
			if len(got) != len(want) {
				t.Fatalf("AllowedTraits has %d items, want %d", len(got), len(want))
			}

			for i := range got {
				if got[i] != want[i] {
					t.Errorf("AllowedTraits[%d] = %q, want %q", i, got[i], want[i])
				}
			}
		})
	}
}
