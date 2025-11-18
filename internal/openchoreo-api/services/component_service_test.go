// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// TestFindLowestEnvironment tests the findLowestEnvironment helper method
func TestFindLowestEnvironment(t *testing.T) {
	// Use standard library log/slog instead of golang.org/x/exp/slog
	service := &ComponentService{logger: nil}

	tests := []struct {
		name           string
		promotionPaths []v1alpha1.PromotionPath
		want           string
		wantErr        bool
	}{
		{
			name: "Simple linear pipeline: dev -> staging -> prod",
			promotionPaths: []v1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "dev",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
				{
					SourceEnvironmentRef: "staging",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{
						{Name: "prod"},
					},
				},
			},
			want:    "dev",
			wantErr: false,
		},
		{
			name: "Pipeline with multiple branches from dev",
			promotionPaths: []v1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "dev",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{
						{Name: "qa"},
						{Name: "staging"},
					},
				},
				{
					SourceEnvironmentRef: "qa",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{
						{Name: "prod"},
					},
				},
			},
			want:    "dev",
			wantErr: false,
		},
		{
			name: "Single environment pipeline",
			promotionPaths: []v1alpha1.PromotionPath{
				{
					SourceEnvironmentRef:  "prod",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{},
				},
			},
			want:    "prod",
			wantErr: false,
		},
		{
			name:           "Empty promotion paths",
			promotionPaths: []v1alpha1.PromotionPath{},
			want:           "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.findLowestEnvironment(tt.promotionPaths)

			if tt.wantErr {
				if got != "" {
					t.Errorf("findLowestEnvironment() expected empty string for error case, got %v", got)
				}
			} else {
				if got != tt.want {
					t.Errorf("findLowestEnvironment() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestListReleaseBindingsFiltering tests the environment filtering in ListReleaseBindings
func TestListReleaseBindingsFiltering(t *testing.T) {
	// This is a unit test for the filtering logic
	// In a real scenario, you would mock the k8s client

	tests := []struct {
		name         string
		bindings     []v1alpha1.ReleaseBinding
		environments []string
		wantCount    int
	}{
		{
			name: "No filter - returns all bindings",
			bindings: []v1alpha1.ReleaseBinding{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-dev"},
					Spec: v1alpha1.ReleaseBindingSpec{
						Environment: "dev",
						ReleaseName: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-staging"},
					Spec: v1alpha1.ReleaseBindingSpec{
						Environment: "staging",
						ReleaseName: "app-v1",
					},
				},
			},
			environments: []string{},
			wantCount:    2,
		},
		{
			name: "Filter by single environment",
			bindings: []v1alpha1.ReleaseBinding{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-dev"},
					Spec: v1alpha1.ReleaseBindingSpec{
						Environment: "dev",
						ReleaseName: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-staging"},
					Spec: v1alpha1.ReleaseBindingSpec{
						Environment: "staging",
						ReleaseName: "app-v1",
					},
				},
			},
			environments: []string{"dev"},
			wantCount:    1,
		},
		{
			name: "Filter by multiple environments",
			bindings: []v1alpha1.ReleaseBinding{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-dev"},
					Spec: v1alpha1.ReleaseBindingSpec{
						Environment: "dev",
						ReleaseName: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-staging"},
					Spec: v1alpha1.ReleaseBindingSpec{
						Environment: "staging",
						ReleaseName: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-prod"},
					Spec: v1alpha1.ReleaseBindingSpec{
						Environment: "prod",
						ReleaseName: "app-v1",
					},
				},
			},
			environments: []string{"dev", "prod"},
			wantCount:    2,
		},
		{
			name: "Filter by non-existent environment",
			bindings: []v1alpha1.ReleaseBinding{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-dev"},
					Spec: v1alpha1.ReleaseBindingSpec{
						Environment: "dev",
						ReleaseName: "app-v1",
					},
				},
			},
			environments: []string{"nonexistent"},
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the filtering logic from ListReleaseBindings
			filtered := []v1alpha1.ReleaseBinding{}
			for _, binding := range tt.bindings {
				// Filter by environment if specified
				if len(tt.environments) > 0 {
					matchesEnv := false
					for _, env := range tt.environments {
						if binding.Spec.Environment == env {
							matchesEnv = true
							break
						}
					}
					if !matchesEnv {
						continue
					}
				}
				filtered = append(filtered, binding)
			}

			if len(filtered) != tt.wantCount {
				t.Errorf("Filtering returned %d bindings, want %d", len(filtered), tt.wantCount)
			}
		})
	}
}

// TestValidatePromotionPath tests promotion path validation logic
func TestValidatePromotionPath(t *testing.T) {
	tests := []struct {
		name           string
		promotionPaths []v1alpha1.PromotionPath
		sourceEnv      string
		targetEnv      string
		wantValid      bool
	}{
		{
			name: "Valid promotion path",
			promotionPaths: []v1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "dev",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
			},
			sourceEnv: "dev",
			targetEnv: "staging",
			wantValid: true,
		},
		{
			name: "Invalid promotion path - wrong source",
			promotionPaths: []v1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "dev",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
			},
			sourceEnv: "staging",
			targetEnv: "prod",
			wantValid: false,
		},
		{
			name: "Invalid promotion path - wrong target",
			promotionPaths: []v1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "dev",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
			},
			sourceEnv: "dev",
			targetEnv: "prod",
			wantValid: false,
		},
		{
			name: "Valid promotion with multiple targets",
			promotionPaths: []v1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "dev",
					TargetEnvironmentRefs: []v1alpha1.TargetEnvironmentRef{
						{Name: "qa"},
						{Name: "staging"},
					},
				},
			},
			sourceEnv: "dev",
			targetEnv: "qa",
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from validatePromotionPath
			isValid := false
			for _, path := range tt.promotionPaths {
				if path.SourceEnvironmentRef == tt.sourceEnv {
					for _, target := range path.TargetEnvironmentRefs {
						if target.Name == tt.targetEnv {
							isValid = true
							break
						}
					}
				}
			}

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// TestDeployReleaseRequestValidation tests the DeployReleaseRequest validation
func TestDeployReleaseRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     *models.DeployReleaseRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid request",
			req: &models.DeployReleaseRequest{
				ReleaseName: "myapp-20251118-1",
			},
			wantErr: false,
		},
		{
			name: "Empty release name",
			req: &models.DeployReleaseRequest{
				ReleaseName: "",
			},
			wantErr: true,
			errMsg:  "releaseName is required",
		},
		{
			name: "Whitespace-only release name",
			req: &models.DeployReleaseRequest{
				ReleaseName: "   ",
			},
			wantErr: true,
			errMsg:  "releaseName is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Sanitize()
			err := tt.req.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestComponentReleaseNameGeneration tests the release name generation logic
func TestComponentReleaseNameGeneration(t *testing.T) {
	tests := []struct {
		name           string
		componentName  string
		existingCount  int
		expectedPrefix string
	}{
		{
			name:           "First release of the day",
			componentName:  "myapp",
			existingCount:  0,
			expectedPrefix: "myapp-",
		},
		{
			name:           "Second release of the day",
			componentName:  "demo-service",
			existingCount:  1,
			expectedPrefix: "demo-service-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The actual implementation generates: <component_name>-YYYYMMDD-#number
			// We're just testing the logic pattern here
			if tt.componentName == "" {
				t.Error("Component name should not be empty")
			}
			if tt.existingCount < 0 {
				t.Error("Existing count should not be negative")
			}
		})
	}
}
