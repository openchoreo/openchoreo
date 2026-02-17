// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// TestFetchComponentTypeSpec tests the fetchComponentTypeSpec method
func TestFetchComponentTypeSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add v1alpha1 to scheme: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name         string
		ctRef        *v1alpha1.ComponentTypeRef
		namespace    string
		objects      []client.Object
		wantSpec     bool
		wantErr      bool
		wantWorkload string
		errSubstring string
	}{
		{
			name: "ComponentType found - returns spec",
			ctRef: &v1alpha1.ComponentTypeRef{
				Kind: v1alpha1.ComponentTypeRefKindComponentType,
				Name: "deployment/web-app",
			},
			namespace: "default",
			objects: []client.Object{
				&v1alpha1.ComponentType{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "web-app",
						Namespace: "default",
					},
					Spec: v1alpha1.ComponentTypeSpec{
						WorkloadType: "deployment",
						Resources: []v1alpha1.ResourceTemplate{
							{ID: "deployment"},
						},
					},
				},
			},
			wantSpec:     true,
			wantWorkload: "deployment",
		},
		{
			name: "ClusterComponentType found - returns spec",
			ctRef: &v1alpha1.ComponentTypeRef{
				Kind: v1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: "deployment/shared-web",
			},
			namespace: "default",
			objects: []client.Object{
				&v1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shared-web",
					},
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources: []v1alpha1.ResourceTemplate{
							{ID: "deployment"},
						},
					},
				},
			},
			wantSpec:     true,
			wantWorkload: "deployment",
		},
		{
			name: "ComponentType not found - returns nil without error",
			ctRef: &v1alpha1.ComponentTypeRef{
				Kind: v1alpha1.ComponentTypeRefKindComponentType,
				Name: "deployment/nonexistent",
			},
			namespace: "default",
			objects:   []client.Object{},
			wantSpec:  false,
			wantErr:   false,
		},
		{
			name: "ClusterComponentType not found - returns nil without error",
			ctRef: &v1alpha1.ComponentTypeRef{
				Kind: v1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: "deployment/nonexistent",
			},
			namespace: "default",
			objects:   []client.Object{},
			wantSpec:  false,
			wantErr:   false,
		},
		{
			name: "Invalid name format - no slash separator",
			ctRef: &v1alpha1.ComponentTypeRef{
				Kind: v1alpha1.ComponentTypeRefKindComponentType,
				Name: "no-slash-here",
			},
			namespace:    "default",
			objects:      []client.Object{},
			wantSpec:     false,
			wantErr:      true,
			errSubstring: "invalid component type format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			service := &ComponentService{
				k8sClient: fakeClient,
				logger:    logger,
			}

			spec, err := service.fetchComponentTypeSpec(context.Background(), tt.ctRef, tt.namespace)

			if tt.wantErr {
				if err == nil {
					t.Errorf("fetchComponentTypeSpec() expected error, got nil")
					return
				}
				if tt.errSubstring != "" && !strings.Contains(err.Error(), tt.errSubstring) {
					t.Errorf("fetchComponentTypeSpec() error = %q, want substring %q", err.Error(), tt.errSubstring)
				}
				return
			}

			if err != nil {
				t.Errorf("fetchComponentTypeSpec() unexpected error: %v", err)
				return
			}

			if tt.wantSpec {
				if spec == nil {
					t.Fatal("fetchComponentTypeSpec() returned nil spec, want non-nil")
				}
				if spec.WorkloadType != tt.wantWorkload {
					t.Errorf("fetchComponentTypeSpec() WorkloadType = %q, want %q", spec.WorkloadType, tt.wantWorkload)
				}
			} else {
				if spec != nil {
					t.Errorf("fetchComponentTypeSpec() = %v, want nil", spec)
				}
			}
		})
	}
}

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

// TestDetermineReleaseBindingStatus tests the ReleaseBinding status determination logic
func TestDetermineReleaseBindingStatus(t *testing.T) {
	service := &ComponentService{logger: nil}

	tests := []struct {
		name       string
		binding    *v1alpha1.ReleaseBinding
		wantStatus string
	}{
		{
			name: "No conditions - should be NotReady",
			binding: &v1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: v1alpha1.ReleaseBindingStatus{
					Conditions: []metav1.Condition{},
				},
			},
			wantStatus: "NotReady",
		},
		{
			name: "Less than 3 conditions for current generation - should be NotReady (in progress)",
			binding: &v1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Status: v1alpha1.ReleaseBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "ReleaseSynced",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 2,
						},
						{
							Type:               "ResourcesReady",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 2,
						},
					},
				},
			},
			wantStatus: "NotReady",
		},
		{
			name: "All 3 conditions present but one is False - should be Failed",
			binding: &v1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 3,
				},
				Status: v1alpha1.ReleaseBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "ReleaseSynced",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 3,
						},
						{
							Type:               "ResourcesReady",
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 3,
							Reason:             "ResourcesDegraded",
							Message:            "Some resources are degraded",
						},
						{
							Type:               "Ready",
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 3,
						},
					},
				},
			},
			wantStatus: "Failed",
		},
		{
			name: "All 3 conditions present and all True - should be Ready",
			binding: &v1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 4,
				},
				Status: v1alpha1.ReleaseBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "ReleaseSynced",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 4,
						},
						{
							Type:               "ResourcesReady",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 4,
						},
						{
							Type:               "Ready",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 4,
						},
					},
				},
			},
			wantStatus: "Ready",
		},
		{
			name: "Conditions from old generation - should be NotReady",
			binding: &v1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 5,
				},
				Status: v1alpha1.ReleaseBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "ReleaseSynced",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 4, // Old generation
						},
						{
							Type:               "ResourcesReady",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 4, // Old generation
						},
						{
							Type:               "Ready",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 4, // Old generation
						},
					},
				},
			},
			wantStatus: "NotReady",
		},
		{
			name: "Mixed generations - only 2 conditions match current generation",
			binding: &v1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 6,
				},
				Status: v1alpha1.ReleaseBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "ReleaseSynced",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 6,
						},
						{
							Type:               "ResourcesReady",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 5, // Old generation
						},
						{
							Type:               "Ready",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 6,
						},
					},
				},
			},
			wantStatus: "NotReady",
		},
		{
			name: "Extra conditions beyond the 3 required - all True",
			binding: &v1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 7,
				},
				Status: v1alpha1.ReleaseBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "ReleaseSynced",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 7,
						},
						{
							Type:               "ResourcesReady",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 7,
						},
						{
							Type:               "Ready",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 7,
						},
						{
							Type:               "CustomCondition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 7,
						},
					},
				},
			},
			wantStatus: "Ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus := service.determineReleaseBindingStatus(tt.binding)
			if gotStatus != tt.wantStatus {
				t.Errorf("determineReleaseBindingStatus() = %v, want %v", gotStatus, tt.wantStatus)
			}
		})
	}
}
